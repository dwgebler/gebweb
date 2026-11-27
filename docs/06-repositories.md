# Repositories and `@ApiResource`

<!-- doctest:strict -->

A repository is a class that owns the storage for one kind of entity: it runs
the SQL, turns rows into objects, and gives the rest of the app a small typed
API. Controllers and services depend on repositories and never see SQL.

When an entity is plain create-read-update-delete data, `@ApiResource` goes
further: give the entity class the decorator and a repository that implements
`Repository<T>`, and Gebweb generates the REST routes, request binding,
validation, field filtering, paging and OpenAPI documentation for it.

## A resource backed by SQLite

The entity declares its fields, which of them clients may read and write, and
its validation rules. The repository implements `Repository<Product>`, plus
the optional `count` and `findBy`:

<!-- doctest:file product.gb -->
```gb
module product;
import db;
import gebweb.query as query;
import gebweb.repository as repository;

@ApiResource("/products")
export class Product {
    @Groups("read")
    ?int id;

    @Groups("read", "write")
    @Assert.notBlank
    string name;

    @Groups("read", "write")
    @Assert.range(min: 0)
    int priceCents;

    static func repositoryClass(): any {
        return ProductRepository;
    }
}

func toProduct(dict<string, any> row): Product {
    let p = Product();
    p.id = row["id"] as int;
    p.name = row["name"] as string;
    p.priceCents = row["price_cents"] as int;
    return p;
}

let sortColumns = {"name": "name", "price": "price_cents", "": "id"};

export class ProductRepository implements repository.Repository<Product> {
    db.Connection conn;

    func ProductRepository(db.Connection conn) {
        this.conn = conn;
    }

    func find(string id): ?Product {
        if (!id.isInt()) {
            return null;
        }
        let row = this.conn.queryOne("SELECT * FROM products WHERE id = ?", id.toInt());
        return row == null ? null : toProduct(row);
    }

    func list(repository.Page page): list<Product> {
        string column = (sortColumns[page.sort] ?? "id") as string;
        let order = page.direction == "desc" ? query.desc(column) : query.asc(column);
        let stmt = query.Query("products")
            .orderBy(order)
            .limit(page.size)
            .offset(page.offset)
            .select([]);
        return this.conn.query(stmt["text"] as string, stmt["params"]).all().map(toProduct);
    }

    func findBy(dict<string, string> criteria): list<Product> {
        let q = query.Query("products").orderBy(query.asc("id"));
        if (criteria.contains("name")) {
            q = q.where(query.like("name", "%" + criteria["name"] + "%"));
        }
        let stmt = q.select([]);
        return this.conn.query(stmt["text"] as string, stmt["params"]).all().map(toProduct);
    }

    func count(): int {
        let row = this.conn.queryOne("SELECT count(*) AS n FROM products");
        return (row as dict<string, any>)["n"] as int;
    }

    func save(Product p): Product {
        if (p.id == null) {
            let result = this.conn.exec(
                "INSERT INTO products (name, price_cents) VALUES (?, ?)",
                p.name,
                p.priceCents
            );
            p.id = result["lastInsertId"] as int;
        } else {
            this.conn.exec(
                "UPDATE products SET name = ?, price_cents = ? WHERE id = ?",
                p.name,
                p.priceCents,
                p.id
            );
        }
        return p;
    }

    func delete(string id): void {
        if (id.isInt()) {
            this.conn.exec("DELETE FROM products WHERE id = ?", id.toInt());
        }
    }
}
```

Mounting it takes the entity class and the repository instance:

<!-- doctest:run -->
```gb
import db;
import gebweb;
import io;
import product;

let conn = db.connect("sqlite", ":memory:");
defer conn.close();
conn.exec(
    "CREATE TABLE products (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, price_cents INTEGER NOT NULL)"
);

let app = gebweb.app([product.Product]);
gebweb.registerInstance(app, product.ProductRepository, product.ProductRepository(conn));
let client = gebweb.TestClient(app);

let created = client.post("/products", {"id": 99, "name": "Kettle", "priceCents": 2999});
io.println("${created.status} ${created.text()}");
client.post("/products", {"name": "Toaster", "priceCents": 1999});
client.post("/products", {"name": "Teapot", "priceCents": 1500});

io.println(client.get("/products?sort=price&limit=2").text());
io.println(client.get("/products?filter=name:pot").text());
io.println(client.request("PATCH", "/products/1", {"priceCents": 2499}, {}).text());
io.println(client.post("/products", {"name": "", "priceCents": -1}).status);
io.println("${client.delete("/products/1").status} ${client.get("/products/1").status}");
```

Output:

```text
201 {"id":1,"name":"Kettle","priceCents":2999}
{"items":[{"id":3,"name":"Teapot","priceCents":1500},{"id":2,"name":"Toaster","priceCents":1999}],"offset":0,"size":2,"total":3}
{"items":[{"id":3,"name":"Teapot","priceCents":1500}],"offset":0,"size":25,"total":1}
{"id":1,"name":"Kettle","priceCents":2499}
422
204 404
```

- The client's `"id": 99` was ignored: `id` is only in the `read` group, so
  clients cannot write it.
- `@Assert` rules on the entity are checked on create, replace and update,
  so the invalid product is a 422.
- `sort` is mapped through a whitelist of columns. A client can ask for any
  sort field, so never pass it to SQL unchecked.
- The repository is a shared singleton holding only the connection pool, so
  concurrent requests can use it safely.

## The generated routes

| Operation | Name in `operations` | Route | Repository call |
|-----------|----------------------|-------|-----------------|
| List | `LIST` (or `GET`) | `GET /products` | `list(page)`, or `findBy(criteria)` with a filter, or `listCursor(cursor)` |
| Create | `CREATE` (or `POST`) | `POST /products` | `save(entity)`, 201 |
| Read | `READ` (or `GET_ONE`) | `GET /products/{id}` | `find(id)`, 404 when null |
| Replace | `REPLACE` (or `PUT`) | `PUT /products/{id}` | `find(id)` then `save(entity)`; fields not sent are cleared |
| Update | `PATCH` | `PATCH /products/{id}` | `find(id)`, changes only the fields sent, then `save(entity)` |
| Delete | `DELETE` | `DELETE /products/{id}` | `delete(id)`, 204 |

To expose fewer operations, list them:
`@ApiResource("/products", {"operations": ["LIST", "READ"]})`. An unknown name
stops the app with the list of valid names. A controller route on the same
method and path replaces the generated one, so you can keep the generated
routes and write one by hand:

<!-- doctest:fragment context=script -->
```gb
@Controller("/products")
class ProductSearch {
    @Get("")
    func list(?string q): dict<string, any> {
        return {"query": q, "items": []};
    }
}
```

## Options

The second argument of `@ApiResource` is an options dict:

| Key | Default | Effect |
|-----|---------|--------|
| `operations` | all six | The operations to generate, by the names in the table above. |
| `readGroups` | `["read"]` | Groups used to serialize responses. |
| `writeGroups` | `["write"]` | Groups a request body may set; other fields are ignored. |

Fields without `@Groups` are read and written in every group. Tag fields the
server controls, such as `id` or timestamps, with read groups only. See
[Serialization groups](07-serialization-groups.md).

## The repository interface

`gebweb.repository.Repository<T>` has four methods:

| Method | Called for | Notes |
|--------|------------|-------|
| `find(string id): ?T` | read, replace, update | Return null for an unknown id, including one that is not a valid id at all. |
| `list(Page page): list<T>` | list | `page.offset`, `page.size`, `page.sort`, `page.direction`. |
| `save(T entity): T` | create, replace, update | Insert when the entity is new, update otherwise; return it with its id. |
| `delete(string id): void` | delete | Deleting an unknown id is not an error. |

Ids arrive as strings from the path, so a repository with integer keys checks
and converts them, as `ProductRepository` does with `isInt` and `toInt`.

Gebweb also uses these methods when the repository has them:

| Method | Effect |
|--------|--------|
| `count(): int` | Adds `total` to unfiltered list responses. |
| `findBy(dict<string, string> criteria): list<T>` | Handles `?filter=field:value` (repeat for several fields). Gebweb pages the matches by `offset` and `limit` and reports their number as `total`. Only honour fields you expect, as `findBy` above does. |
| `listCursor(Cursor cursor): CursorPage<T>` | Switches listing to cursor pagination, below. |

## Paging

A list request takes `offset` (default 0), `limit` (default 25, at most 100),
`sort` and `direction` (`asc` or `desc`). A `limit` outside 1 to 100, a
negative `offset` or a non-number is a 400, so a client cannot ask for an
unbounded page. The response is
`{"items": [...], "offset": 0, "size": 25, "total": 3}`, with `total` present
when the repository has `count` or the request was filtered.

### Cursor pagination

Offset paging slows down on large tables because the database still walks
every skipped row. A repository with `listCursor` gets keyset paging instead:
each response carries an opaque `nextCursor`, and the client sends it back as
`?after=` for the next page.

<!-- doctest:fragment context=script -->
```gb
import gebweb.repository as repository;

class Product {
    int id;
    string name;
}

class ProductRepository {
    db.Connection conn;

    func ProductRepository(db.Connection conn) {
        this.conn = conn;
    }

    func listCursor(repository.Cursor cursor): repository.CursorPage<Product> {
        int after = cursor.after == "" ? 0 : repository.decodeCursor(cursor.after).toInt();
        let rows = this.conn.query("SELECT id, name FROM products WHERE id > ? ORDER BY id LIMIT ?", after, cursor.size + 1).all();
        list<Product> items = [];
        for (row in rows.slice(0, rows.length() < cursor.size ? rows.length() : cursor.size)) {
            let p = Product();
            p.id = row["id"] as int;
            p.name = row["name"] as string;
            items.push(p);
        }
        ?string next = rows.length() > cursor.size ? repository.encodeCursor(items[items.length() - 1].id as string) : null;
        return repository.CursorPage<Product>(items, next);
    }
}
```

The response is `{"items": [...], "nextCursor": "..."}`, with `nextCursor`
null on the last page. `limit` sets `cursor.size` as before; `offset`,
`direction` and `count` are not used. A filtered request still goes to
`findBy`. The repository decodes and validates the cursor, and clients must
treat it as opaque. The [cursor resources example](examples/cursor_resources.html)
shows validation of a malformed cursor.

## Repositories without `@ApiResource`

Most repositories serve ordinary controllers, as in
[Getting started](01-getting-started.md#a-repository). They need not implement
`Repository<T>`: give them the methods your controllers and services need,
register the instance in `build`, and inject it. Use `@ApiResource` when the
generated REST contract is what you want, and controllers when the routes do
more than store and fetch, such as checking ownership, triggering emails or
spanning several entities.

## Query builder

`gebweb.query` builds parameterised SQL without string concatenation. A chain
starts from `Query(table)` and ends with `select(columns)`, `count()` or
`delete()`, which return `{"text": sql, "params": values}`:

<!-- doctest:run -->
```gb
import gebweb.query as query;
import io;

let stmt = query.Query("users")
    .where(query.eq("status", "active"))
    .where(query.gt("score", 10).or(query.in_("role", ["admin", "owner"])))
    .orderBy(query.desc("created_at"))
    .limit(20)
    .select(["id", "name"]);
io.println(stmt["text"]);
io.println(stmt["params"]);
```

Output:

```text
SELECT "id", "name" FROM "users" WHERE ("status" = ? AND ("score" > ? OR "role" IN (?, ?))) ORDER BY "created_at" DESC LIMIT 20
["active", 10, "admin", "owner"]
```

Pass the result to the connection as `conn.query(stmt["text"], stmt["params"])`.
Values always travel as parameters. Column names are quoted, but they still
must come from your code or a whitelist, not straight from a request.

| Builder | SQL |
|---------|-----|
| `eq`, `neq`, `gt`, `ge`, `lt`, `le` | `=`, `<>`, `>`, `>=`, `<`, `<=` |
| `like(field, pattern)` | `LIKE` |
| `in_(field, values)` | `IN (...)`; an empty list matches nothing |
| `isNull(field)`, `notNull(field)` | `IS NULL`, `IS NOT NULL` |
| `raw(sql, params)` | a literal fragment with its own parameters |
| `.and(other)`, `.or(other)`, `.not()` | combine conditions |
| `asc(field)`, `desc(field)`, `.then(order)` | ordering |
| `.limit(n)`, `.offset(n)` | paging |

The builder has no joins or subqueries; write those queries as SQL.

## Soft delete

Soft delete keeps deleted rows for audit or restore, marking them with a
nullable `deleted_at` Unix time. The helpers set and clear the field and scope
queries:

| Helper | Effect |
|--------|--------|
| `gebweb.markDeleted(entity)` | sets `entity.deleted_at` to now |
| `gebweb.restore(entity)` | sets `entity.deleted_at` to null |
| `gebweb.isDeleted(entity)` | whether it is set |
| `gebweb.excludeDeleted(query)` | adds `deleted_at IS NULL` |
| `gebweb.onlyTrashed(query)` | adds `deleted_at IS NOT NULL` |
| `gebweb.withTrashed(query)` | adds nothing; states the intent |

<!-- doctest:fragment context=script -->
```gb
import gebweb.query as query;

class Note {
    int id;
    ?int deleted_at;
}

class NoteRepository {
    db.Connection conn;

    func NoteRepository(db.Connection conn) {
        this.conn = conn;
    }

    func visible(): list<dict<string, any>> {
        let stmt = gebweb.excludeDeleted(query.Query("notes")).orderBy(query.asc("id")).select([]);
        return this.conn.query(stmt["text"] as string, stmt["params"]).all();
    }

    func softDelete(Note note): void {
        gebweb.markDeleted(note);
        this.conn.exec("UPDATE notes SET deleted_at = ? WHERE id = ?", note.deleted_at, note.id);
    }
}
```

Every query that should hide deleted rows must use `excludeDeleted`,
including `find`, so keep them in the repository.

## Testing

Tests build the app with an in-memory database and the real repository, so
the SQL is exercised too; see [Testing](14-testing.md). To isolate a
controller from storage, register a hand-written stub with
`gebweb.registerInstance(app, ProductRepository, stub)` before the first
request.

## Reference

- `@ApiResource(path, options = {})` on an entity class with
  `static func repositoryClass(): any`.
- `gebweb.repository.Repository<T>`: `find`, `list`, `save`, `delete`;
  optional `count`, `findBy`, `listCursor`.
- `gebweb.repository.Page`, `Cursor`, `CursorPage<T>(items, nextCursor)`,
  `encodeCursor(text)`, `decodeCursor(cursor)`, `MAX_PAGE_SIZE` (100).
- `gebweb.query`: `Query(table)`, condition and order builders above.
- Soft delete: `gebweb.markDeleted`, `restore`, `isDeleted`,
  `excludeDeleted`, `onlyTrashed`, `withTrashed`.

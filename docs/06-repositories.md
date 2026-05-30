# Repositories and `@ApiResource`

The repository pattern abstracts over a persistence backend without
hauling in a full ORM. Combined with `@ApiResource`, a single class
declaration generates the six standard CRUD routes for an entity.

## The `Repository<T>` interface

```gb
import gebweb.repository as repository;

interface Repository<T> {
    func find(string id): ?T;
    func list(repository.Page page): list<T>;
    func save(T entity): T;
    func delete(string id): void;
}
```

Repositories are user-implemented:

```gb
class WidgetRepo implements repository.Repository<Widget> {
    dict<string, Widget> store;

    func WidgetRepo() {
        this.store = {};
    }

    func find(string id): ?Widget {
        if (this.store.contains(id)) { return this.store[id]; }
        return null;
    }

    func list(repository.Page page): list<Widget> {
        let keys = this.store.keys();
        list<Widget> out = [];
        for (i in page.offset..<(page.offset + page.size)) {
            if (i >= keys.length()) { break; }
            out = out.push(this.store[keys[i] as string]);
        }
        return out;
    }

    func save(Widget e): Widget {
        if (e.id == null || e.id == "") {
            e.id = "w" + ((this.store.keys().length() + 1) as string);
        }
        this.store[e.id] = e;
        return e;
    }

    func delete(string id): void {
        if (this.store.contains(id)) { this.store.delete(id); }
    }
}
```

Pagination uses `repository.Page` (`offset`, `size`, `sort`,
`direction`). Build pages with `gebweb.page(offset, size, sort,
direction)`.

## Optional repository methods

The framework probes the repository instance for these and uses them
when present:

- `count(): int` - total count, surfaces as `total` in the
  paginated-list response.
- `findBy(criteria: dict<string, any>): list<T>` - filtered list,
  used when the request carries a `?filter=field:value` query.

## `@ApiResource`

Decorate an entity class with `@ApiResource` and add a static
`repositoryClass()` method pointing at the repo type. The framework
generates six routes:

```gb
@ApiResource("/widgets")
class Widget {
    string id;
    string name;
    int weight;

    static func repositoryClass(): any { return WidgetRepo; }
}

let app = gebweb.app([Widget]);
gebweb.register(app, WidgetRepo, func(): WidgetRepo {
    return WidgetRepo();
});
```

| Operation | Method | Path                     |
|-----------|--------|--------------------------|
| LIST      | GET    | `/widgets`               |
| CREATE    | POST   | `/widgets`               |
| GET_ONE   | GET    | `/widgets/{id}`          |
| REPLACE   | PUT    | `/widgets/{id}`          |
| UPDATE    | PATCH  | `/widgets/{id}`          |
| DELETE    | DELETE | `/widgets/{id}`          |

A user-declared `@Get`/`@Post`/... on the same path overrides the
auto-generated route, so you can keep auto-CRUD for five operations
and hand-write the sixth.

## `@ApiResource` options

Pass a second-arg options dict to scope the operations or attach
serialization groups:

```gb
@ApiResource("/widgets", {
    "operations": ["GET", "GET_ONE"],
    "readGroups": ["read", "summary"],
    "writeGroups": ["write"],
})
class Widget { /* ... */ }
```

- `operations` - whitelist of CRUD operations to expose (default:
  all six).
- `readGroups` / `writeGroups` - default serialization groups for the
  resource (see [Serialization groups](07-serialization-groups.md)).

## DI container

Repositories are constructed through the same DI container that
wires controllers and the rest of the application. The
`gebweb.register(app, classRef, factory)` shorthand is the
typical place to register a repository factory, and
`gebweb.registerInstance(app, classRef, repo)` swaps in a stub
for tests:

```gb
let repo = WidgetRepo();
let app = gebweb.app([Widget]);
gebweb.registerInstance(app, WidgetRepo, repo);
```

See [Dependency injection](06a-dependency-injection.md) for the
full container surface: lifecycle model, registration methods,
constructor autowiring rules, the `@Param("key")` decorator,
interface-typed parameters, and testing patterns.

## Query DSL

Hand-rolling SQL inside a repository is fine for small CRUD, but
gets noisy fast. The `gebweb.query` module (re-exported on the
facade) builds parameterised SQL via composable `Where` and
`OrderBy` builders and a `Query` chain.

```gb
import gebweb;

class UserRepo {
    db.Connection conn;

    func findActive(int limit): list<dict<string, any>> {
        let sql = gebweb.Query("users")
            .where(gebweb.eq("status", "active"))
            .where(gebweb.gt("score", 10))
            .orderBy(gebweb.desc("created_at"))
            .limit(limit)
            .select(["id", "name", "score", "created_at"]);
        return this.conn.query(sql["text"], sql["params"] as list<any>).all();
    }
}
```

Predicate factories: `eq`, `neq`, `gt`, `ge`, `lt`, `le`, `like`,
`in_`, `isNull`, `notNull`, `raw(sql, params)`. Compose with
`.and(other)`, `.or(other)`, `.not()`. Ordering: `asc(field)` /
`desc(field)`, chain with `.then(asc(...))`.

The terminal `.select(cols)` / `.count()` / `.delete()` calls
return a `{text: string, params: list<any>}` dict that you hand
straight to `conn.query(text, params)` or `conn.exec(text,
params)`. Values are *always* sent as parameters; the DSL never
splices user input into the SQL string.

`in_([])` compiles to `(1 = 0)` so an empty filter set returns
zero rows rather than syntax-erroring. The DSL is intentionally
small (no joins, no subqueries) - it covers ~80% of repository
queries; everything else stays in hand-written SQL via
`.raw(sql, params)` or by calling `conn.query` directly.

## Reference

- Repository interface: `gebweb.repository.Repository<T>` with
  `find(id)`, `list(page)`, `save(entity)`, `delete(id)`.
- Optional repo methods: `count(): int`,
  `findBy(criteria): list<T>`.
- `gebweb.page(offset, size, sort, direction): Page` - build a Page.
- `@ApiResource(path, opts?)` - decorate the entity class.
- `static func repositoryClass(): any` - required by `@ApiResource`.
- Query DSL: `gebweb.eq` / `neq` / `gt` / `ge` / `lt` / `le` /
  `like` / `in_` / `isNull` / `notNull`; `gebweb.asc` / `desc`;
  `gebweb.Query(table).where(...).orderBy(...).limit(n).offset(m).select(cols)`,
  `.count()`, `.delete()`.
- DI register / resolve: see
  [Dependency injection](06a-dependency-injection.md) for the
  full reference.

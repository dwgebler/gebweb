# Routing and decorators

<!-- doctest:strict -->

A route is a controller method with an HTTP-method decorator. Its full path is
built from up to three parts, outermost first:

1. the API version from `@ApiVersion("v2")`, if any (`/v2`);
2. the controller prefix from `@Controller("/users")` (`/users`);
3. the method's own path from `@Get("/{id}")` (`/{id}`).

So `GET /v2/users/{id}` reaches the method above. How the controller is
created and mounted is covered in
[App construction](01-getting-started.md#app-construction).

## A resource controller

A typical JSON resource has one controller with a route per operation. The
example below uses an in-memory store so it runs on its own; a real app keeps
the data in a repository, as in [Getting started](01-getting-started.md).

<!-- doctest:run -->
```gb
import gebweb;
import io;
import store;

class Book {
    int id;
    string title;

    func Book(int id, string title) {
        this.id = id;
        this.title = title;
    }
}

class BookInput {
    @Assert.notBlank
    string title;
}

@Service
class BookStore {
    store.Store books = store.Store();

    func all(): list<Book> {
        list<Book> out = [];
        for (key in this.books.keys().sorted()) {
            out.push(this.books.get(key) as Book);
        }
        return out;
    }

    func find(int id): ?Book {
        return this.books.get(id) as ?Book;
    }

    func save(Book book): Book {
        this.books.set(book.id, book);
        return book;
    }

    func remove(int id): bool {
        return this.books.delete(id);
    }
}

@Controller("/books")
class BookController extends gebweb.Controller {
    BookStore books;

    func BookController(BookStore books) {
        parent();
        this.books = books;
    }

    @Get("")
    func list(): list<Book> {
        return this.books.all();
    }

    @Get("/{id}")
    func show(int id): Book {
        let found = this.books.find(id);
        if (found == null) {
            this.notFound("no book ${id}");
        }
        return found as Book;
    }

    @Put("/{id}")
    func replace(int id, @Body BookInput input): Book {
        return this.books.save(Book(id, input.title));
    }

    @Delete("/{id}")
    func remove(int id): gebweb.Response {
        if (!this.books.remove(id)) {
            this.notFound("no book ${id}");
        }
        return this.noContent();
    }
}

let app = gebweb.app();
io.print(gebweb.formatRouteTable(gebweb.routes(app)));

let client = gebweb.TestClient(app);
io.println(client.put("/books/1", {"title": "Dune"}).text());
io.println(client.get("/books").text());
io.println(client.delete("/books/1").status);
io.println(client.get("/books/1").status);
```

Output:

```text
METHOD  PATH                          HANDLER
GET     /books                        BookController.list
DELETE  /books/{id}                   BookController.remove
PUT     /books/{id}                   BookController.replace
GET     /books/{id}                   BookController.show
{"id":1,"title":"Dune"}
[{"id":1,"title":"Dune"}]
204
404
```

`gebweb.routes(app)` returns the route table, running discovery first, and
`gebweb.formatRouteTable` formats it; `gebweb routes` prints the same table
for a whole app. Discovery builds the controllers, so register their
dependencies before calling `routes`.

## HTTP method decorators

| Decorator | Method |
|-----------|--------|
| `@Get(path)` | GET, and HEAD |
| `@Post(path)` | POST |
| `@Put(path)` | PUT |
| `@Patch(path)` | PATCH |
| `@Delete(path)` | DELETE |
| `@Options(path)` | OPTIONS |
| `@Route(method, path)` | any method, including nonstandard ones such as `PURGE` |
| `@Route([methods], path)` | each method in the list |

A `@Get` route also answers HEAD requests: the server sends the same status
and headers without a body. `TestClient` returns the body for a HEAD request
too, so assert on status and headers there.

The decorators can be written module-qualified, such as `@gebweb.Get`, which
helps when another imported module defines its own `Get` decorator.

### One handler for several methods

Give `@Route` a list of methods to serve them all from one handler. Each
method becomes its own route and its own OpenAPI operation. Method names are
case-insensitive. Stacking route decorators, such as `@Get` and `@Post` on one
method, does the same.

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/reports")
class ReportController {
    @Route(["GET", "POST"], "/search")
    func search(gebweb.Request req, ?string q): dict<string, any> {
        string term = q ?? ((req.json() as dict<string, any>)["q"] as string);
        return {"method": req.method(), "term": term};
    }
}

let client = gebweb.TestClient(gebweb.app());
io.println(client.get("/reports/search?q=sales").text());
io.println(client.post("/reports/search", {"q": "returns"}).text());
io.println(client.request("PUT", "/reports/search", null, {}).status);
```

Output:

```text
{"method":"GET","term":"sales"}
{"method":"POST","term":"returns"}
404
```

A request with a method the route does not list gets a 404, as if the path
were unknown.

## Path parameters

A `{name}` segment captures one path segment. Declare a handler parameter with
the same name and Gebweb converts the segment to the parameter's type:

<!-- doctest:fragment context=controller -->
```gb
@Get("/authors/{authorId}/books/{slug}")
func book(int authorId, string slug): dict<string, any> {
    return {"author": authorId, "slug": slug};
}
```

A segment that does not convert gets a 400 Problem Details response, such as
`path parameter 'authorId' expects int, got 'abc'`, before the handler runs.
To use a different parameter name than the segment, mark it
`@PathParam("authorId") int id`; see
[Parameter binding](03-parameter-binding.md#explicit-parameter-decorators).

A fixed segment takes precedence over a parameter in the same position, so
`/users/new` and `/users/{id}` can be declared together, in any order and in
any controllers:

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/users")
class UserController {
    @Get("/{id}")
    func show(int id): dict<string, any> {
        return {"user": id};
    }

    @Get("/new")
    func newForm(): dict<string, any> {
        return {"form": "new user"};
    }
}

let client = gebweb.TestClient(gebweb.app());
io.println(client.get("/users/new").text());
io.println(client.get("/users/42").text());
```

Output:

```text
{"form":"new user"}
{"user":42}
```

## Controller prefix

`@Controller(prefix)` marks a class for discovery and prepends `prefix` to
each of its routes. `@Controller` with no argument mounts the routes as
written. Use `@Get("")` for the collection itself, so the route is `/books`
rather than `/books/`; requests match with or without a trailing slash either
way.

## Inherited routes

A controller inherits its parent's decorated methods, so shared routes can
live in a base class. Overriding a method replaces the inherited route along
with the method; decorate the override to keep serving it:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class StatusController extends gebweb.Controller {
    @Get("/ping")
    func ping(): string {
        return "pong";
    }

    @Get("/version")
    func version(): string {
        return "1.0";
    }
}

@Controller("/billing")
class BillingController extends StatusController {
    @Get("/version")
    func version(): string {
        return "2.3";
    }
}

@Controller("/search")
class SearchController extends StatusController {
    func version(): string {
        return "internal";
    }
}

let client = gebweb.TestClient(gebweb.app());
for (path in ["/billing/ping", "/billing/version", "/search/ping", "/search/version"]) {
    let r = client.get(path);
    io.println("${path} ${r.status} ${r.text()}");
}
```

Output:

```text
/billing/ping 200 "pong"
/billing/version 200 "2.3"
/search/ping 200 "pong"
/search/version 404 not found
```

`StatusController` has no `@Controller`, so it is not mounted itself; only its
subclasses are.

## API versioning and deprecation

`@ApiVersion("v2")` on a controller prefixes each of its routes with `/v2`
and tags them `v2` in the OpenAPI document. A method's own `@ApiVersion`
overrides the controller's, so one controller can keep an old route beside
the current ones. The label is used as the path segment as written; a leading
`/` is optional.

`@Deprecated` on a method or a controller marks its routes deprecated in
OpenAPI and adds a `Deprecation: true` response header. Give it a date,
`@Deprecated("2026-12-31T23:59:59Z")`, to also send a `Sunset` header
(RFC 8594) and an `x-sunset` OpenAPI field. The date is passed through as
written, so use an RFC 3339 or HTTP-date value.

<!-- doctest:run -->
```gb
import gebweb;
import io;

@ApiVersion("v2")
@Controller("/orders")
class OrderController {
    @Get("")
    func list(): list<string> {
        return ["o-1"];
    }

    @Get("/export")
    @ApiVersion("v1")
    @Deprecated("2026-12-31T23:59:59Z")
    func export(): list<string> {
        return ["o-1"];
    }
}

let client = gebweb.TestClient(gebweb.app());
io.println(client.get("/v2/orders").status);
let old = client.get("/v1/orders/export");
io.println("${old.status} Deprecation=${old.headers["Deprecation"]}");
io.println("Sunset=${old.headers["Sunset"]}");
```

Output:

```text
200
200 Deprecation=true
Sunset=2026-12-31T23:59:59Z
```

## OpenAPI metadata

These decorators change only the generated OpenAPI document, not dispatch:

| Decorator | Effect |
|-----------|--------|
| `@Summary("text")` | One-line operation summary, shown as the title in SwaggerUI. |
| `@Description("text")` | Longer operation description. |
| `@Tag("name")` | Operation tag; on a controller it applies to every route. The default is the class name without its `Controller` suffix. |
| `@OperationId("id")` | The operation's `operationId`. Controller routes have none unless you set one; client generators use it for method names. |
| `@ApiResponse(status, "description", schema: Class)` | An extra documented response, such as a 404. |

See [OpenAPI and SwaggerUI](13-openapi.md) for request and response schemas,
security schemes and the documentation endpoints.

## Reference

- Route decorators: `@Get(path)`, `@Post(path)`, `@Put(path)`,
  `@Patch(path)`, `@Delete(path)`, `@Options(path)`,
  `@Route(method, path)`, `@Route([methods], path)`; module-qualified forms
  such as `@gebweb.Get`.
- Controller prefix and discovery: `@Controller(prefix = "")` on the class.
- Path parameters: `{name}` in the path, bound to the handler parameter of the
  same name or to `@PathParam("name")`.
- Versioning: `@ApiVersion(label)` on a controller or method.
- Deprecation: `@Deprecated` or `@Deprecated(sunsetDate)` on a controller or
  method.
- Metadata: `@Summary(text)`, `@Description(text)`, `@Tag(name)`,
  `@OperationId(id)`, `@ApiResponse(status, description, ...)`.
- `gebweb.routes(app): list<dict<string, any>>` and
  `gebweb.formatRouteTable(list<dict<string, any>> routes): string`.

# AGENTS.md: Gebweb for AI agents

This file is a dense, idiomatic cheatsheet for AI coding agents
working in Gebweb apps. Pair it with the Geblang `AGENTS.md`
(language basics: syntax, comments, types, classes) for full
context. This file alone is enough for editing tasks against
a Gebweb app.

## What Gebweb is

A typed, decorator-driven web framework for Geblang, modelled on
FastAPI / Symfony / API Platform. Wires HTTP routing, OpenAPI,
DI, auth, validation, repositories, background jobs, events,
messaging, mailers, storage, CSRF, sessions, view templates,
WebSockets, and an encrypted secrets vault. Apps are usually
small: a `main.gb`, a few controllers, optional `services.yaml`.

## The minimum app

```gb
import gebweb;

class HelloController {
    @Get("/")
    func index(): dict<string, any> {
        return {"hello": "world"};
    }
}

let app = gebweb.app([HelloController]);
gebweb.serve(app, "127.0.0.1:8080");
```

`gebweb.app(controllers)` constructs the app, walks every
controller class with reflection, and registers every method that
carries a routing decorator. `gebweb.serve(app, addr)` blocks on
the HTTP server; `gebweb.listen(app, addr)` is non-blocking.

## Routing

Decorate handler methods inside a controller class:

```gb
class UserController {
    @Get("/users")          func list(): list<User> { ... }
    @Get("/users/{id}")     func get(string id): User { ... }
    @Post("/users")         func create(UserDto body): User { ... }
    @Put("/users/{id}")     func replace(string id, UserDto body): User { ... }
    @Patch("/users/{id}")   func update(string id, UserDto body): User { ... }
    @Delete("/users/{id}")  func remove(string id): void { ... }
    @Route("HEAD", "/ping") func ping(): void { ... }
}

@Controller("/api")
class ApiController { # every route in here lands under /api
    @Get("/health")
    func health(): dict<string, any> { return {"ok": true}; }
}
```

Decorators recognised at routing time: `@Get`, `@Post`, `@Put`,
`@Patch`, `@Delete`, `@Options`, `@Route`, plus the parameter
binding decorators below. `@Controller("/prefix")` on a class
adds a path prefix to every route in it.

## Parameter binding

Handler parameters bind by name and type. The framework looks for
path params first, then query, then body, then headers:

```gb
@Get("/users/{id}")
func get(string id, ?int verbose): User { ... }
```

For explicit control, use the parameter decorators:

```gb
@Post("/users/{id}")
func update(
    @PathParam("id") string id,
    @QueryParam("upsert") bool upsert,
    @Body UserDto body,
    @Header("X-Trace-Id") ?string traceId,
): User { ... }
```

Body classes (DTOs) get JSON-parsed from the request body.
Validate them with `@Assert.*` decorators on the fields.

## Validation

```gb
class UserDto {
    @Assert.email
    string email;

    @Assert.minLength(8)
    string password;

    @Assert.range(18, 120)
    int age;
}
```

Built-in (names are exact and case-sensitive; an unknown name is
silently skipped, not an error): `email`, `url`, `uuid`,
`notBlank`, `minLength(n)`, `maxLength(n)`, `range(min, max)`,
`regex("re")`, `in(["a", "b"])`. Custom:
`gebweb.registerAssertion(app, "name", func(any v, list<any>
args, dict<string, any> named): ?string { ... })` returns null on
pass, error string on fail.

Validation failures become 422 Problem Details for JSON clients,
303 redirect with session-stashed input + errors for HTML
clients.

## Response shapes

```gb
return {"id": "u-1"};               # JSON 200
return null;                        # 204 No Content
return gebweb.html("<p>ok</p>");    # HTML 200
return {"status": 418, "body": "I'm a teapot",
        "headers": {"Content-Type": "text/plain"}};
return gebweb.file("./report.pdf", {"attachment": true});
return gebweb.stream(handler);
return this.redirect("/login", 303);    # controller helper (no gebweb.redirect)
```

Throw for error responses:

```gb
throw gebweb.notFound("user not found");
throw gebweb.badRequest("missing email");
throw gebweb.unprocessableEntity("validation failed", [...]);

import gebweb.errors as errors;
throw errors.HttpException(503, "database is down", "Service Unavailable");
```

Inside a controller class extending `gebweb.Controller`, use the
method short forms: `this.json(data, status)`, `this.text(body)`,
`this.created(data, location)`, `this.redirect(loc)`,
`this.notFound(detail)`, `this.problem(status, title, detail)`.

## Dependency injection

Constructor injection. Controllers, repositories, and discovered
services get autowired:

```gb
class Database {
    db.Connection conn;
    func Database(db.Connection conn) { this.conn = conn; }
}

class UserRepo {
    Database database;
    func UserRepo(Database database) { this.database = database; }
}

class UserController {
    UserRepo repo;
    func UserController(UserRepo repo) { this.repo = repo; }
}

let conn = db.connect("sqlite", "./app.db");
let app = gebweb.app([]);
gebweb.register(app, Database, func(): Database {
    return Database(conn);
});
gebweb.addControllers(app, [UserController]);
```

Lifecycle helpers:

- `gebweb.register(app, T, factory)` singleton.
- `gebweb.registerInstance(app, T, instance)` pre-built singleton.
- `gebweb.registerPerRequest(app, T, factory)` per-request scope.

Service ids and YAML services give a declarative alternative.
Create `config/services.yaml` next to your app entry point:

```yaml
services:
  UserRepo: {}
  app.summariser:
    class: Summariser
    args:
      client: "@AnthropicClient"

bindings:
  Notifier: "@EmailNotifier"
```

`@Service` on a class registers it under its name. `@Service("custom.id")`
uses a custom id. Look up by id with `gebweb.service(app, id)`.

## Auth

```gb
gebweb.useAuthenticator(app, CurrentUser, func(dict<string, any> req): ?any {
    # return a user value or null
});

@Auth                          # requires any authenticated user
@RequiresRole("admin", "owner")
@RequiresPermission("widgets.write")
class AdminController { ... }
```

Variants: `gebweb.useSessionAuth`, `gebweb.useApiKeyAuth`. The
authenticated user is injected into any handler parameter typed
with the user class.

## Middleware

```gb
gebweb.use(app, gebweb.cors({"allowOrigins": ["*"]}));
gebweb.before(app, gebweb.rateLimit({"rate": 100}));
gebweb.use(app, gebweb.securityHeaders({"csp": {...}}));
gebweb.use(app, gebweb.compress({}));
gebweb.use(app, gebweb.requestLog({}));
```

`before` runs early and can short-circuit by returning a response
dict. `use` shapes the response after the handler. Per-route
`@RateLimit(rps, burst)` and `@Cors({...})` decorate a single
handler.

## Caching

```gb
import web.cache as wc;

gebweb.useCacheStore(app, wc.fileCacheStore("/tmp/cache", 600));

class WidgetController {
    @Get("/widgets")
    @Cache(ttl: 60, vary: ["X-User"])
    func list(): list<Widget> { ... }
}
```

Manual invalidation: build the cache key with
`gebweb.cache.cacheKey(request, varyHeaders)` and call
`store.delete(key)`.

## Background jobs

```gb
class EmailSender {
    @Job("send-welcome")
    func send(any payload): void {
        let p = payload as dict<string, any>;
        sendEmail(p["to"] as string);
    }
}

let conn = db.connect("sqlite", "./app.db");
let app = gebweb.app([]);
gebweb.useJobs(app, conn);
gebweb.discover(app);              # picks up @Job classes
gebweb.enqueue(app, "send-welcome", {"to": "a@b.com"});
```

Run the worker in a separate process: `gebweb worker`.

## Events

```gb
gebweb.useEvents(app);

class UserCreated {
    @On("user.created")
    func handle(gebweb.Event ev): void {
        io.println(ev.payload["id"]);
    }
}

gebweb.discover(app);
gebweb.publish(app, "user.created", {"id": "u-1"});
```

Synchronous in-process. Subscriber errors aggregate into one
`RuntimeError`; the publisher catches it once.

## Repositories

```gb
@ApiResource("/users")
class User {
    string id;
    string email;
    static func repositoryClass(): any { return UserRepo; }
}

class UserRepo {
    func list(gebweb.Page page): gebweb.Page { ... }
    func find(string id): ?User { ... }
    func create(User u): User { ... }
    func update(string id, User u): ?User { ... }
    func delete(string id): bool { ... }
}
```

The framework generates `GET/POST/PUT/PATCH/DELETE /users` and
`GET /users/{id}` from the repo. Override any single route by
declaring a handler on the same path.

For cursor pagination, add `listCursor(c: Cursor): CursorPage<T>`.

## Templates

```gb
gebweb.useViews(app, "templates");

@Get("/page")
func page(dict<string, any> request): dict<string, any> {
    return gebweb.htmlView(app, request, "page.html", {"name": "Ada"});
}
```

Twig-style syntax: `{{ var }}`, `{% if %}`, `{% for %}`,
`{% extends "..." %}`, `{% block %}`, `{% include "..." %}`,
`{{ x | filter }}`. Built-in filters cover escape/raw, upper/
lower, length, default, json, date, replace, join, split,
first/last, abs, round.

`gebweb.viewsFilter(app, "name", fn)` registers a custom filter.
`gebweb.registerViewContext(app, "key", fn)` merges a value into
every rendered context.

## Testing

```gb
import test;
import gebweb;
import controllers;

class UserTest extends test.Test {
    gebweb.TestClient client;

    func setUp(): void {
        this.client = gebweb.TestClient(gebweb.app([UserController]));
    }

    @test
    func listsUsers(): void {
        let r = this.client.get("/users");
        r.assertStatus(200);
        this.assertEquals(0, (r.json() as list<any>).length());
    }
}
```

`TestClient` dispatches requests in-process. `r.assertStatus`,
`r.json()`, `r.text()`, `r.headers` for assertions.

For session-backed flows, use `client.request("GET", "/me", null,
{"Cookie": "geb_sid=..."})`.

## CLI

```
gebweb new myapp
gebweb dev                    # hot-reload
gebweb routes                 # print routes table
gebweb build                  # release binary
gebweb generate controller User
gebweb migrate create | up | down | status
gebweb worker                 # background jobs + messaging
gebweb secrets init | edit | set | get | list
```

## Idioms

- **One controller per resource**. Put related routes in the same
  class so `@Prefix`, `@Auth`, and `@RequiresRole` apply
  uniformly.
- **DTOs as classes**, not dicts. Lets you declare validation
  decorators and gets type-checked at the boundary.
- **Throw `HttpException` early**. Don't return a 4xx dict by
  hand; the thrown exception goes through the Problem Details
  renderer.
- **Wire infrastructure in main.gb**. Keep controller code
  framework-agnostic by accepting collaborators via the
  constructor and registering them once at startup.
- **Use `gebweb.discover(app)`** when you want `@Service`,
  `@Job`, `@On`, `@OnMessage`, `@Scheduled`, `@Tag` classes to
  auto-register from a `reflect.classes()` sweep.

## Anti-idioms

- Don't construct response dicts with arbitrary keys. The
  framework expects `{"status", "headers", "body"}`. Other keys
  are ignored.
- Don't put framework logic in DTOs. Their only job is field
  declarations and validation decorators.
- Don't read `request["headers"]` directly when a parameter
  decorator (`@Header("name")`) would do the job.
- Don't reach for the DI container from inside a handler. Inject
  the dependency at construction time instead.
- Don't store mutable state on the app value. Use the parameter
  store, the cache, or the session, depending on the scope.

## When in doubt

Write a TestClient-driven test and inspect the response shape:

```gb
let client = gebweb.TestClient(gebweb.app([MyController]));
let r = client.get("/some/path");
io.println(r.headers);
io.println(r.json());
```

That's the fastest way to confirm framework behaviour for a
specific scenario without spinning up a server.

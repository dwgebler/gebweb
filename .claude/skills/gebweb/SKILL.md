---
name: gebweb
description: >-
  Building, running, testing, and deploying web applications with Gebweb, the
  Geblang web framework. Use this whenever you write or modify a Gebweb app -
  controllers, routes, request/parameter binding, views/templates, validation,
  repositories, auth, middleware, background jobs, websockets - or operate the
  `gebweb` CLI (new / dev / build / routes / generate / migrate / worker /
  secrets). Trigger it on mentions of Gebweb, `import gebweb`, route decorators
  (`@Get`, `@Post`, `@Controller`, `@ApiResource`, `@Auth`), `gebweb.app`,
  `gebweb.serve`, the `TestClient`, or `.gb` web code. Gebweb has framework-
  specific conventions (exact `@Assert.*` names, `repositoryClass()`,
  `{name}` path params, shared-state concurrency rules) that are easy to get
  wrong - use this skill to stay correct. Pair it with the `geblang` skill,
  which covers the underlying language.
---

# Gebweb

Gebweb is the web framework for Geblang: a decorator-driven, dependency-injected
MVC framework for building HTTP APIs and server-rendered apps. An app is a set of
controller classes whose methods are routes; the framework wires routing,
parameter binding, DI, validation, serialization, auth, and OpenAPI around them.

**This skill assumes the `geblang` skill** for the underlying language (syntax,
types, the `geblang` CLI, the gotchas like `//` integer division and `parent()`).
Read that first for anything language-level; this skill covers only the
framework. The `gebweb` CLI shells out to the `geblang` binary, so both must be
on `$PATH`.

## Ground before you write

The framework surface is large and versioned. The reference files here are a
map; confirm exact signatures against the installed framework:

- `geblang doc <gebweb-src-dir>` prints signatures from the framework source.
- `gebweb help <command>` documents a CLI command.
- `gebweb routes` prints the live route table for an app (verify your routing).
- The framework's own chapter docs (`gebweb/docs/`) are the prose reference.

Do not assume a decorator or facade name from another framework (Symfony,
Laravel, Spring, FastAPI). Gebweb's names are specific - verify, then write.

## The app in one glance

```gb
module main;
import gebweb;
import http;

export class CreateUser {
    @Assert.notBlank
    string name;
}

@Service
export class UserService {
    func find(int id): ?dict<string, any> {
        return id == 1 ? {"id": 1, "name": "Ada"} : null;
    }

    func add(CreateUser dto): dict<string, any> {
        return {"id": 2, "name": dto.name};
    }
}

@Controller("/users")
export class UserController extends gebweb.Controller {
    UserService users;

    func UserController(UserService users) {   # constructor injection
        parent();
        this.users = users;
    }

    @Get("/{id}")                                # {id} binds to the int param
    func show(int id): http.Response {
        let user = this.users.find(id);
        if (user == null) {
            this.notFound("no user ${id}");      # throws a 404; returns void
        }
        return this.json(user);
    }

    @Post("")                                    # "" = the prefix itself; "/" adds a trailing slash
    func create(CreateUser body): http.Response {  # user-class param = JSON body, validated
        let user = this.users.add(body);
        return this.created(user, "/users/${user["id"]}");
    }
}

export func main(list<string> args): int {
    gebweb.cli(gebweb.app(), {"name": "users", "port": "8080"});  # cli returns void
    return 0;
}
```

An invalid body never reaches `create`: it becomes a 422 Problem Details
response. Classes a test imports must be `export`ed.

## Discovery and lifetimes

The entrypoint's import graph determines loaded classes. Gebweb walks
`reflect.classes()` and reads stored decorator/constructor metadata; it does
not scan directories or inspect unloaded files. Import component modules before
starting dispatch. Reflection needs no instance, but importing a module can run
its top-level code. Discovery then constructs controllers/tagged handlers via DI.

`app()` auto-mounts loaded controllers; an explicit controller list limits that
set. Discovery runs once per app. Concrete class constructor dependencies can
autowire without `@Service`; the marker registers a named service. A unique
registered interface implementation can be selected; ambiguity needs a binding.

Controllers and ordinary services are shared across requests. Prefer stateless
services, Store/atomic operations or durable storage. Store operations are
individually safe and copy values; a sequence of operations is not a transaction.
Discovery caching is opt-in and fingerprints class names, not decorator values;
remove its manifest after decorator-only changes. It does not replace imports.

## CLI workflow

| Command | Use |
|---|---|
| `gebweb new <name> [--type app\|api] [--db sqlite\|postgres\|mysql\|pgvector]` | Scaffold a project; its `geblang.yaml` already depends on gebweb, so run `geblang install` next. |
| `gebweb dev [--entry src/main.gb]` | Hot-reload dev server (watches `src/`). |
| `gebweb build [--out build/app] [--docker]` | Bundle a self-contained binary (embeds templates/assets). |
| `gebweb routes` | Print the route table without serving (apps using `gebweb.cli` or `gebweb.serve`). |
| `gebweb generate <controller\|dto\|repository\|resource> <Name>` | Scaffold boilerplate. Also `generate client <spec> <Name>`. |
| `gebweb migrate <create\|up\|down\|status>` | Versioned SQL migrations against `DATABASE_URL`. |
| `gebweb worker [--entry app.worker] [--job n] [--handle n]` | Run the job + messaging worker (`worker dlq` for dead letters). |
| `gebweb secrets <init\|edit\|set\|get\|list>` | Manage the encrypted secrets vault. |
| `gebweb docker` | Generate Dockerfile + compose.yaml. |

Standard loop: `gebweb new` -> `geblang install` -> write controllers ->
`gebweb dev` -> `gebweb routes` to confirm wiring -> `geblang test src/` ->
`gebweb build`. In an existing project, add the framework with
`geblang install github.com/dwgebler/gebweb@v<version>`.

## Critical gotchas (framework-specific)

1. **`@Assert.*` names are case-sensitive registry keys.** Startup checks DTOs
   bound by routes/resources and nested `@Valid` fields; unknown names throw
   UnknownAssertionError. Use the current validation manual for names. Presence
   constraints such as `notBlank` are needed because many other rules skip null.
2. **`@ApiResource` requires a static `repositoryClass()`** on the decorated
   class returning the repository class - not `repository()`. A missing one
   throws at startup.
3. **There is no `gebweb.redirect(...)`.** Redirect with the controller method
   `this.redirect(location, status)` or `http.redirect(url, status)`.
4. **`gebweb.view` arg order is `(app, name, ctx, request?)`** - `request` is the
   optional 4th argument. In a controller prefer `this.view(request, name, ctx)`.
5. **Concurrency: requests run on separate goroutines and controllers/services/
   singletons are shared.** Never mutate a plain shared `dict`/`list`/`set`/
   object across requests - use a `store.Store` singleton or
   `gebweb.registerPerRequest`. Unsynchronized shared mutation can crash the
   process. Read-only data shared by every request (a compiled config, lookup
   tables) can be built once, frozen with `freeze.deep` and kept in a
   `store.Store`, which hands deep-frozen values out without copying.
6. **Path parameters use `{name}`**, matched to a handler parameter of the same
   name (or `@PathParam("name")`).
7. **A handler returns an `http.Response`** (built via `this.json`/`this.html`/
   `this.view`/...) or a response dict whose contract is exactly
   `{"status", "headers", "body"}`. The error helpers (`this.notFound(detail)`,
   `badRequest`, `forbidden`, ...) THROW and return `void`: call them as a
   statement, never `return this.notFound()`. Each takes a detail string.
8. **`TestClient` does not drive live WebSocket/SSE/chunked streams** - use a
   real `gebweb.listen` server for wire-level streaming tests.
9. **`gebweb.jwt` / `gebweb.jwtIssue` / `jwtVerify` are HS256-only.** For
   RS/ES tokens use the language's `crypt.jwtSign` / `crypt.jwtVerify`, and
   publish public keys with `gebweb.useJwks(app, [...])`.
10. **Templates use `{% elseif %}`, not `elif`, and `{% for %}` takes ONE
    loop variable** (`{% for row in rows %}`; there is no `for k, v in d`).
    Pass lists of dicts from the controller for tables.
11. **Stub services before controllers are built.** `gebweb.app([Ctrl])` and
    `addControllers` construct controllers immediately, so register stubs
    first. With `gebweb.app()` (discovery), controllers are built on first
    dispatch: `let app = gebweb.app(); gebweb.registerInstance(app, Svc, fake);`
    then create the `TestClient`.
12. **Decorator arguments can be types and functions** (Geblang 1.44.0):
    `@ApiResponse(404, "Missing", schema: errors.Problem)` (with
    `import gebweb.errors as errors;`),
    `@Job("charge", retry: (int attempt) => attempt * 2000)`. Only on
    module-level classes, functions and their members.
13. **`@ApiResource` list `limit` must be 1..100 and `offset` >= 0**; other
    values get a 400. A repository with `listCursor(Cursor): CursorPage<T>`
    gets cursor pagination (`?after=...`) automatically.
14. **Run the framework's own Geblang test suite from the `gebweb/` directory**
    (relative fixture/i18n paths). Your own app's tests run from your app root.

## Reference files

- **`references/app-authoring.md`** - the full programming model: facade +
  entrypoint, controllers and helpers, routing + handler decorators, parameter
  binding, dependency injection, validation, repositories + `@ApiResource` +
  query DSL, serialization groups, auth + JWT, middleware, caching, views,
  websockets/SSE, jobs/events/scheduler/messaging, uploads, OpenAPI, i18n,
  multi-tenancy, and the rest of the facade.
- **`references/cli-and-testing.md`** - the full `gebweb` CLI reference, project
  layout, dev/build/deploy, migrations, workers and the DLQ, secrets, and
  in-process testing with `TestClient`.

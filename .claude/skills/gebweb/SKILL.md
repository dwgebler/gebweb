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
import gebweb;

@Controller("/users")
class UserController extends gebweb.Controller {
    func UserController(UserService users) { this.users = users; }   # DI

    @Get("/{id}")
    func show(int id): http.Response {                # {id} binds to the param
        let u = this.users.find(id);
        if (u == null) { return this.notFound(); }
        return this.json(u);
    }

    @Post("/")
    func create(@Body CreateUser dto): http.Response { # body -> typed DTO
        return this.created(this.users.add(dto), "/users/${dto.id}");
    }
}

export func main(list<string> args): int {
    let app = gebweb.app();                # no list = auto-discover @Controller classes
    return gebweb.cli(app, {"addr": ":8080"});
}
```

## CLI workflow

| Command | Use |
|---|---|
| `gebweb new <name> [--type app\|api] [--db sqlite\|postgres\|mysql\|pgvector]` | Scaffold a project (interactive wizard; flags skip prompts). |
| `gebweb dev [--entry src/main.gb]` | Hot-reload dev server (watches `src/`). |
| `gebweb build [--out build/app] [--docker]` | Bundle a self-contained binary (embeds templates/assets). |
| `gebweb routes` | Print the route table. |
| `gebweb generate <controller\|dto\|repository\|resource> <Name>` | Scaffold boilerplate. Also `generate client <spec> <Name>`. |
| `gebweb migrate <create\|up\|down\|status>` | Versioned SQL migrations against `DATABASE_URL`. |
| `gebweb worker [--job n] [--handle n] [--jobs-only] [--messaging-only]` | Run the job + messaging worker (`worker dlq` for dead letters). |
| `gebweb secrets <init\|edit\|set\|get\|list>` | Manage the encrypted secrets vault. |
| `gebweb docker` | Generate Dockerfile + compose.yaml. |

Standard loop: `gebweb new` -> write controllers -> `gebweb dev` -> `gebweb
routes` to confirm wiring -> `geblang test tests/` -> `gebweb build`.

## Critical gotchas (framework-specific)

1. **`@Assert.*` validator names are exact and silently no-op when unknown.**
   The resolver strips `Assert.` and looks the literal remainder up in a
   registry; a miss is skipped without error. Valid names (lowercase camelCase):
   `email`, `url`, `uuid`, `regex`, `minLength`, `maxLength`, `range`, `in`,
   `notBlank`. `@Assert.Email` / `@Assert.Length` / `@Assert.NotBlank` compile
   but validate NOTHING.
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
   process.
6. **Path parameters use `{name}`**, matched to a handler parameter of the same
   name (or `@PathParam("name")`).
7. **A handler returns an `http.Response`** (built via `this.json`/`this.html`/
   `this.view`/...) or a response dict whose contract is exactly
   `{"status", "headers", "body"}`.
8. **`TestClient` does not drive live WebSocket/SSE/chunked streams** - use a
   real `gebweb.listen` server for wire-level streaming tests.
9. **JWT helpers (`gebweb.jwt`) are HS256-only** in the 1.x line.
10. **Run the framework's own Geblang test suite from the `gebweb/` directory**
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

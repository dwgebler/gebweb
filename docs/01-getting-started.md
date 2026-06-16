# Getting started

Gebweb apps are built from controller classes: each method decorated
with a route annotation (`@Get`, `@Post`, ...) becomes an HTTP handler.
Mark a class `@Controller` and it is discovered and mounted
automatically; mark a class `@Service` and the DI container autowires
it into the controllers that depend on it. The framework wires every
route through a binding adapter and mounts SwaggerUI at `/docs` plus
the OpenAPI spec at `/openapi.json`.

## A first API

This small notes API shows the shape of a real Gebweb app: a service
holds the state, a controller exposes it, and nothing is registered by
hand. `gebweb.app()` is called with no controller list - the framework
discovers `NotesController` (it carries `@Controller`) and autowires
`NoteService` (it carries `@Service`) into its constructor.

```gb
import gebweb;
import http;

@Service
class NoteService {
    dict<string, dict<string, any>> notes;
    int counter;

    func NoteService() { this.notes = {}; this.counter = 0; }

    func add(string text): dict<string, any> {
        this.counter = this.counter + 1;
        let note = {"id": "n-${this.counter}", "text": text};
        this.notes[note["id"] as string] = note;
        return note;
    }

    func all(): list<dict<string, any>> {
        let out = [];
        for (id in this.notes.keys()) { out = out.push(this.notes[id]); }
        return out;
    }

    func find(string id): ?dict<string, any> {
        if (this.notes.contains(id)) { return this.notes[id]; }
        return null;
    }
}

class NoteInput {
    string text;
}

@Controller("/notes")
class NotesController extends gebweb.Controller {
    NoteService notes;

    func NotesController(NoteService notes) {
        this.notes = notes;
    }

    @Get("")
    @Summary("List notes")
    func index(): http.Response {
        return this.json(this.notes.all());
    }

    @Post("")
    @Summary("Create a note")
    func create(NoteInput body): http.Response {
        return this.json(this.notes.add(body.text), 201);
    }

    @Get("/{id}")
    @Summary("Fetch one note")
    func show(string id): http.Response {
        let note = this.notes.find(id);
        if (note == null) { throw gebweb.notFound("no note " + id); }
        return this.json(note);
    }
}

let app = gebweb.app();
gebweb.serve(app, "127.0.0.1:8080");
```

`POST /notes` with `{"text": "buy milk"}` returns `201` and the created
note; `GET /notes/n-1` returns it; `GET /notes/missing` returns a `404`
RFC 9457 problem document. Visit `/docs` for the auto-generated
SwaggerUI page describing every route.

## Request lifecycle

A request flows through:

1. **Routing.** `stdlib/web/router` matches method + path against the
   registered route table and resolves path parameters.
2. **Middleware (before).** Any request-phase callable registered via
   `gebweb.before` runs. Returning a response dict short-circuits.
3. **Cache lookup.** If the matched handler carries `@Cache`, the
   framework consults the registered cache store and returns the
   cached response on a hit.
4. **Authentication.** If the handler (or its enclosing controller
   class) carries `@Auth` or `@RequiresRole`, the registered
   authenticator runs. Failure becomes a 401 / 403.
5. **Parameter binding.** Path / query / body / file parameters are
   coerced into the handler's typed parameter list.
6. **Validation.** Any DTO bound from the request body is run
   through the registered `@Assert.*` validators.
7. **Handler.** Your method runs.
8. **Response shaping.** Dicts and lists become JSON; pre-shaped
   response dicts pass through; `gebweb.stream` / `@Sse` /
   `@WebSocket` are recognised by the lower-level dispatcher.
9. **Middleware (after).** Response-phase middleware (e.g.
   `cors`, `securityHeaders`, `compress`) transforms the response.

## App construction

`gebweb.app(controllers)` accepts three kinds of entry in the list:

- An already-constructed controller instance
  (`gebweb.app([HelloController()])`).
- A controller class reference; the DI container instantiates it
  with autowired dependencies (see
  [Dependency injection](06a-dependency-injection.md) for the
  rules and registration surface).
- A class decorated with `@ApiResource`; the framework generates
  auto-CRUD routes (see [Repositories](06-repositories.md)).

`gebweb.setInfo(app, info)` overrides the OpenAPI `info` object:

```gb
let app = gebweb.setInfo(gebweb.app([NotesController]), {
    "title": "Notes",
    "version": "0.1.0",
    "description": "A minimal Gebweb application.",
});
```

The `/openapi.json` (spec) and `/docs` (SwaggerUI) routes mount automatically.
An explicit `gebweb.app` option turns them off and always overrides the
environment: `{"docs": false}` disables both, `{"swaggerUi": false}` keeps the
spec but drops the UI, and `{"openapi": false}` disables both (the UI needs the
spec). `GEBWEB_DOCS=off` or `GEBWEB_ENV=production` disable them via the
environment; `gebweb.useDocsAuth(app, guard)` keeps them mounted behind auth.

## Serving

- `gebweb.cli(app, opts)` - the standard entrypoint for deployable
  apps. Resolves ports and TLS from command-line flags, `GEBWEB_*`
  environment variables, and the `opts` defaults (in that precedence),
  prints a "serving http://... - Ctrl+C to stop" banner, and shuts
  down cleanly on SIGINT / SIGTERM. Flags: `--port`, `--tls-port`,
  `--host`, `--domain` (LetsEncrypt autocert with an HTTP redirect
  listener), `--self-signed` (local HTTPS with a generated
  certificate), `--no-tls`, `--acme-email`, `--acme-cache`, `--help`.

```gb
gebweb.cli(app, {"name": "myapp", "port": 8080});
# full option/precedence/TLS/drain reference: docs/37-deployment.md
# ./myapp --port 9000
# ./myapp --self-signed              # https://localhost:443, generated cert
# ./myapp --domain example.com       # LetsEncrypt on :443, redirect on :80
```

- `gebweb.serve(app, addr)` - blocking, address-only. Use when the
  operational surface is managed elsewhere.
- `gebweb.listen(app, addr)` - non-blocking. Returns an `int` listen
  handle suitable for `http.shutdown(handle)`.
- `gebweb.dispatcher(app)` - returns the in-process `callable(request)
  : response` for direct invocation (used by `TestClient`).
- `gebweb.routes(app)` - returns the registered route table for
  introspection / debugging.

To serve several ports at once, call `gebweb.listen` for each (it is
non-blocking) and `http.wait` on a handle to keep the process alive - for
example plain HTTP and self-signed HTTPS from the same app:

```gb
gebweb.listen(app, "127.0.0.1:8080");
let server = gebweb.listen(app, "127.0.0.1:8443", {"tls": {"selfSigned": true}});
http.wait(server);
```

## Reference

- `gebweb.app(list<any> controllers = [], ?dict<string, any> opts = null): GebwebApp`
- `gebweb.setInfo(GebwebApp app, dict<string, any> info): GebwebApp`
- `gebweb.cli(GebwebApp app, dict<string, any> opts = {}): void`
- `gebweb.serve(GebwebApp app, string address, ?dict<string, any> opts = null): void`
- `gebweb.listen(GebwebApp app, string address, ?dict<string, any> opts = null): int`
- `gebweb.dispatcher(GebwebApp app): callable`
- `gebweb.routes(GebwebApp app): list<dict<string, any>>`

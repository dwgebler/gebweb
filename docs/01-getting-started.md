# Getting started

Gebweb apps are built from controller classes. Each method decorated
with a route annotation (`@Get`, `@Post`, ...) becomes an HTTP
handler. The framework reflects on the controllers at app-construction
time, wires every route through a binding adapter, and mounts
SwaggerUI at `/docs` plus the OpenAPI spec at `/openapi.json`.

## Hello world

```gb
import gebweb;

class HelloController {
    @Get("/")
    @Summary("Service banner")
    func index(): dict<string, any> {
        return {"service": "hello", "version": "0.1.0"};
    }

    @Get("/hello/{name}")
    @Summary("Greet someone")
    func hello(string name, ?string lang): dict<string, any> {
        string greeting = "Hello";
        if (lang != null && (lang as string).lower() == "fr") {
            greeting = "Bonjour";
        }
        return {"message": greeting + ", " + name + "!"};
    }
}

let app = gebweb.app([HelloController()]);
gebweb.serve(app, "127.0.0.1:8080");
```

Open `http://127.0.0.1:8080/hello/Ada?lang=fr` in a browser to see
`{"message": "Bonjour, Ada!"}`. Visit `/docs` for the auto-generated
SwaggerUI page describing both routes.

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
let app = gebweb.setInfo(gebweb.app([HelloController()]), {
    "title": "Hello",
    "version": "0.1.0",
    "description": "A minimal Gebweb application.",
});
```

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

## Reference

- `gebweb.app(list<any> controllers): GebwebApp`
- `gebweb.setInfo(GebwebApp app, dict<string, any> info): GebwebApp`
- `gebweb.cli(GebwebApp app, dict<string, any> opts = {}): void`
- `gebweb.serve(GebwebApp app, string address): void`
- `gebweb.listen(GebwebApp app, string address): int`
- `gebweb.dispatcher(GebwebApp app): callable`
- `gebweb.routes(GebwebApp app): list<dict<string, any>>`

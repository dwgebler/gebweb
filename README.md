# Gebweb

An opinionated, typed, decorator-driven web framework for
[Geblang](https://github.com/dwgebler/geblang). Modelled on FastAPI,
Symfony, and API Platform: write controller classes with decorated
handlers, get automatic OpenAPI 3.1 + SwaggerUI, parameter binding
from typed signatures, the repository pattern with `@ApiResource`
auto-CRUD, validation, serialization groups, JWT / session / API-key
auth, CSRF + flash + form rehydration, an asset pipeline,
WebSockets / SSE, background jobs + scheduled tasks, message brokers,
and a `gebweb` CLI for scaffolding, dev mode, migrations, and the
worker loop.

## Status

Version 1.5.2. Stable public API.

## Install

Gebweb ships in two pieces: the framework (the Geblang source you
`import gebweb` from) and the `gebweb` CLI binary (Go, used for
scaffolding, hot-reload, migrations, and the background-job
worker). They install independently.

### Framework

Add the package as a dependency of your project:

```sh
geblang install github.com/dwgebler/gebweb@v1.5.1
```

Use `@latest` to track the newest release tag, or pin to a
specific version (e.g. `@v1.5.1`).

Then `import gebweb;` from your code. This is all you need to write
and run Gebweb applications via `geblang src/main.gb` or
`geblang test tests/`. The [Geblang language toolchain](https://github.com/dwgebler/geblang)
is the only other prerequisite.

### CLI

The `gebweb` CLI is built from this repo's Go source. Pick one of:

```sh
# Easiest: install straight from the module path.
go install github.com/dwgebler/gebweb/cmd/gebweb@v1.5.1
```

```sh
# Or build from a checkout.
git clone https://github.com/dwgebler/gebweb
cd gebweb
go build -o gebweb ./cmd/gebweb
sudo install -m 0755 gebweb /usr/local/bin/gebweb
```

Verify:

```sh
gebweb --version    # gebweb 1.5.1
gebweb --help       # list subcommands
```

The CLI shells out to the host `geblang` binary at runtime, so both
need to be on `$PATH`. See the [CLI chapter](docs/15-cli.md) for the
full subcommand reference; every subcommand also supports `--help`.

## Quick start

The fastest way in is the project wizard. It prompts for the project type
(server-rendered app or JSON API), database, Docker, and port; pass `--yes`
(or flags) to skip the prompts.

```sh
gebweb new myapp                  # interactive
# or non-interactive, e.g. a Postgres-backed API:
gebweb new myapp --type api --db postgres --yes

cd myapp
gebweb dev                        # hot-reloading dev server
geblang test src/                 # run the generated TestClient suite
```

The scaffold is a complete, runnable project: a controller, a model, a
repository, a `.env`, a test, and (for an app) a template plus a CSS/TS asset
wired through the build pipeline. Ship it as a single binary:

```sh
gebweb build                      # self-contained binary at build/app
gebweb build --docker             # also emit a Dockerfile + compose.yaml
```

`gebweb build` compiles and minifies your assets (JS/TS/JSX/CSS via esbuild,
SASS via dart-sass), minifies templates, vendors SwaggerUI for offline docs,
and embeds all of it in the binary - so the deployed artifact needs nothing on
disk beside it. The same source runs unchanged under `gebweb dev`.

## A first API

Gebweb discovers your controllers and services - you never register them by
hand. Mark a service with `@Service` and a controller with `@Controller`: the
container autowires the service into the controller's constructor, and
`gebweb.app()` (no arguments) mounts every `@Controller` it finds.

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
    NoteService notes;  # autowired - no manual wiring

    func NotesController(NoteService notes) {
        this.notes = notes;
    }

    @Get("")
    func index(): http.Response {
        return this.json(this.notes.all());
    }

    @Post("")
    func create(NoteInput body): http.Response {    # JSON body binds to NoteInput
        return this.json(this.notes.add(body.text), 201);
    }

    @Get("/{id}")
    func show(string id): http.Response {
        let note = this.notes.find(id);
        if (note == null) { throw gebweb.notFound("no note " + id); }
        return this.json(note);
    }
}

let app = gebweb.app();          # no controller list - discovered automatically
gebweb.listen(app, "127.0.0.1:8080");                                 # plain HTTP
let server = gebweb.listen(app, "127.0.0.1:8443", {"tls": {"selfSigned": true}});  # self-signed HTTPS
http.wait(server);
```

The app listens on both ports at once. Run it with `geblang main.gb`, then
exercise the API over either (`-k` accepts the self-signed certificate):

```sh
curl 127.0.0.1:8080/notes -d '{"text":"buy milk"}'    # {"id":"n-1","text":"buy milk"}
curl -k https://127.0.0.1:8443/notes                   # [{"id":"n-1","text":"buy milk"}]
curl 127.0.0.1:8080/notes/nope                         # 404 Problem Details JSON
```

Open <http://127.0.0.1:8080/docs> (or <https://127.0.0.1:8443/docs>) for the
auto-generated SwaggerUI.

### Serving and TLS

The example above binds two ports at once with `gebweb.listen` (non-blocking,
returns a handle) plus `http.wait`. For a single blocking server,
`gebweb.serve(app, addr, opts)` picks the transport from `opts`:

```gb
gebweb.serve(app, "127.0.0.1:8080");                                  # plain HTTP
gebweb.serve(app, "127.0.0.1:8443", {"tls": {"selfSigned": true}});   # local HTTPS, self-signed cert
gebweb.serve(app, ":443", {"tls": {"autoCert": "api.example.com"}});  # production HTTPS, Let's Encrypt
```

`autoCert` obtains and renews certificates automatically over ACME (the host
must be publicly resolvable). For a flag-driven entrypoint (`--port`,
`--self-signed`, `--domain`) use `gebweb.cli(app)`; see the
[deployment chapter](docs/37-deployment.md).

### Docs in production

The `/openapi.json` (spec) and `/docs` (SwaggerUI) routes mount by default. An
explicit `gebweb.app` option turns them off and always wins over the
environment:

```gb
gebweb.app(controllers, {"docs": false});       # both off
gebweb.app(controllers, {"swaggerUi": false});  # keep the spec, drop the UI
gebweb.app(controllers, {"openapi": false});    # both off (the UI needs the spec)
```

Or disable them via the environment with `GEBWEB_DOCS=off` or
`GEBWEB_ENV=production`. To keep the routes but require authentication, use
`gebweb.useDocsAuth(app, guard)`. Pass a controller list to `gebweb.app([...])`
only to register controllers explicitly instead of by discovery.

## What you get

- **Routing**: `@Get` / `@Post` / `@Put` / `@Patch` / `@Delete` /
  `@Route`, controller `@Prefix`, `{name}` path segments.
- **Parameter binding**: typed signatures auto-bind path / query
  string / body / headers; `@PathParam`, `@QueryParam`, `@Body`,
  `@Header` decorators when you want to be explicit.
- **Validation**: `@Assert.Email`, `@Assert.Length`,
  `@Assert.Range`, `@Assert.Pattern`, `@Assert.NotBlank`,
  `@Assert.Choice`, plus custom validators. Failures become RFC
  7807 Problem Details (JSON) or post-redirect-get with preserved
  input + per-field errors (HTML).
- **Serialization**: `@Groups` per-field read / write filters.
- **Auth**: pluggable authenticator, `@Auth` and `@RequiresRole`,
  bearer / API-key / session strategies, JWT helpers, OpenAPI
  security schemes.
- **Data layer**: `Repository<T>` interface, `@ApiResource`
  auto-CRUD, query DSL (`gebweb.eq` / `like` / `gt` / ...,
  `.where(...).orderBy(...).limit(n)`), offset + cursor pagination,
  schema migrations via `gebweb migrate`.
- **Server-rendered UX**: CSRF (`gebweb.useCsrf`, `@CsrfExempt`),
  flash messages (`gebweb.flash`, `{{ flashes }}`), form
  rehydration (`{{ old }}`, `{{ errors }}`), asset fingerprinting
  (`gebweb.useStaticAssets`, `{{ asset("app.css") }}`), security
  headers + CSP nonces.
- **Views**: Twig-style template engine with 17 built-in filters,
  custom filters, auto HTML-escape.
- **Streaming**: `@WebSocket`, `@Sse`.
- **Uploads**: multipart parsing, `UploadedFile`,
  `dict<string, UploadedFile>` parameters.
- **DI**: constructor injection with autowiring; per-request
  scope; `@Param("key")` for primitive config (db URLs, secrets,
  feature flags) registered via `gebweb.parameter`.
- **Background work**: `@Job` handlers with retry / backoff;
  `@Scheduled` cron with leader election; `@On` event bus;
  `@OnMessage` handlers for RabbitMQ / STOMP / SQS / Kafka.
- **Integrations**: mailer (SMTP / memory / log), file storage
  (memory / local / S3), response caching (`@Cache`).
- **OpenAPI 3.1**: auto-generated spec at `/openapi.json`;
  SwaggerUI mounted at `/docs` (vendored and served offline from a
  built binary).
- **Middleware**: `cors`, `securityHeaders`, `requestId`,
  `requestLog`, `compress`, `rateLimit`, plus `gebweb.use` /
  `before` / `after` hooks.
- **Single-binary builds**: `gebweb build` compiles + minifies assets
  (esbuild / dart-sass), minifies templates, and embeds them plus the
  vendored SwaggerUI in one self-contained binary; `--docker` also
  generates a `Dockerfile` and `compose.yaml` for the chosen database.
- **CLI**: an interactive project wizard (`gebweb new`), plus `dev`,
  `build`, `docker`, `routes`, `generate`, `migrate`, `worker`. Each
  subcommand supports `--help`.
- **Testing**: in-process `TestClient` integrated with the
  `test.Test` base class.

## Documentation

The full manual is under [`docs/`](docs/), starting at
[`docs/00-index.md`](docs/00-index.md). Each chapter opens with an
overview, walks through the feature with code, and ends with a
"Reference" subsection listing every helper signature for the
topic.

## Examples

[`examples/`](examples/) holds runnable applications. Single-file starters:

- `hello.gb` - minimal three-endpoint hello world.
- `notes.gb` - auto-discovery: an injected `@Service` and an
  auto-mounted `@Controller`, no manual registration.
- `widgets.gb` - full CRUD via `@ApiResource` + repository.
- `auth.gb` - `@Auth` + `@RequiresRole` walkthrough.
- `responses.gb` - HTML / file-download / streaming routes.

Run any of them with `geblang examples/<name>.gb`.

Larger, multi-file projects (each its own package with `geblang.yaml`,
`src/`, and tests):

- `feature_tour/` - a broad tour: API-key auth, cursor pagination,
  background jobs, scheduled cleanup, events, mailer, storage.
- `server_rendered_blog/` - server-rendered UX: views, CSRF, flash,
  forms, the asset pipeline.
- `chat/` - WebSocket fan-out with `@WebSocket`.
- `tasks/` - a focused CRUD task manager.

## Layout

- [`src/`](src/) - the framework modules (`gebweb`, `gebweb.app`,
  `gebweb.auth`, `gebweb.binding`, `gebweb.cache`,
  `gebweb.decorators`, `gebweb.di`, `gebweb.errors`, `gebweb.jwt`,
  `gebweb.middleware`, `gebweb.openapi`, `gebweb.repository`,
  `gebweb.resource`, `gebweb.response`, `gebweb.serialization`,
  `gebweb.streaming`, `gebweb.swaggerui`, `gebweb.testclient`,
  `gebweb.types`, `gebweb.uploads`, `gebweb.validation`, ...).
- [`docs/`](docs/) - chapter-per-feature manual.
- [`tests/`](tests/) - framework test suite (`geblang test tests/`).
- [`examples/`](examples/) - runnable example applications.
- `geblang.yaml` - the package manifest.

## License

MIT. See [`LICENSE`](LICENSE).

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

Version 1.1.0. Stable public API.

## Install

Gebweb ships in two pieces: the framework (the Geblang source you
`import gebweb` from) and the `gebweb` CLI binary (Go, used for
scaffolding, hot-reload, migrations, and the background-job
worker). They install independently.

### Framework

Add the package as a dependency of your project:

```sh
geblang install github.com/dwgebler/gebweb@v1.0.0
```

Then `import gebweb;` from your code. This is all you need to write
and run Gebweb applications via `geblang src/main.gb` or
`geblang test tests/`. The [Geblang language toolchain](https://github.com/dwgebler/geblang)
is the only other prerequisite.

### CLI

The `gebweb` CLI is built from this repo's Go source. Pick one of:

```sh
# Easiest: install straight from the module path.
go install github.com/dwgebler/gebweb/cmd/gebweb@v1.0.0
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
gebweb --version    # gebweb 1.0.0
gebweb --help       # list subcommands
```

The CLI shells out to the host `geblang` binary at runtime, so both
need to be on `$PATH`. See the [CLI chapter](docs/15-cli.md) for the
full subcommand reference; every subcommand also supports `--help`.

## Hello world

```gb
import gebweb;

class GreetingDTO {
    string name;
    ?string greeting;
}

class HelloController {
    @Get("/")
    func root(): dict<string, any> {
        return {"message": "hello from gebweb"};
    }

    @Get("/hello/{name}")
    func hello(string name): dict<string, any> {
        return {"message": "hello, " + name};
    }

    @Post("/greetings")
    func create(GreetingDTO body): dict<string, any> {
        return {"saved": body.name};
    }
}

let app = gebweb.app([HelloController]);
gebweb.serve(app, ":8080");
```

Open <http://127.0.0.1:8080/> and <http://127.0.0.1:8080/docs> for
the auto-generated SwaggerUI.

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
  SwaggerUI mounted at `/docs`.
- **Middleware**: `cors`, `securityHeaders`, `requestId`,
  `requestLog`, `compress`, `rateLimit`, plus `gebweb.use` /
  `before` / `after` hooks.
- **CLI**: `gebweb new`, `dev`, `build`, `routes`, `generate`,
  `migrate`, `worker`. Each subcommand supports `--help`.
- **Testing**: in-process `TestClient` integrated with the
  `test.Test` base class.

## Documentation

The full manual is under [`docs/`](docs/), starting at
[`docs/00-index.md`](docs/00-index.md). Each chapter opens with an
overview, walks through the feature with code, and ends with a
"Reference" subsection listing every helper signature for the
topic.

## Examples

[`examples/`](examples/) holds runnable applications:

- `hello.gb` - minimal three-endpoint hello world.
- `widgets.gb` - full CRUD via `@ApiResource` + repository.
- `auth.gb` - `@Auth` + `@RequiresRole` walkthrough.
- `responses.gb` - HTML / file-download / streaming routes.

Run any of them with `geblang examples/<name>.gb`.

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

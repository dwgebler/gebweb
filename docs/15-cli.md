# The `gebweb` CLI

<!-- doctest:strict -->

The `gebweb` binary is the developer interface to the framework. It
scaffolds projects, runs them with hot-reload, builds release
binaries, prints the routes table, emits boilerplate files, runs
schema migrations, and drives the background-job + messaging worker.

## An entry point the CLI can run

`dev`, `routes` and `build` use `src/main.gb` by default. Keep application
construction separate from the entry point so tests can build the same app
without starting a server. This small application keeps its database connection
in a repository and lets discovery construct the controller.

Put this in `src/status_app.gb`:

<!-- doctest:file status_app.gb -->
```gb
import db;
import gebweb;

class StatusRepository {
    db.Connection conn;

    func StatusRepository(db.Connection conn) {
        this.conn = conn;
    }

    func ready(): bool {
        return this.conn.queryOne("SELECT 1 AS ready")["ready"] == 1;
    }
}

@Controller("/health")
class HealthController {
    StatusRepository repository;

    func HealthController(StatusRepository repository) {
        this.repository = repository;
    }

    @Get("")
    func show(): dict<string, bool> {
        return {"ready": this.repository.ready()};
    }
}

export func build(db.Connection conn): gebweb.GebwebApp {
    let app = gebweb.app();
    gebweb.registerInstance(app, StatusRepository, StatusRepository(conn));
    return app;
}
```

Then `src/main.gb` handles arguments before opening resources and registers
cleanup before serving:

<!-- doctest:run file=main.gb -->
```gb
module main;
import db;
import gebweb;
import gebweb.cli as cli;
import status_app;

export func main(list<string> args): int {
    cli.checkArgs(args, {"name": "status"});
    let conn = db.connect("sqlite", ":memory:");
    let app = status_app.build(conn);
    gebweb.onShutdown(app, "database", func(): void {
        conn.close();
    });
    gebweb.cli(app, {"name": "status", "port": 8080});
    return 0;
}
```

`gebweb routes` prints this table and returns without opening a listener:

```text
METHOD  PATH                          HANDLER
GET     /health                       HealthController.show
```

The same factory works in a test or a short diagnostic script:

<!-- doctest:run -->
```gb
import db;
import gebweb;
import io;
import status_app;

let conn = db.connect("sqlite", ":memory:");
let client = gebweb.TestClient(status_app.build(conn));
let response = client.get("/health");
io.println(response.status);
io.println(response.json()["ready"]);
conn.close();
```

```text
200
true
```

For a persistent application, open the configured database instead of
`:memory:`. `cli.checkArgs` answers `--help` and `--version`, rejects unknown
arguments and applies `--dev` before database setup. `gebweb.cli` handles the
HTTP flags and graceful shutdown; see [Deployment](37-deployment.md).

## Install

Gebweb 1.11.1 requires Geblang 1.47.0 or later. Install the Geblang toolchain
first, then download a `gebweb` CLI archive for
your platform from the [Gebweb releases](https://github.com/dwgebler/gebweb/releases/latest).
Extract the binary and put it on your `PATH`. With Go installed, you can install
the CLI from its module instead:

```sh
go install github.com/dwgebler/gebweb/cmd/gebweb@v1.11.1
```

The CLI and the framework package install separately. `gebweb` shells out to
`geblang`, so keep both binaries on your `PATH`. `gebweb new myapp` declares the
framework package in the generated `geblang.yaml`, pinned to the CLI's own
version; run `geblang install` inside `myapp` to fetch it so `import gebweb;`
resolves. In an existing project, `geblang install github.com/dwgebler/gebweb@v1.11.1`
adds and fetches it. See [Getting started](01-getting-started.md#install-gebweb)
for the full sequence.

## `gebweb new [<name>]`

Interactive project wizard. Run with no flags, it prompts for the project
name, type, database, Docker, and port; pass any of them as flags to skip
the matching prompt, or `--yes` (or pipe stdin) to take defaults
non-interactively.

    gebweb new                                   # fully interactive
    gebweb new blog --type app --db sqlite --yes
    gebweb new api  --type api --db postgres --docker --port 9000 --yes

| Option | Default | Effect |
|--------|---------|--------|
| `--type app` or `--type api` | `app` | Server-rendered app with templates/assets, or JSON API. |
| `--db DRIVER` | `sqlite` | `sqlite`, `postgres`, `pgvector` or `mysql`. |
| `--docker` / `--no-docker` | No Docker files | Generate or explicitly decline Docker files. |
| `--port PORT` | `8080` | Port written to `.env` and Docker configuration. |
| `--yes`, `-y` | Interactive on a terminal | Accept unspecified defaults without prompts; a project name is still required. |

It scaffolds a buildable entry (`src/main.gb`, a `module` exporting `main`
that loads `.env`, checks its arguments with `cli.checkArgs` before opening the database,
closes the database in a shutdown hook and serves through `gebweb.cli`, so
`--help`, `--version`, `--port`, `--dev` and graceful Ctrl+C work from the
start; see [Deployment](37-deployment.md)),
a `geblang.yaml` that depends on the framework package, a `.env` (with
`GEBWEB_PORT` and the DB DSN), a sample controller + model + repository that
stores widgets in the configured database, a `TestClient` suite, and a
`.gitignore`. An `app` project also
gets a `templates/page.html` and a CSS/TS asset wired through the asset
pipeline; with `--docker` it adds a `Dockerfile` and `compose.yaml` (with the
matching DB service). Run `geblang install` inside the new project before
running `gebweb dev` or `gebweb build`.

The scaffold passes constructed controller instances explicitly. The factory
example above shows how to grow it into an application using discovery and
registered repositories.

## `gebweb dev`

Runs the project with hot-reload. The CLI watches `src/`
recursively via fsnotify; when a `.gb` or `.yaml` file changes, it
sends `SIGTERM` to the running child, waits up to 2 s for it to
exit cleanly, and starts a new one. Burst saves are coalesced via
a 200 ms debounce.

    cd myapp
    geblang install
    gebweb dev

By default, the entry point is `src/main.gb`. Override with
`--entry`. When the project has an `assets:` block, `gebweb dev`
compiles the asset entry points once (unminified) before starting
so the dev server serves them from disk.

| Option | Default | Effect |
|--------|---------|--------|
| `--entry PATH` | `src/main.gb` | Entry file to restart. |

The watcher restarts on `.gb` and `.yaml` changes under the directories it
watches in `src/`. It does not watch project-root configuration or compile
assets again on each save. View-template reload is a separate framework policy;
see [Templates](04a-views.md). `dev` does not itself set `GEBWEB_ENV=dev`;
configure that environment when you want development error details and profiling.

## `gebweb build`

Processes assets/templates and wraps `geblang build` for the bundling step:

    gebweb build               # builds ./build/app
    gebweb build --out dist/myapp
    gebweb build --no-minify   # skip minification
    gebweb build --no-sass     # skip SASS when dart-sass is absent
    gebweb build --no-swagger  # skip embedding SwaggerUI assets

| Option | Default | Effect |
|--------|---------|--------|
| `--entry PATH` | `src/main.gb` | Entry module to bundle. |
| `--out PATH` | `build/app` | Output executable. |
| `--geblang PATH` | `GEBLANG_BIN`, then `geblang` on `PATH` | Compiler used for this build. |
| `--no-minify` | Minification enabled | Leave assets and templates unminified. |
| `--no-sass` | SASS enabled | Skip SASS when its compiler is absent; an installed compiler still runs. |
| `--no-swagger` | SwaggerUI assets embedded | Skip downloading and embedding the UI assets. |
| `--docker` | Disabled | Generate Docker files after a successful build. |
| `--db DRIVER` | `sqlite` | Database service for `--docker`. |
| `--port PORT` | `8080` | Port for generated Docker files, not a compiled-in HTTP setting. |
| `--force` | Disabled | Overwrite Docker files when used with `--docker`. |

`--geblang` and `GEBLANG_BIN` select the compiler for `build`; the `dev`,
`routes` and legacy worker commands invoke `geblang` from `PATH`.

With an `assets:` block in `geblang.yaml`, the asset entry points are
compiled and minified (JS/TS/JSX/CSS via esbuild, SASS via dart-sass),
HTML templates are minified, and the compiled output, `templates/`, and
`public/` are embedded in the binary so it is self-contained. The app
resolves them at run time via `sys.bundleDir()`. See the
[asset pipeline](26-assets.md) chapter for the full config.

`--no-swagger` does not disable the documentation routes. Disable SwaggerUI
in the app or provide its assets separately when using that flag; see
[OpenAPI](13-openapi.md).

`gebweb build --docker` also generates a Dockerfile and compose.yaml after
building (see `gebweb docker` below).

## `gebweb docker`

Generates a `Dockerfile` and `compose.yaml` for the project:

    gebweb docker                      # sqlite, port 8080
    gebweb docker --db postgres
    gebweb docker --db pgvector --port 9000
    gebweb docker --out dist/myapp     # use a custom build output
    gebweb docker --force              # overwrite existing files

The Dockerfile copies the host-built binary (`gebweb build`) into a
`gcr.io/distroless/base-debian12` image (including glibc) and wires `GEBWEB_PORT`. The compose file runs the app service with
the port and `.env` wired in, plus an optional database service:

- `sqlite` (default): no DB service; a named volume persists the database file.
- `postgres`: `postgres:16` with a healthcheck and named volume.
- `pgvector`: `pgvector/pgvector:pg16` (postgres with the vector extension).
- `mysql`: `mysql:8` with a healthcheck and named volume.

| Option | Default | Effect |
|--------|---------|--------|
| `--db DRIVER` | `sqlite` | Choose the database service described above. |
| `--out PATH` | `build/app` | Binary copied into the image, with its adjacent `.NOTICES.txt` file. |
| `--port PORT` | `8080` | Published application port. |
| `--force` | Disabled | Replace existing Docker files. |

Build for the target container platform before packaging: this command generates
files and does not cross-compile the executable.

Existing `Dockerfile` / `compose.yaml` are left untouched unless `--force` is
given, so your edits are safe. The same flags work on `gebweb build --docker`,
which builds the binary first and then generates the files.

## `gebweb routes`

Runs the app with `GEBWEB_PRINT_ROUTES=1` set. `gebweb.cli` and
`gebweb.serve` detect the variable, print the route table and return
without binding a port. Code that runs before them, such as reading
required configuration, still runs. To format a table in your own code,
use `gebweb.formatRouteTable(gebweb.routes(app))`.

`--entry PATH` selects a different entry file (default `src/main.gb`). Because
the app executes, route inspection can still open databases, run migrations or
require secrets; give CI the configuration its startup needs.

    $ gebweb routes
    METHOD  PATH                          HANDLER
    GET     /                             PagesController.index
    GET     /hello/{who}                  PagesController.hello

## `gebweb generate <kind> <Name>`

Emits boilerplate under `src/`:

    gebweb generate controller User    # src/user_controller.gb
    gebweb generate dto User           # src/user_dto.gb
    gebweb generate repository User    # src/user_repository.gb
    gebweb generate resource Widget    # full bundle: controller + dto + repository + test

`Name` must be PascalCase. The snippets are starting points: replace placeholder
fields and persistence logic before using them in an application.
The CLI refuses to overwrite an existing file.

## `gebweb generate client <spec> <Name>`

Generates a typed HTTP-client class from an OpenAPI 3.x spec. Accepts
YAML or JSON (JSON is parsed as the YAML subset it is). The output is
a single self-contained `src/<name>_client.gb` file with:

- One exported data class per `components/schemas/*` object (`Pet`,
  `NewPet`, ...). Schema fields become typed Geblang fields; the
  schema's `required` list controls nullability (non-required
  becomes `?type`).
- A `<Name>Client` class wrapping the stdlib `http` module. The
  constructor takes a base URL plus an optional `auth` config dict.
  One method per operation; each builds the URL (path interpolation
  from path params, query string from query params), the headers
  (configured auth and declared header parameters), and the body (`json.stringify`
  of typed DTOs), calls `http.requestWithOptions`, and decodes the
  response into the schema-declared return type via `json.parse`
  or `json.parseAs`. Unsupported union/composition shapes fall back to `any`;
  this is a practical client generator rather than a complete OpenAPI validator.
- Auth: covers the four common security schemes from `components/
  securitySchemes`. The constructor's `auth` dict accepts:
  `bearerToken` (HTTP bearer), `basicUser` + `basicPassword` (HTTP
  basic), and `apiKey` (apiKey scheme, automatically routed to the
  configured header / query parameter / cookie). OAuth2 / OIDC
  schemes use a supplied bearer token; the client does not acquire or refresh it.
- A 4xx / 5xx response raises `RuntimeError` with the HTTP method,
  path, status, and response body so callers can `try` / `catch`
  cleanly.

The spec must be a local file; the generator does not fetch URLs:

```sh
gebweb generate client spec.yaml Petstore
```

For `Petstore`, the output is `src/petstore_client.gb` and the class is
`PetstoreClient`. The constructor is `PetstoreClient(string baseUrl,
dict<string, any> auth = {})`; supply the base URL explicitly. Generated methods
and DTO fields depend on the spec, so inspect that output before writing calls.

| Auth key | Default | Effect when the spec declares the scheme |
|----------|---------|-----------------------------------------|
| `bearerToken` | `null` | Send an HTTP bearer token. |
| `basicUser`, `basicPassword` | `null` | Send HTTP Basic credentials when both are set. |
| `apiKey` | `null` | Send the key at the location declared by the spec. |

The client exposes the supplied schemes through shared request-building helpers.
Do not assume it selects credentials per operation or implements OAuth login.
Use a client instance configured for its intended API and trust boundary.

Regenerate from the source spec; the generated file carries a "do
not edit" header. The CLI refuses to overwrite an existing file,
so delete the old output first when re-running.

## `gebweb migrate <create|up|down|status>`

Applies versioned SQL migrations to the database in `DATABASE_URL`.
See [Database migrations](17-migrations.md) for the file format
and the full subcommand reference.

    gebweb migrate create add_users     # writes migrations/<timestamp>_add_users.sql
    gebweb migrate up                   # applies all pending
    gebweb migrate down --steps 1       # rolls back the most recent
    gebweb migrate status               # prints applied / pending

## `gebweb secrets <init|edit|set|get|list>`

Manages an encrypted secrets file for use with
`gebweb.useSecrets(app, gebweb.encryptedFileSecrets())`. The
vault stores name -> value pairs that YAML `%secret(name)%`
markers resolve to.

```sh
gebweb secrets init                    # one-off, creates key + empty vault
gebweb secrets set stripe.key sk_live_abc
gebweb secrets get stripe.key
gebweb secrets list
gebweb secrets edit                    # opens $EDITOR on plaintext
```

| Option / environment | Default | Effect |
|----------------------|---------|--------|
| `--key-file PATH` | `config/secrets.key` | Key file when no environment key is supplied. |
| `--file PATH` | `config/secrets.enc` | Encrypted vault path. |
| `GEBWEB_SECRETS_KEY` | Unset | Supply the encryption key without a key file. |
| `--force` | Disabled | Allow `init` to replace existing files. |

Keep the key out of source control and release bundles. `get` prints plaintext;
`set` values supplied on the command line may be retained in shell history.
See [services.yaml](29-services-yaml.md) for application wiring.

## `gebweb worker`

Prefer a module entry exporting `main(list<string> args): int` and returning
`gebweb.workerCli(app, args)` after configuring jobs and discovering handlers.
Unlike the HTTP entry point, it passes worker arguments to `workerCli`, not
`cli.checkArgs`. See [Background jobs](18-background-jobs.md) for setup.

```sh
gebweb worker --entry app.worker
gebweb worker --entry app.worker --once
gebweb worker --entry app.worker --max-jobs 10
gebweb worker --entry app.worker stats --format json
gebweb worker --entry app.worker dlq list
```

A module entry (no `.gb` suffix) delegates to `geblang worker --entry MODULE`.
Worker options after `--entry` are forwarded to the module's `main`. `--once`
and `--max-jobs N` bound job processing; `stats` and `dlq list|retry|purge`
operate through that app's configured engine. Run `--help` through the module
for its worker options.

### Legacy file bootstrap

With no `--entry`, or with a `.gb` file path, the command starts the file
(default `src/main.gb`) with `GEBWEB_RUN=worker`. Your entry point must branch
on that variable and select the worker loop: `gebweb.cli` is an HTTP entry
point and does not select worker mode automatically.

| Legacy option | Default | Effect |
|---------------|---------|--------|
| `--entry PATH.gb` | `src/main.gb` | File bootstrap. |
| `--job NAME` | All job names | Repeatable job-name selection via `GEBWEB_WORKER_JOBS`. |
| `--handle NAME` | All configured handles | Repeatable messaging selection via `GEBWEB_WORKER_HANDLES`. |
| `--jobs-only` | No kind selection | Set `GEBWEB_WORKER_KIND=jobs`. |
| `--messaging-only` | No kind selection | Set `GEBWEB_WORKER_KIND=messaging`. |

The job and message loops honour their respective name/handle environment
filters. The bootstrap must use `GEBWEB_WORKER_KIND` to choose which loop to
start; setting it does not make `gebweb.cli` run workers. Do not run two
blocking loops consecutively expecting them to consume concurrently. Prefer
separate processes for jobs and messaging:

```sh
gebweb worker --jobs-only --job email --job sms
gebweb worker --messaging-only --handle orders
```

These commands require a cooperating file bootstrap. For module messaging
workers, use `--entry MODULE --messaging-only --handle NAME` instead.

Multiple workers can share a work pool: job claims are atomic and broker
consumers coordinate through their broker. Size pools to the database and
external services they use. The legacy `gebweb worker dlq list|retry|purge`
commands without a module entry use `DATABASE_URL`; prefer the module form
when the app configures its own job engine.

## When to use each

| Command | Workflow |
|---------|----------|
| `gebweb new` | One-off; scaffold a new project. |
| `gebweb dev` | Inner loop; restarts on save. |
| `gebweb build` | Release; bundles a self-contained binary. |
| `gebweb docker` | Release; generate Dockerfile + compose.yaml. |
| `gebweb routes` | CI / debugging; static dump of the routes table. |
| `gebweb generate` | Boilerplate; emit a new class skeleton. |
| `gebweb migrate` | Schema versioning; apply / roll back SQL. |
| `gebweb worker` | Long-running; drain the background-job queue. |
| `gebweb secrets` | One-off; manage the encrypted secrets file (see [services.yaml](29-services-yaml.md)). |

## Claude Code skill

Gebweb ships a Claude Code skill that equips Claude to build and operate Gebweb
applications: controllers and routing decorators, parameter binding, dependency
injection, views, validation, repositories and `@ApiResource`, auth, and the
`gebweb` CLI commands documented above. It pairs with the `geblang` language
skill.

The skill is bundled at `.claude/skills/gebweb/` (a `SKILL.md` plus reference
files for the app-authoring surface and the CLI/testing workflow). Claude Code
loads it automatically as a project skill inside a checkout of this repo; to use
it elsewhere, copy it into your user skills directory:

```sh
cp -r .claude/skills/gebweb ~/.claude/skills/gebweb
```

## Reference

| Command | Purpose / defaults |
|---------|--------------------|
| `gebweb help [COMMAND]` | Help for all commands or one command. |
| `gebweb version` | Installed CLI version. |
| `gebweb licenses` | Third-party attribution notices. |
| `gebweb dev`, `gebweb routes` | Run `src/main.gb`; override with `--entry PATH`. |
| `gebweb build` | Build `src/main.gb` to `build/app`. |
| `gebweb worker --entry MODULE` | Pass worker operations to the module entry point. |

Related chapters: [Getting started](01-getting-started.md),
[Testing](14-testing.md), [Migrations](17-migrations.md),
[Background jobs](18-background-jobs.md), [Assets](26-assets.md),
[Configuration and secrets](29-services-yaml.md), and
[Deployment](37-deployment.md).

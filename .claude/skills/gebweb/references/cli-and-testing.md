# Gebweb CLI and testing reference

The `gebweb` command and how to run, build, and test an app. Verified against
Gebweb 1.10.0. The `gebweb` binary shells out to `geblang`, so both must be on
`$PATH`. Run `gebweb help <command>` for version-matched text.

## Project layout (from `gebweb new`)

```
myapp/
  geblang.yaml               # name, source, gebweb dependency (+ assets: for apps)
  .env                       # GEBWEB_PORT, DB_DRIVER, DATABASE_URL
  README.md, .gitignore
  src/
    main.gb                  # entrypoint: export func main(list<string> args): int
    main_test.gb             # TestClient suite (run: geblang test src/)
    widget.gb                # sample model
    widget_repository.gb     # sample repository over the injected db.Connection
    widget_controller.gb     # --type api; home_controller.gb for --type app
  templates/page.html        # --type app only
  assets/app.ts, app.css     # --type app only
```

Run `geblang install` in the new project before anything else: it fetches the
gebweb package the manifest declares.

## Commands

### new
```sh
gebweb new <name> [--type app|api] [--db sqlite|postgres|mysql|pgvector] \
                  [--docker] [--port <n>] [--yes|-y]
```
Scaffolds a project via an interactive wizard; flags skip the prompts.
`--type api` omits view/template scaffolding. The manifest pins gebweb to the
CLI's own version.

### dev
```sh
gebweb dev [--entry src/main.gb]
```
Hot-reload dev server: watches `src/`, restarts on change (fsnotify, debounced).

### routes
```sh
gebweb routes [--entry src/main.gb]
```
Prints the route table (method + path + handler) by running the entry with
`GEBWEB_PRINT_ROUTES=1`; `gebweb.cli` and `gebweb.serve` print and return
without binding. Code before them (e.g. required env reads) still runs, so set
the variables it needs. Use it to confirm your decorators wired the routes.

### generate (alias: gen)
```sh
gebweb generate controller <Name>
gebweb generate dto        <Name>
gebweb generate repository <Name>
gebweb generate resource   <Name>
gebweb generate client <openapi-spec> <Name>
```
Scaffolds boilerplate under `src/`. `generate client` builds a typed HTTP client
from an OpenAPI 3.x spec.

### migrate
```sh
gebweb migrate create <name>          # scaffold a timestamped migration
gebweb migrate up [--target <ver>]    # apply pending migrations (optionally up to <ver>)
gebweb migrate down [--steps <n>]     # roll back the most recent n migrations (default 1)
gebweb migrate status                 # show applied / pending
```
Runs against `DATABASE_URL` (sqlite / postgres / mysql). A migration file uses
`-- +gebweb up` / `-- +gebweb down` markers to separate the two directions. Note:
`down` takes `--steps` (the count to roll back), not `--target`.

### worker
```sh
gebweb worker [--entry src/main.gb] [--job <name>] [--handle <name>] \
              [--jobs-only] [--messaging-only]
gebweb worker dlq <list|retry|purge> [ids...] [--all]
```
Runs the background-job and message-handler worker with graceful SIGINT/SIGTERM
drain. `--job` / `--handle` filter to specific names (repeatable). With a module
entry (`gebweb worker --entry app.worker`), that module exports
`main(args)` returning `gebweb.workerCli(app, args)`; a file entry instead sees
`GEBWEB_RUN=worker` and calls `gebweb.runWorker(app)` /
`gebweb.runMessageWorker(app)`. `worker dlq` lists, retries and purges failed
jobs.

### build
```sh
gebweb build [--entry src/main.gb] [--out build/app] [--db <driver>] \
             [--port <n>] [--no-minify] [--no-sass] [--no-swagger] \
             [--docker] [--force]
```
Produces a single self-contained binary via `geblang build`, embedding templates
and static assets. `--docker` also emits a Dockerfile + compose.yaml.

### docker / secrets / version / licenses
```sh
gebweb docker [--db <driver>] [--port <n>] [--out <dir>] [--force]
gebweb secrets <init|edit|set|get|list> [--key-file <f>] [--file <f>] [--force]
gebweb version          # or --version
gebweb licenses
```
`secrets` manages an AES-256-GCM vault (`config/secrets.enc`) that
`%secret(...)%` markers in `config/services.yaml` resolve through.

## Serving in code

The entrypoint exports `main`. Prefer `gebweb.cli` for deployables:

```gb
export func main(list<string> args): int {
    let app = gebweb.app();
    gebweb.cli(app, {"name": "myapp", "port": "8080"});  # void; flags/env override opts
    return 0;
}
```

`gebweb.cli` reads `--port`, `--tls-port`, `--domain`, `--self-signed`,
`--no-tls` and `GEBWEB_*` variables (`GEBWEB_PORT`, ...), handles TLS and drains
in-flight requests on SIGINT/SIGTERM.

`gebweb.serve(app, addr, opts?)` blocks; `gebweb.listen(app, addr, opts?): int`
is non-blocking (use it for wire-level streaming tests).

## Testing with TestClient (`testclient.gb`)

Tests are `*_test.gb` files (the scaffold keeps them in `src/`; run
`geblang test src/` from the app root) that drive the app in-process - no
socket, fast, deterministic. Classes a test imports from another module must
be `export`ed.

```gb
import test;
import gebweb;
import main;

class FakeUsers extends main.UserService {
    func find(int id): ?dict<string, any> {
        return {"id": id, "name": "Ada"};
    }
}

class UserApiTest extends test.Test {
    gebweb.TestClient client;

    func setup(): void {
        let app = gebweb.app();                                   # discovery: controllers built on first request
        gebweb.registerInstance(app, main.UserService, FakeUsers());  # so stubs registered now are used
        this.client = gebweb.TestClient(app);
    }

    @test
    func getsUser(): void {
        let res = this.client.get("/users/1");
        res.assertStatus(200);
        this.assertEquals("Ada", res.json()["name"]);
    }

    @test
    func createsUser(): void {
        let res = this.client.post("/users", {"name": "Bo"});
        res.assertStatus(201);
    }
}
```

`TestClient` methods: `get(path)`, `post(path, body)`, `put(path, body)`,
`patch(path, body)`, `delete(path)`, `request(method, path, body, headers)`,
`send(method, path, body, headers)`, `multipart(path, parts, extraHeaders)`.
`TestResponse` exposes `status` / `headers` / `body` fields and `json()`,
`text()`, `assertStatus(want)` methods. Stub dependencies with
`gebweb.registerInstance` before any controller that needs them is built:
`gebweb.app([Ctrl])` builds listed controllers immediately, so with an explicit
list construct the controller yourself with its fakes
(`gebweb.app([UserController(FakeUsers())])`). The `setup` / `teardown`
methods run around each test.

`TestClient` does NOT drive live WebSocket / SSE / chunked-stream bodies (the
handler returns its upgrade/stream object but the push body is not invoked) - for
those, run a real `gebweb.listen` server and connect over the wire.

Note for framework contributors only: Gebweb's OWN test suite must run from the
`gebweb/` directory (`cd gebweb && ../geblang test tests/`) because its fixtures
use relative paths; this does not apply to your application's tests.

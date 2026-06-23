# Gebweb CLI and testing reference

The `gebweb` command and how to run, build, and test an app. Verified against
Gebweb 1.6.0. The `gebweb` binary shells out to `geblang`, so both must be on
`$PATH`. Run `gebweb help <command>` for version-matched text.

## Project layout (from `gebweb new`)

```
myapp/
  geblang.yaml          # package manifest (name, source, dependencies)
  .env                  # DATABASE_URL and other config
  src/
    main.gb             # entrypoint: export func main(list<string> args): int
    controllers/        # @Controller classes (scaffolded sample)
    ...
  templates/            # views (if --type app)
  tests/                # TestClient suites
```

## Commands

### new
```sh
gebweb new <name> [--type app|api] [--db sqlite|postgres|mysql|pgvector] \
                  [--docker] [--port <n>] [--yes|-y]
```
Scaffolds a project via an interactive wizard; flags skip the prompts.
`--type api` omits view/template scaffolding.

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
route-printing enabled. Use it to confirm your decorators wired the routes you
expect.

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
drain. `--job` / `--handle` filter to specific names (repeatable). `worker dlq`
manages dead-lettered jobs in the `gebweb_jobs` table.

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
    return gebweb.cli(app, {"addr": ":8080"});   # flag/env/opts precedence, TLS, graceful drain
}
```

`gebweb.serve(app, addr, opts?)` blocks; `gebweb.listen(app, addr, opts?): int`
is non-blocking (use it for wire-level streaming tests).

## Testing with TestClient (`testclient.gb`)

Tests are `*_test.gb` files (run with `geblang test tests/` from the app root)
that drive the app in-process - no socket, fast, deterministic.

```gb
import test;
import gebweb;

class UserApiTest extends test.Test {
    func setup(): void {
        let app = gebweb.app([UserController]);
        gebweb.registerInstance(app, UserService, FakeUsers());   # stub deps first
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
`gebweb.registerInstance` before building the client.

`TestClient` does NOT drive live WebSocket / SSE / chunked-stream bodies (the
handler returns its upgrade/stream object but the push body is not invoked) - for
those, run a real `gebweb.listen` server and connect over the wire.

Note for framework contributors only: Gebweb's OWN test suite must run from the
`gebweb/` directory (`cd gebweb && ../geblang test tests/`) because its fixtures
use relative paths; this does not apply to your application's tests.

# The `gebweb` CLI

The `gebweb` binary is the developer interface to the framework. It
scaffolds projects, runs them with hot-reload, builds release
binaries, prints the routes table, emits boilerplate files, runs
schema migrations, and drives the background-job + messaging worker.

## Build / install

The CLI's Go source ships in `cmd/gebweb/` alongside the framework
modules. Build it with the standard Go toolchain:

    go build -o gebweb ./cmd/gebweb
    sudo install -m 0755 gebweb /usr/local/bin/gebweb

`gebweb` shells out to `geblang` for execution, so both binaries
need to be on `$PATH`.

## `gebweb new <name>`

Creates a new Gebweb project under `./<name>/`:

    .
    └── myapp/
        ├── geblang.yaml
        ├── README.md
        └── src/
            ├── main.gb
            └── main_test.gb

The scaffolded `main.gb` is a tiny one-controller app with a
health-check route and a parameterised `/hello/{who}` route. The
test exercises the health endpoint via `gebweb.testclient`.

## `gebweb dev`

Runs the project with hot-reload. The CLI watches `src/`
recursively via fsnotify; when a `.gb` or `.yaml` file changes, it
sends `SIGTERM` to the running child, waits up to 2 s for it to
exit cleanly, and starts a new one. Burst saves are coalesced via
a 200 ms debounce.

    cd myapp
    gebweb dev

By default, the entry point is `src/main.gb`. Override with
`--entry`.

## `gebweb build`

Wraps `geblang build` for the bundling step:

    gebweb build               # builds ./build/app
    gebweb build --out dist/myapp

By default, the entry point is `src/main.gb` and the output is
`./build/app`. Both can be overridden with `--entry` and `--out`.

## `gebweb routes`

Runs the app with `GEBWEB_PRINT_ROUTES=1` set. The framework
detects the variable on startup and prints the routes table
before serving (or you can opt in explicitly via
`gebweb.printRoutesAndExit(app)` in your own startup code).

    $ gebweb routes
    GET   /
    GET   /hello/{who}

## `gebweb generate <kind> <Name>`

Emits boilerplate under `src/`:

    gebweb generate controller User    # src/user_controller.gb
    gebweb generate dto User           # src/user_dto.gb
    gebweb generate repository User    # src/user_repository.gb
    gebweb generate resource Widget    # full bundle: controller + dto + repository + test

Each kind drops a small idiomatic scaffold ready to be filled in.
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
  (auth + per-operation overrides), and the body (`json.stringify`
  of typed DTOs), calls `http.requestWithOptions`, and decodes the
  response into the schema-declared return type via `json.parse`
  or `json.parseAs`.
- Auth: covers the four common security schemes from `components/
  securitySchemes`. The constructor's `auth` dict accepts:
  `bearerToken` (HTTP bearer), `basicUser` + `basicPassword` (HTTP
  basic), and `apiKey` (apiKey scheme, automatically routed to the
  configured header / query parameter / cookie). OAuth2 / OIDC
  bindings are treated as a bearer-token holder for v1.
- A 4xx / 5xx response raises `RuntimeError` with the HTTP method,
  path, status, and response body so callers can `try` / `catch`
  cleanly.

Example:

    # spec.yaml from your vendor
    gebweb generate client spec.yaml Petstore   # writes src/petstore_client.gb

    # in code
    import gebweb;
    import petstore_client as petstore;

    let client = petstore.PetstoreClient("https://api.example.com/v1", {
        "bearerToken": gebweb.parameter(app, "petstore.token") as string
    });
    let pets = client.listPets(20, "house");      # list<Pet>
    let pet  = client.getPet("p-42");             # Pet
    let made = client.createPet(petstore.NewPet());

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

```
gebweb secrets init                    # one-off, creates key + empty vault
gebweb secrets set stripe.key sk_live_abc
gebweb secrets get stripe.key
gebweb secrets list
gebweb secrets edit                    # opens $EDITOR on plaintext
```

See [services.yaml](29-services-yaml.md) for the wiring on the
app side.

## `gebweb worker`

Runs the background-job and messaging worker loops. Re-runs
`src/main.gb` with `GEBWEB_RUN=worker` in the environment; your
main.gb branches on it and calls `gebweb.runWorker(app)` and / or
`gebweb.runMessageWorker(app)` instead of `gebweb.serve(...)`. See
[Background jobs](18-background-jobs.md) and
[Message brokers](22-messaging.md).

    gebweb worker                              # drain everything
    gebweb worker --job email --job sms        # only email + sms job names
    gebweb worker --handle orders              # only the orders messaging handle
    gebweb worker --jobs-only                  # skip messaging
    gebweb worker --messaging-only --handle audit
                                               # only the audit messaging loop

Filtering flags translate into env vars
(`GEBWEB_WORKER_JOBS` / `GEBWEB_WORKER_HANDLES` /
`GEBWEB_WORKER_KIND`) that `gebweb.runWorker` and
`gebweb.runMessageWorker` honour automatically; main.gb does not
need to plumb anything through. Different worker processes can
specialise on different work pools (e.g. one server runs email
jobs, another runs image-resize jobs) by composing these flags.

Run multiple workers in parallel by starting `gebweb worker` in
multiple processes - job-row claims are atomic and messaging
brokers handle the consumer fan-out themselves.

Pass `--help` to any subcommand for its full options.

## When to use each

| Command | Workflow |
|---------|----------|
| `gebweb new` | One-off; scaffold a new project. |
| `gebweb dev` | Inner loop; restarts on save. |
| `gebweb build` | Release; bundles a self-contained binary. |
| `gebweb routes` | CI / debugging; static dump of the routes table. |
| `gebweb generate` | Boilerplate; emit a new class skeleton. |
| `gebweb migrate` | Schema versioning; apply / roll back SQL. |
| `gebweb worker` | Long-running; drain the background-job queue. |
| `gebweb secrets` | One-off; manage the encrypted secrets file (see [services.yaml](29-services-yaml.md)). |

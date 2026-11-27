# Getting started

<!-- doctest:strict -->

A Gebweb app is a Geblang program built from a few kinds of parts:

- **Data classes** describe what the API accepts and returns. Gebweb binds
  request bodies to them and validates them with `@Assert.*` decorators.
- **Repositories and services** hold the application logic and talk to the
  database. They are ordinary classes.
- **Controllers** map HTTP routes to methods. Each method declares typed
  parameters and a typed return value, and Gebweb does the HTTP plumbing.
- **A `build` function** wires those parts into an app from the dependencies
  it is given, such as a database connection.
- **An entry point**, `main`, reads configuration, calls `build`, and hands the
  app to `gebweb.cli`, which serves it with production-ready flags, HTTPS and
  graceful shutdown.

This chapter installs Gebweb, then builds a small notes API from those parts.
Every example in it runs as shown.

## Install Gebweb

Install the [Geblang toolchain](https://github.com/dwgebler/geblang), version
1.47.0 or later, first.
Gebweb has two parts: the framework package that your program imports and an
optional CLI for creating, developing, and building projects.

Download the `gebweb` CLI for your platform from the
[Gebweb releases](https://github.com/dwgebler/gebweb/releases/latest), extract
it, and put the binary on your `PATH`. If you have Go installed, you can build
the CLI instead:

```sh
go install github.com/dwgebler/gebweb/cmd/gebweb@v1.11.1
```

Keep both `geblang` and `gebweb` on your `PATH`. Create a project, then fetch
its dependencies:

```sh
gebweb new myapp
cd myapp
geblang install
gebweb dev
```

The wizard writes a runnable project and a `geblang.yaml` that declares the
framework package at the CLI's version; `geblang install` fetches it so
`import gebweb;` resolves. Run `geblang test src/` for the generated tests.

In an existing Geblang project, add and fetch the framework from the project
root:

```sh
geblang install github.com/dwgebler/gebweb@v1.11.1
```

Use `@latest` instead of a version tag to select the newest release. The CLI is
optional when you run applications directly with Geblang.

## A first API

The notes API stores notes in SQLite and serves three routes: list notes,
fetch one, and create one. It is split into one module per part:

```text
notes/
  geblang.yaml
  src/
    note.gb               the data the API accepts and returns
    note_repository.gb    SQL behind a small typed API
    notes_controller.gb   the HTTP routes
    app.gb                build(): wires the app from its dependencies
    main.gb               entry point: configuration, then gebweb.cli
```

### Data classes

`Note` is what the API returns. `NewNote` is what a client sends to create
one; it has no `id`, because the database assigns it, and its decorators say
what a valid note is.

<!-- doctest:file note.gb -->
```gb
module note;

export class Note {
    int id;
    string text;

    func Note(int id, string text) {
        this.id = id;
        this.text = text;
    }
}

export class NewNote {
    @Assert.notBlank
    @Assert.maxLength(500)
    string text;
}
```

Keeping input and output classes separate means a client can never set a
field it should not control, such as `id`. See [Validation](05-validation.md)
for every `@Assert` rule.

### A repository

The repository owns the SQL and returns typed values, so nothing else in the
app deals in database rows. It holds the shared `db.Connection`, which is a
connection pool that every request can use at the same time, and no other
state.

<!-- doctest:file note_repository.gb -->
```gb
module note_repository;
import db;
import note;

func toNote(dict<string, any> row): note.Note {
    return note.Note(row["id"] as int, row["text"] as string);
}

export class NoteRepository {
    db.Connection conn;

    func NoteRepository(db.Connection conn) {
        this.conn = conn;
    }

    func all(): list<note.Note> {
        return this.conn.query("SELECT id, text FROM notes ORDER BY id").all().map(toNote);
    }

    func find(int id): ?note.Note {
        let row = this.conn.queryOne("SELECT id, text FROM notes WHERE id = ?", id);
        return row == null ? null : toNote(row);
    }

    func add(note.NewNote input): note.Note {
        let result = this.conn.exec("INSERT INTO notes (text) VALUES (?)", input.text);
        return note.Note(result["lastInsertId"] as int, input.text);
    }
}
```

Values always go through `?` placeholders, never into the SQL string. Requests
run in parallel and share the repository, so it must not keep per-request
state in its fields; see [Concurrency and shared state](35-concurrency.md).

### A controller

`@Controller("/notes")` marks the class for discovery and prefixes its routes.
Each route method declares what it needs, and Gebweb supplies it:

<!-- doctest:file notes_controller.gb -->
```gb
module notes_controller;
import gebweb;
import note;
import note_repository as repo;

@Controller("/notes")
export class NotesController extends gebweb.Controller {
    repo.NoteRepository notes;

    func NotesController(repo.NoteRepository notes) {
        parent();
        this.notes = notes;
    }

    @Get("")
    @Summary("List notes")
    func list(): list<note.Note> {
        return this.notes.all();
    }

    @Get("/{id}")
    @Summary("Fetch one note")
    func show(int id): note.Note {
        let found = this.notes.find(id);
        if (found == null) {
            this.notFound("no note ${id}");
        }
        return found as note.Note;
    }

    @Post("")
    @Summary("Create a note")
    func create(@Body note.NewNote input): gebweb.Response {
        let saved = this.notes.add(input);
        return this.created(saved, "/notes/${saved.id}");
    }
}
```

- The constructor asks for a `NoteRepository`. Gebweb's dependency-injection
  container supplies it when it builds the controller.
- `show(int id)` receives the `{id}` path segment converted to an `int`. A
  segment that is not a number gets a 400 response before the method runs.
- `create(@Body note.NewNote input)` receives the JSON (or form) body as a
  validated `NewNote`. An invalid body gets a 422 response listing every
  failing field, and the method never runs.
- `list` and `show` return typed values, which Gebweb encodes as JSON.
  `create` returns a response built with `this.created`, which sets the 201
  status and the `Location` header.
- `this.notFound(...)` throws an HTTP exception that becomes a 404 response.

Every route also appears in the generated OpenAPI document at
`/openapi.json` and in SwaggerUI at `/docs`, with `@Summary` as its title.

### Wiring the app

`build` creates the app around the dependencies it receives. The entry point
passes it a real database; tests pass an in-memory one. Nothing in `build`
reads the environment, so the same wiring runs everywhere.

<!-- doctest:file app.gb -->
```gb
/**
 * Builds the notes application around its database.
 * @allowUnusedImports
 */
module app;
import db;
import gebweb;
import note_repository as repo;
import notes_controller;

let migrations = [
    {
        "id": "001_create_notes",
        "sql": "CREATE TABLE notes (id INTEGER PRIMARY KEY AUTOINCREMENT, text TEXT NOT NULL)",
    },
];

export func build(db.Connection conn): gebweb.GebwebApp {
    conn.migrate(migrations);
    let application = gebweb.setInfo(gebweb.app(), {"title": "Notes", "version": "0.1.0"});
    gebweb.registerInstance(application, repo.NoteRepository, repo.NoteRepository(conn));
    return application;
}
```

- `conn.migrate` applies each migration once and records it, so `build` is
  safe to run on every start. See [Database migrations](17-migrations.md).
- `gebweb.app()` with no arguments discovers every loaded class marked
  `@Controller`. Discovery only sees classes in modules that have been
  imported, which is why `app.gb` imports `notes_controller` without naming
  anything from it. The `@allowUnusedImports` tag in the file's doc block
  tells `geblang check` that the import is deliberate.
- `registerInstance` gives the container the one `NoteRepository` to share.
  The container builds the controller with it on the first request.

### The entry point

<!-- doctest:run file=main.gb -->
```gb
module main;
import app;
import db;
import gebweb;
import gebweb.cli as cli;
import sys;

export func main(list<string> args): int {
    cli.checkArgs(args, {"name": "notes"});
    let conn = db.connect("sqlite", sys.getenv("DATABASE_URL") ?? "notes.db");
    let application = app.build(conn);
    gebweb.onShutdown(application, "database", func(): void {
        conn.close();
    });
    gebweb.cli(application, {"name": "notes", "port": 8080});
    return 0;
}
```

`gebweb routes` runs this entry point but prints the routes instead of serving:

```text
METHOD  PATH                          HANDLER
POST    /notes                        NotesController.create
GET     /notes                        NotesController.list
GET     /notes/{id}                   NotesController.show
```

- `cli.checkArgs` answers `--help` and `--version` and rejects unknown flags
  before anything else happens, so a mistyped flag never opens the database.
- The shutdown hook closes the database after the server has stopped taking
  requests and the in-flight ones have finished.
- `gebweb.cli` serves the app. Run `gebweb dev` (or `geblang run src/main.gb`)
  and it listens on port 8080; `--port`, `--host`, `--self-signed`,
  `--domain` and more are covered in [Deployment](37-deployment.md).

### Trying it

`gebweb.TestClient` sends requests straight to the app without opening a
port. This script builds the app on an in-memory database and exercises each
route:

<!-- doctest:run -->
```gb
import app;
import db;
import gebweb;
import io;

let client = gebweb.TestClient(app.build(db.connect("sqlite", ":memory:")));

let created = client.post("/notes", {"text": "buy milk"});
io.println("${created.status} ${created.headers["Location"]} ${created.text()}");

io.println(client.get("/notes").text());

let missing = client.get("/notes/99");
io.println("${missing.status} ${missing.json()["detail"]}");

let invalid = client.post("/notes", {"text": ""});
io.println("${invalid.status} ${invalid.json()["errors"]}");
```

Output:

```text
201 /notes/1 {"id":1,"text":"buy milk"}
[{"id":1,"text":"buy milk"}]
404 no note 99
422 [{"code": "notBlank", "field": "text", "message": "must not be blank"}]
```

Error responses use the RFC 9457 Problem Details format, so clients get a
`status`, a `title` and a `detail` on every error, plus an `errors` list for
validation failures. The same calls from a shell, with the app running:

```sh
curl -i -X POST localhost:8080/notes -H 'Content-Type: application/json' -d '{"text": "buy milk"}'
curl localhost:8080/notes
```

Put the same calls in test classes to keep them as tests; see
[Testing](14-testing.md).

## How a request is handled

A request passes through these steps:

1. **Routing.** The method and path are matched against the route table and
   the path parameters are extracted. No match is a 404.
2. **Request middleware.** Functions registered with `gebweb.before` run. One
   that returns a response ends the request there. See
   [Middleware](09-middleware.md).
3. **Cache lookup.** A handler marked `@Cache` can be answered from the
   response cache. See [Caching](10-caching.md).
4. **Authentication and authorization.** `@Auth`, `@RequiresRole` and policy
   gates run, and failures become 401 or 403. See
   [Authentication](08-auth.md).
5. **Binding.** Path, query, header and body values are converted to the
   handler's parameter types. A value that cannot be converted is a 400. See
   [Parameter binding](03-parameter-binding.md).
6. **Validation.** A bound body object is checked against its `@Assert` rules.
   Failures are a 422.
7. **The handler** runs.
8. **Response shaping.** A returned value is encoded as JSON; a response
   built with a controller helper is sent as built. See
   [Responses](04-responses.md).
9. **Response middleware.** Functions registered with `gebweb.use` can adjust
   the response, for example to add headers or compress it.

## App construction

`gebweb.app()` creates the app and loads `config/services.yaml` when that file
exists ([services.yaml](29-services-yaml.md)). Called with no arguments, it
discovers controllers: when the app first handles a request, through
`gebweb.cli`, `serve`, `listen`, `TestClient` or `dispatcher`, Gebweb mounts
every loaded class marked `@Controller`. The same sweep registers `@Service`
classes, `@Policy` methods and tagged event, job, message and scheduled
handlers. It runs once per app.

Discovery inspects the classes that are loaded, not the files on disk. A
module that nothing imports is never loaded, so its controllers are never
found. Import every module that holds controllers or handlers from the module
that builds the app, as `app.gb` does above.

The container builds each controller by resolving its constructor
parameters. A parameter typed as a class is resolved recursively: the
container constructs it from its own constructor, so a chain of services
needs no registration. Register what the container cannot construct on its
own, such as a service that needs a database connection, with
`registerInstance` or a factory before the first request. See
[Dependency injection](06a-dependency-injection.md) for factories,
per-request services, configuration parameters and interfaces.

To mount only some controllers, pass them to `gebweb.app`. A nonempty list is
the complete controller set; other `@Controller` classes are not mounted,
though services, policies and handlers are still discovered. The list takes
controller classes (built by the container), already-constructed controller
instances, and `@ApiResource` classes ([Repositories](06-repositories.md)).
`gebweb.app([])` behaves like `gebweb.app()`.

`gebweb.setInfo(app, info)` sets the OpenAPI `info` object, as `build` does
above. The `/openapi.json` and `/docs` routes are mounted by default; turn
them off with `gebweb.app([], {"docs": false})`, keep the spec but drop
SwaggerUI with `{"swaggerUi": false}`, or set `GEBWEB_DOCS=off`. They are off
by default when `GEBWEB_ENV` is `prod` or `production`.
`gebweb.useDocsAuth(app, guard)` keeps them mounted behind a check. See
[OpenAPI and SwaggerUI](13-openapi.md).

## Serving

`gebweb.cli(app, opts)` is the entry point for a deployable app, as in
`main.gb` above. It reads the port, address and TLS settings from command-line
flags, then `GEBWEB_*` environment variables, then the defaults in `opts`;
prints a banner; and on SIGINT or SIGTERM stops accepting requests, finishes
the in-flight ones and runs the shutdown hooks. Its `opts` and flags are
listed in [Deployment](37-deployment.md#options-and-precedence).

The other ways to run an app are for programs that manage their own process:

| Function | Use |
|----------|-----|
| `gebweb.serve(app, address, opts)` | Serve on one address and block until shutdown. No flags, banner or signal handling. |
| `gebweb.listen(app, address, opts)` | Start serving in the background and return a handle for `http.wait` or `http.shutdown`, for example to serve several ports. |
| `gebweb.dispatcher(app)` | Return the in-process request handler that `TestClient` uses. |
| `gebweb.routes(app)` | Return the registered route table. |

`serve` and `listen` take the server options of `http.listen` in `opts`, such
as `tls`, `maxConcurrent`, `queueSize`, `onOverload` and `trustedProxies`.

## Reference

- `gebweb.app(list<any> controllers = [], ?dict<string, any> opts = null): GebwebApp`
- `gebweb.setInfo(GebwebApp app, dict<string, any> info): GebwebApp`
- `gebweb.cli(GebwebApp app, dict<string, any> opts = {}): void`
- `gebweb.cli.checkArgs(list<string> args, dict<string, any> opts = {}): dict<string, any>`
- `gebweb.onShutdown(GebwebApp app, string name, callable hook, dict<string, any> opts = {}): GebwebApp`
- `gebweb.serve(GebwebApp app, string address, ?dict<string, any> opts = null): void`
- `gebweb.listen(GebwebApp app, string address, ?dict<string, any> opts = null): int`
- `gebweb.dispatcher(GebwebApp app): callable`
- `gebweb.routes(GebwebApp app): list<dict<string, any>>`
- `gebweb.TestClient(GebwebApp app): TestClient`

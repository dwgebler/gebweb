# Build a bookmarks app

<!-- doctest:strict -->

Build a small application that stores links in SQLite, renders an HTML form,
and exposes the same data through a JSON API. You will wire typed controllers
to one repository, validate input, carry CSRF tokens and session cookies, and
produce a standalone binary with its templates embedded.

This chapter targets Geblang 1.47.0 or later and Gebweb 1.11.1. Install both
command-line tools as described in [Getting started](01-getting-started.md#install-gebweb).
The completed project lives under `examples/bookmarks` in the framework checkout
and in the manual's [example source browser](examples/bookmarks/index.html).

The example is a local, single-user application. It has no authentication or
per-user ownership. See [Authentication](08-auth.md) before exposing a private
collection to other users.

## Create an empty project

Start in a new directory, with `geblang` and `gebweb` on your PATH:

```sh
mkdir bookmarks
cd bookmarks
mkdir src templates
```

Create `geblang.yaml`:

```yaml
name: bookmarks
version: 0.1.0
source: src
```

Install the framework package into this project:

```sh
geblang install github.com/dwgebler/gebweb@v1.11.1
```

The command adds the dependency to the manifest and pins its commit in
`geblang.lock`. Commit that lockfile with your application. The CLI and framework
package install separately: having `gebweb` on PATH does not by itself make
`import gebweb;` resolve.

The checked-out example instead uses `path: ../..` to reach its sibling
framework source. Keep the installed dependency when following this chapter in
an independent project.

Create these files in order. The application becomes runnable after the entry
point and environment configuration are in place.

### `.gitignore`

```gitignore
.env
.geblang-cache/
.gebweb-cache/
build/
sessions/
*.db
*.db-shm
*.db-wal
```

## Separate stored data from submitted input

`Bookmark` is the value returned by the repository and serialized to JSON.
`NewBookmark` is the write boundary: it declares a URL and title, with
validation on those fields. The database assigns the ID.

The same input type serves the HTML and JSON endpoints. Validation checks the
URL scheme and title before either controller calls the repository.

### `src/bookmark.gb`

<!-- doctest:file bookmark.gb -->
```gb
module bookmark;

export class Bookmark {
    int id;
    string url;
    string title;

    func Bookmark(int id, string url, string title) {
        this.id = id;
        this.url = url;
        this.title = title;
    }
}

export class NewBookmark {
    @Assert.url
    string url;

    @Assert.notBlank
    @Assert.maxLength(200)
    string title;
}
```

## Define the database schema

The migration has a stable ID and SQL body. `conn.migrate` records completed
migrations, so restarting the app retains the table and its data. Add a new
migration for later schema changes rather than editing an already-applied one.

This example applies migrations at startup through the database connection.
It does not use the separate `gebweb migrate` directory workflow.

### `src/migrations.gb`

<!-- doctest:file migrations.gb -->
```gb
module migrations;

/* Schema changes, applied in order and recorded so each runs once. */
export func all(): list<dict<string, any>> {
    return [
        {
            "id": "001_create_bookmarks",
            "sql": "CREATE TABLE bookmarks (id INTEGER PRIMARY KEY AUTOINCREMENT, url TEXT NOT NULL, title TEXT NOT NULL)",
        },
    ];
}
```

## Keep SQL in a repository

The repository receives a `db.Connection`; it does not create or close that
connection. `main` registers its shutdown cleanup. Controllers can therefore use the same
repository implementation with an in-memory database in tests.

SQL values are bound with `?` placeholders. `queryOne` returns the first row or
`null` and releases its cursor; `all()` materializes the list of rows before
mapping them to typed output values.

### `src/bookmark_repository.gb`

<!-- doctest:file bookmark_repository.gb -->
```gb
module bookmark_repository;
import bookmark;
import db;

func toBookmark(dict<string, any> row): bookmark.Bookmark {
    return bookmark.Bookmark(row["id"] as int, row["url"] as string, row["title"] as string);
}

export class BookmarkRepository {
    db.Connection conn;

    func BookmarkRepository(db.Connection conn) {
        this.conn = conn;
    }

    func all(): list<bookmark.Bookmark> {
        return this.conn.query("SELECT * FROM bookmarks ORDER BY id").all().map(toBookmark);
    }

    func find(int id): ?bookmark.Bookmark {
        let row = this.conn.queryOne("SELECT * FROM bookmarks WHERE id = ?", id);
        return row == null ? null : toBookmark(row);
    }

    func add(bookmark.NewBookmark input): bookmark.Bookmark {
        let result = this.conn.exec(
            "INSERT INTO bookmarks (url, title) VALUES (?, ?)",
            input.url,
            input.title
        );
        return bookmark.Bookmark(result["lastInsertId"] as int, input.url, input.title);
    }

    func remove(int id): bool {
        return (this.conn.exec("DELETE FROM bookmarks WHERE id = ?", id)["rowsAffected"] as int) > 0;
    }
}
```

## Add the JSON routes

Both controllers extend `gebweb.Controller`, so each constructor calls
`parent()` before retaining its injected repository. This controller groups
routes under `/api/bookmarks`.

`@Body` binds the input class, and the integer `id` comes from the route.
Returning the list or a `Bookmark` produces JSON. The response helpers set 201
with a Location header for creation and 204 for deletion. `this.notFound`
throws the framework's 404 exception.

### `src/api_controller.gb`

<!-- doctest:file api_controller.gb -->
```gb
module api_controller;
import bookmark;
import bookmark_repository as repo;
import gebweb;

@Controller("/api/bookmarks")
export class ApiController extends gebweb.Controller {
    repo.BookmarkRepository bookmarks;

    func ApiController(repo.BookmarkRepository bookmarks) {
        parent();
        this.bookmarks = bookmarks;
    }

    @Get("/")
    func list(): list<bookmark.Bookmark> {
        return this.bookmarks.all();
    }

    @Get("/{id}")
    func show(int id): bookmark.Bookmark {
        let found = this.bookmarks.find(id);
        if (found == null) {
            this.notFound("no bookmark " + (id as string));
        }
        return found as bookmark.Bookmark;
    }

    @Post("/")
    func create(@Body bookmark.NewBookmark input): gebweb.Response {
        let saved = this.bookmarks.add(input);
        return this.created(saved, "/api/bookmarks/" + (saved.id as string));
    }

    @Delete("/{id}")
    func remove(int id): gebweb.Response {
        if (!this.bookmarks.remove(id)) {
            this.notFound("no bookmark " + (id as string));
        }
        return this.noContent();
    }
}
```

## Add the HTML page and form handler

The page loads the same repository data. Passing `request` into `this.view`
lets the framework inject CSRF, flash messages, old input and validation errors.

On success, the form handler saves the bookmark and sends a 303 redirect to
`/`. The success message is persisted in the session for the next page render.

### `src/pages_controller.gb`

<!-- doctest:file pages_controller.gb -->
```gb
module pages_controller;
import bookmark;
import bookmark_repository as repo;
import gebweb;

@Controller("")
export class PagesController extends gebweb.Controller {
    repo.BookmarkRepository bookmarks;

    func PagesController(repo.BookmarkRepository bookmarks) {
        parent();
        this.bookmarks = bookmarks;
    }

    @Get("/")
    func index(gebweb.Request request): gebweb.Response {
        return this.view(request, "bookmarks.html", {"bookmarks": this.bookmarks.all()});
    }

    @Post("/bookmarks")
    func add(gebweb.Request request, @Body bookmark.NewBookmark input): dict<string, any> {
        this.bookmarks.add(input);
        return this.redirectWithFlash(request, "/", "success", "Bookmark saved.");
    }
}
```

The template posts URL-encoded fields to `/bookmarks`. The hidden `_csrf`
field carries the token paired with the browser's CSRF cookie. `old` and
`errors` are supplied after a failed HTML submission; `default('')` leaves
an untouched form empty. Keep the default HTML escaping enabled for both
stored values and old input.

### `templates/bookmarks.html`

<!-- doctest:file templates/bookmarks.html -->
```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Bookmarks</title>
</head>
<body>
  <h1>Bookmarks</h1>
  {% for msg in flashes.success %}
    <p class="notice">{{ msg }}</p>
  {% endfor %}
  <ul>
  {% for b in bookmarks %}
    <li><a href="{{ b.url }}">{{ b.title }}</a></li>
  {% endfor %}
  </ul>
  <form method="post" action="/bookmarks">
    <input type="hidden" name="_csrf" value="{{ csrf }}">
    <label>URL <input name="url" value="{{ old.url|default('') }}"></label>
    {% if errors.url %}<p class="error">{{ errors.url|first }}</p>{% endif %}
    <label>Title <input name="title" value="{{ old.title|default('') }}"></label>
    {% if errors.title %}<p class="error">{{ errors.title|first }}</p>{% endif %}
    <button type="submit">Save</button>
  </form>
</body>
</html>
```

## Build the app from explicit dependencies

The factory accepts the connection, CSRF secret and session directory instead
of reading process configuration. Tests can supply temporary values without
changing the route wiring.

Register the repository instance before binding controllers so constructor
injection can resolve it. The imports load the controller classes; discovery
uses loaded classes, not a scan of source directories. The `@Controller` decorators mark both the prefixed API controller and the
root pages controller for discovery; no explicit controller list is needed.

### `src/app.gb`

<!-- doctest:file app.gb -->
```gb
/** @allowUnusedImports */
module app;
import api_controller;
import bookmark_repository as repo;
import db;
import gebweb;
import migrations;
import pages_controller;
import web.session as session;

/* Builds the application around its dependencies, so tests can pass an in-memory database. */
export func build(db.Connection conn, string csrfSecret, string sessionDir): gebweb.GebwebApp {
    if (csrfSecret.trim().isEmpty()) {
        throw ValueError("CSRF_SECRET must not be empty");
    }
    conn.migrate(migrations.all());
    let app = gebweb.app();
    gebweb.registerInstance(app, repo.BookmarkRepository, repo.BookmarkRepository(conn));
    gebweb.useViews(app, "templates");
    gebweb.useSession(app, session.fileSessionStore(sessionDir, 86400));
    gebweb.useCsrf(app, {"secret": csrfSecret});
    return app;
}
```

## Configure and run the entry point

The entry point checks CLI arguments before opening resources, loads `.env`
when present, requires the three configuration values, and hands the completed
app to `gebweb.cli`. Its named shutdown hook closes the connection after
in-flight requests drain.
Values loaded by `dotenv.loadAndApply` overwrite matching process environment
values. Keep that precedence in mind when using a local `.env`.

The CLI supplies the standard server flags, environment settings and graceful
shutdown behavior. The app defaults to loopback port 8080.

### `src/main.gb`

<!-- doctest:file main.gb -->
```gb
module main;
import app;
import db;
import dotenv;
import gebweb;
import gebweb.cli as cli;
import io;
import secrets;

export func main(list<string> args): int {
    cli.checkArgs(args, {"name": "bookmarks"});
    if (io.exists(".env")) {
        dotenv.loadAndApply(".env");
    }
    let conn = db.connect("sqlite", secrets.requireEnv("DATABASE_URL"));
    let application = app.build(
        conn,
        secrets.requireEnv("CSRF_SECRET"),
        secrets.requireEnv("SESSION_DIR")
    );
    gebweb.onShutdown(application, "database", func(): void {
        conn.close();
    });
    gebweb.cli(application, {"name": "bookmarks", "host": "127.0.0.1"});
    return 0;
}
```

Create the environment example below, then copy it to `.env`. Fill in
`CSRF_SECRET` with a random value, for example the output of
`openssl rand -hex 32`. `secrets.requireEnv` rejects an unset variable;
the app factory additionally rejects empty or whitespace-only CSRF secrets.

### `.env.example`

```dotenv
DATABASE_URL=bookmarks.db
SESSION_DIR=sessions
CSRF_SECRET=
```

```sh
cp .env.example .env
# Edit .env and fill in CSRF_SECRET.
geblang check --strict src/
geblang -m main
```

Open <http://127.0.0.1:8080/>. Submit a URL and title: the browser follows the
303 redirect, displays the saved bookmark and shows the success message once.
Use `GEBWEB_PORT` or `--port` to change the port.

To exercise server-side validation from the form, submit an invalid URL or an
empty title. The response depends on the client's Accept header:

| Request | Result |
| --- | --- |
| Valid HTML form with CSRF cookie and token | 303 to `/`, saved bookmark and one-shot success flash. |
| Invalid input with `Accept: text/html` | 303 back to the referring page, with old input and field errors for one render. |
| Invalid input with `Accept: application/json` | 422 Problem Details with validation errors. |
| Write without a valid matching CSRF cookie and token | 403 before the write handler runs. |
| Fetch an unknown ID, or delete one with a valid CSRF cookie/token pair | 404. |

The redirect target for an invalid form comes from Referer, falling back to
the submitted path. A browser submitting the root page normally sends that
Referer. Explicit API and test clients choose their headers themselves.

## Call the JSON API with CSRF protection

The API keeps CSRF protection enabled. First fetch the form with a cookie jar,
then send its hidden token in `X-CSRF-Token` together with that cookie:

```sh
curl -sS -c cookies.txt http://127.0.0.1:8080/ > form.html
TOKEN=$(sed -n 's/.*name="_csrf" value="\([^"]*\)".*/\1/p' form.html)
curl -i -b cookies.txt \
  -H "X-CSRF-Token: $TOKEN" \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://geblang.com","title":"Geblang"}' \
  http://127.0.0.1:8080/api/bookmarks/
curl -sS http://127.0.0.1:8080/api/bookmarks/
```

Creation returns 201 with the assigned ID and a Location header such as
`/api/bookmarks/1`. The list endpoint returns an array. Deleting uses
`DELETE /api/bookmarks/{id}` with the same CSRF cookie/header pair; it returns
204 on success and 404 if the record is missing.

## Test the application through its public routes

The completed example's
[full test fixture](examples/bookmarks/src/app_test.html) is
`examples/bookmarks/src/app_test.gb`. Copy that file into your project's `src/`
to run the same suite. Its fixture creates a fresh in-memory SQLite connection
and temporary session directory, then closes and removes them after each test.
The following runnable check uses the same factory and templates. It obtains
the token from the rendered form, sends the matching cookie on writes, checks
validation and exercises create, list, fetch and delete through public routes.
The token is deliberately reused only within this one test client.

<!-- doctest:run -->
```gb
import app;
import db;
import gebweb;
import io;
import log;
import re;

let conn = db.connect("sqlite", ":memory:");
string directory = io.tempDir("bookmarks-manual-");
try {
    let application = app.build(conn, "example-only-csrf-secret", directory);
    gebweb.useLogger(application, log.stderr());
    let client = gebweb.TestClient(application);
    let page = client.get("/");
    let tokenField = re.match('name="_csrf" value="([^"]+)"', page.text()) as dict<string, any>;
    string token = tokenField["groups"][1] as string;
    dict<string, any> headers = {
        "Cookie": "geb_csrf=" + token,
        "X-CSRF-Token": token,
        "Accept": "application/json",
    };
    io.println(page.status);
    io.println(
        client.post("/api/bookmarks/", {"url": "https://geblang.com", "title": "Blocked"}).status
    );
    io.println(
        client.request("POST", "/api/bookmarks/", {"url": "invalid", "title": ""}, headers).status
    );
    let created = client.request(
        "POST",
        "/api/bookmarks/",
        {"url": "https://geblang.com", "title": "Geblang"},
        headers
    );
    io.println(created.status);
    io.println(created.headers["Location"]);
    io.println(client.get("/api/bookmarks/1").json()["title"]);
    io.println((client.get("/api/bookmarks/").json() as list<any>).length());
    io.println(client.request("DELETE", "/api/bookmarks/1", null, headers).status);
    io.println(client.get("/api/bookmarks/1").status);
} finally {
    conn.close();
    for (name in io.listDir(directory)) {
        io.remove(directory + "/" + (name as string));
    }
    io.remove(directory);
}
```

```text
200
403
422
201
/api/bookmarks/1
Geblang
1
204
404
```

`TestClient` dispatches in process. Carry cookies explicitly: the fixture's
`request` helper collects Set-Cookie values and sends Cookie on
the next request. JSON writes obtain a CSRF token and send the header; the form
tests send the hidden field instead. This keeps the protection active during
both kinds of test.

The suite checks empty-secret rejection, create/list/fetch/delete, 404s,
422 validation, missing and
mismatched CSRF tokens, a successful form redirect and one-shot flash,
retained escaped input, and one-shot validation errors. Run it on both runtimes:

```sh
geblang check --strict src/
geblang test src/
geblang test --runtime=evaluator src/
```

Both runs should report nine passed tests and zero failures. The checked-out
example also provides `bash verify.sh`. That script copies the app to a temporary
directory, runs real HTTP requests against VM and evaluator servers, confirms
warm-cache reuse and persistence across restarts, then builds and runs the
standalone app in an isolated directory. It requires Bash, curl and GNU find;
its README explains binary-path and test-port overrides.

## Build and run a standalone binary

Stop the development server, then build:

```sh
gebweb build --no-swagger
./build/app
```

The application does not use SwaggerUI, so `--no-swagger` avoids downloading
those assets. The build includes the application, framework and templates.
The database and session files stay external. A copied binary can run from
another directory with `DATABASE_URL`, `SESSION_DIR` and `CSRF_SECRET` supplied
in that directory's `.env` or process environment.

The same app factory now serves three callers: the real entry point, the
in-process tests and the HTTP verification script. Apply
[Application best practices](39-best-practices.md) as the app grows. See
[Forms](25-forms.md), [Dependency injection](06a-dependency-injection.md),
[Testing](14-testing.md) and [Deployment](37-deployment.md) for the contracts
behind this example.

The app is local and single-user. Keep it bound to loopback until you add
authentication, authorization and ownership checks. For HTTPS deployment,
configure secure session/CSRF cookie policies and trusted proxy handling as
described in [security](27-security.md) and [deployment](37-deployment.md).

## Reference

| Part | Responsibility |
|------|----------------|
| `NewBookmark` | Validate submitted URL and title before repository writes. |
| `BookmarkRepository` | Own SQL operations through an injected connection. |
| `app.build` | Apply migrations and configure DI, views, sessions and CSRF. |
| `main` | Check arguments, load configuration and register connection cleanup. |
| `app_test.gb` | Test browser and API behavior with isolated storage. |
| `verify.sh` | Verify real HTTP, warm caches, restarts and the embedded binary. |

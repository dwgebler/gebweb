# Bookmarks

A small SQLite-backed app with a server-rendered form and a JSON API. It uses
Geblang 1.47.0 or later and Gebweb 1.11.1. The manifest resolves the framework
from this checkout using `path: ../..`.

Follow the [build-an-app tutorial](../../docs/38-building-bookmarks.md) for a
walkthrough of the production files, request flow, tests and standalone build.

Run the commands below from this directory, with `geblang` and `gebweb` on PATH.
From the monorepo, `make build` builds `geblang` and `build/gebweb`.

## Start the app

Copy `.env.example` to `.env`. Set `CSRF_SECRET` to a random value, for example
the output of `openssl rand -hex 32`. The app rejects empty or whitespace-only
CSRF secrets. Keep the SQLite database and session
directory outside version control; the included `.gitignore` excludes them.

```sh
cp .env.example .env
# Edit .env and fill in CSRF_SECRET.
geblang -m main
```

Open <http://127.0.0.1:8080/>. Startup applies the migration automatically.
`DATABASE_URL` is a SQLite filename; `SESSION_DIR` holds expiring file sessions.
The app reads `.env` when present. Process environment values also work.
Values loaded from `.env` overwrite matching environment values.
Use `GEBWEB_PORT` or `--port` to choose another port.

## Behavior

| Route | Behavior |
| --- | --- |
| `GET /` | Render bookmarks, a CSRF-protected form and one-shot form/flash state. |
| `POST /bookmarks` | Validate, save and redirect with a flash; invalid HTML input redirects with field errors and old input. |
| `GET /api/bookmarks/` | List bookmarks as JSON. |
| `POST /api/bookmarks/` | Create a bookmark; return 201 and a Location header. |
| `GET /api/bookmarks/{id}` | Return a bookmark, or 404. |
| `DELETE /api/bookmarks/{id}` | Delete a bookmark; return 204, or 404 when missing. |

All writes require CSRF validation. A browser sends the hidden form token and
cookie. An API caller first fetches `/` with a cookie jar, then sends that
cookie and its token in `X-CSRF-Token`. JSON clients receive 422 on validation
failure. This example has no authentication or per-user ownership.

## Test

```sh
geblang check --strict src/
geblang test src/
geblang test --runtime=evaluator src/
```

The tests use an in-memory database and a temporary session directory,
closed and removed after each test. The test client explicitly carries cookies
between requests, as a browser does.

`bash verify.sh` also checks real HTTP on both runtimes, a second VM launch
using its cache, persistence across restarts, and an isolated standalone binary.
It also checks that CLI help works without configuration and before discovered
controllers are constructed. It requires Bash, curl and GNU find, copies the app to a temporary directory,
uses temporary data, and defaults to port 18092. Local `.env` files are excluded.
Override paths with `GEBLANG_BIN` and `GEBWEB_BIN`, and the test port with
`BOOKMARKS_TEST_PORT`. It refuses to reuse an active HTTP port.

## Build

```sh
gebweb build --no-swagger
./build/app
```

The binary embeds the application, framework and templates. SwaggerUI is unused,
so `--no-swagger` avoids downloading it. Run the binary from any directory with
`DATABASE_URL`, `SESSION_DIR` and `CSRF_SECRET` set, or supply a `.env` in that
directory. Database files and sessions remain external to the binary.

## Code layout

- `src/app.gb`: builds an app from the connection, CSRF secret and session path.
- `src/bookmark.gb`: output model and validated input type.
- `src/bookmark_repository.gb`: parameterized SQL and row-to-model conversion.
- `src/migrations.gb`: the initial schema migration.
- `src/pages_controller.gb` and `templates/bookmarks.html`: HTML form flow.
- `src/api_controller.gb`: JSON routes over the same repository.
- `src/main.gb`: environment configuration, connection ownership and serving.
- `src/app_test.gb`: API and browser-flow tests.

See the [forms chapter](../../docs/25-forms.md) and
[testing chapter](../../docs/14-testing.md) for the framework contracts.

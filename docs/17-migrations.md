# Database migrations

Gebweb ships a built-in schema migration runner driven by the
`gebweb` CLI. Migrations are plain `.sql` files versioned by
timestamp; the runner tracks applied versions in a
`gebweb_schema_migrations` table on the same database.

Supported drivers (auto-selected from the `DATABASE_URL` scheme):

- `sqlite://` - SQLite via `modernc.org/sqlite`
- `postgres://` / `postgresql://` - Postgres via `jackc/pgx`
- `mysql://` - MySQL / MariaDB via `go-sql-driver/mysql`

## Quickstart

    export DATABASE_URL="sqlite://./app.db"

    gebweb migrate create add_users
    # -> wrote migrations/20260101120000_add_users.sql

    # edit migrations/20260101120000_add_users.sql ...

    gebweb migrate up
    # -> applied 20260101120000_add_users

    gebweb migrate status
    # applied   20260101120000_add_users

    gebweb migrate down --steps 1
    # -> rolled back 20260101120000_add_users

## File format

Each migration is a single `.sql` file under `migrations/` named
`<YYYYMMDDHHMMSS>_<name>.sql`. The file contains two labelled
blocks:

    -- +gebweb up
    CREATE TABLE users (
        id   INTEGER PRIMARY KEY,
        name TEXT NOT NULL
    );

    -- +gebweb down
    DROP TABLE users;

Anything before the first marker is ignored, so the file can
carry a comment header.

The `up` block is required to run `migrate up`; the `down` block
is required to run `migrate down` against that version. Each
block is executed inside a transaction along with the schema
table update, so a failure rolls the whole step back.

## Subcommands

### `gebweb migrate create <name>`

Writes a new migration file with a fresh timestamp and a template
body. The name is lower-cased and non-alphanumeric characters are
replaced with underscores, so `"Add Users Table"` becomes
`add_users_table`.

### `gebweb migrate up [--target <version>]`

Applies every migration whose version is not yet recorded in
`gebweb_schema_migrations`, in ascending order. Stops early when
`--target <version>` is given (the target is inclusive).

### `gebweb migrate down [--steps N]`

Rolls back the most recently applied migration. `--steps N` rolls
back the last N applied migrations in reverse order. Defaults to
1.

### `gebweb migrate status`

Prints every migration file under `migrations/`, marked `applied`
or `pending`.

## Configuration

The runner reads `DATABASE_URL` from the process environment. URL
shapes:

    sqlite://./app.db                          # relative path
    sqlite:///var/db/app.db                    # absolute path
    postgres://user:pass@host:5432/dbname
    mysql://user:pass@host:3306/dbname

For SQLite the URL after the scheme is the file path; for
Postgres the URL is passed straight to `pgx`; for MySQL the URL
is converted to the `user:pass@tcp(host:port)/dbname` form that
`go-sql-driver/mysql` expects.

## Conventions

- Treat migration files as append-only once they've shipped.
  Editing an already-applied migration won't re-apply it; create
  a new file instead.
- Keep migrations small. One conceptual change per file makes
  rollback predictable.
- Use deterministic SQL (no `CURRENT_TIMESTAMP` for default values
  you intend to backfill from app code) so dev and prod converge.
- Commit migration files alongside the code that depends on them.

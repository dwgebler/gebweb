# Database migrations

<!-- doctest:strict -->

Migrations bring a database schema up to the version the application needs.
Gebweb applications can apply migrations through their existing `db.Connection`
or use the `gebweb migrate` CLI to operate on versioned SQL files. Choose one
owner for each schema change: the two runners have different file formats and
independent migration ledgers.

## Applying application migrations in process

For an app that owns its setup, keep the ordered migrations in a module and
apply them in `build(conn)` before constructing repositories and handling
requests. This also lets tests build the same schema on an in-memory database
without starting another process.

Put this in `src/app_schema.gb`:

<!-- doctest:file app_schema.gb -->
```gb
export func all(): list<dict<string, any>> {
    return [
        {
            "id": "001_create_widgets",
            "sql": "CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT NOT NULL)",
        },
        {
            "id": "002_unique_widget_names",
            "sql": "CREATE UNIQUE INDEX widgets_name ON widgets(name)",
        },
    ];
}
```

This SQLite example applies both migrations, inserts data, then runs setup
again. The second call skips the recorded migrations and keeps the data:

<!-- doctest:run -->
```gb
import db;
import io;
import app_schema;

let conn = db.connect("sqlite", ":memory:");
let first = conn.migrate(app_schema.all());
io.println(first["applied"].length());
io.println(first["skipped"].length());
conn.exec("INSERT INTO widgets (id, name) VALUES (?, ?)", [1, "Notebook"]);

let second = conn.migrate(app_schema.all());
io.println(second["applied"].length());
io.println(second["skipped"].length());
io.println(conn.queryOne("SELECT COUNT(*) AS total FROM widgets")["total"]);
conn.close();
```

```text
2
0
0
2
1
```

| In-process entry / result | Meaning |
|---------------------------|---------|
| `id` | Required non-empty string; identifies the change in `geblang_migrations`. |
| `sql` | Required non-empty SQL string to execute for that change. |
| Result `applied` | List of IDs applied by this call, in supplied order. |
| Result `skipped` | List of IDs already recorded, in supplied order. |

`conn.migrate` uses the supplied list order, not a filename sort. On SQLite,
it applies the batch and updates its ledger in one transaction. If any change
fails, the batch rolls back. There is no `down` block or automatic rollback
command for this API; deploy a new forward migration or use your recovery plan.

For example, this failed batch rolls back both the schema and its version
records. Retrying the valid migrations then applies both from scratch:

<!-- doctest:run -->
```gb
import db;
import io;
import app_schema;

let conn = db.connect("sqlite", ":memory:");
let broken = app_schema.all();
broken.push({"id": "003_bad_insert", "sql": "INSERT INTO missing_table VALUES (1)"});
try {
    conn.migrate(broken);
} catch (Error error) {
    io.println("migration failed");
}
let retry = conn.migrate(app_schema.all());
io.println(retry["applied"].length());
io.println(retry["skipped"].length());
conn.close();
```

```text
migration failed
2
0
```

Keep the connection in the repositories that need it, register those instances
before discovery, and close it through `gebweb.onShutdown` in the entry point.
See [Getting started](01-getting-started.md) for the complete
`main(args)` -> `cli.checkArgs` -> `build(conn)` -> `gebweb.cli` pattern.
Do not shell out to the CLI to prepare a private in-memory test database: a
separate process opens a different database.

## CLI quickstart

The CLI runner reads SQL files from `./migrations/` and records their versions
in `gebweb_schema_migrations`. Run it from the project directory, with
`DATABASE_URL` in the process environment:

```sh
export DATABASE_URL='sqlite:./app.db'
gebweb migrate create add_users
```

`create` writes `migrations/<UTC timestamp>_add_users.sql`. Edit that file
before running `up`. For example, a file named
`migrations/20260101120000_add_users.sql` could contain:

<!-- doctest:file migrations/20260101120000_add_users.sql -->
```sql
-- +gebweb up
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
);

-- +gebweb down
DROP TABLE users;
```

Then:

```sh
gebweb migrate up
gebweb migrate status
gebweb migrate down --steps 1
```

For that single file, the output is:

```text
applied 20260101120000_add_users
applied   20260101120000_add_users
rolled back 20260101120000_add_users
```

The last command drops the users table, including its data. Run it only when
that rollback is intended. Use a temporary database to practise the workflow.

## SQL file format

Use `<version>_<name>.sql` under `migrations/`. The generator uses a 14-digit
UTC timestamp, `YYYYMMDDHHMMSS`, for the version. The runner orders version
strings lexically; keep the same fixed-width format across a project and give
each migration a unique version. The MySQL CLI ledger accepts version strings
up to 255 characters; generated timestamps fit comfortably within that limit.

- `-- +gebweb up` begins the forward SQL.
- `-- +gebweb down` begins the rollback SQL.
- Lines before the first marker are ignored, so the file may have a header.
- A non-empty `up` block is required when applying that file; a non-empty
  `down` block is required when rolling it back.

The version prefix, rather than the whole filename or its contents, identifies
a migration in the CLI ledger. Once a version is applied, changing its SQL does
not run it again. Keep shipped migrations unchanged and add a new one instead.
Do not give two files the same timestamp prefix, even if their names differ.

Each CLI migration runs in its own transaction with its ledger update. On
SQLite and PostgreSQL, transactional DDL lets a failed step roll back the schema
change and the ledger entry together. A later failure does not roll back earlier
successful steps from the same `up` command. Database-specific DDL transaction
rules still apply; do not assume every driver can undo schema changes.

## CLI commands and options

| Command / option | Default | Effect |
|------------------|---------|--------|
| `gebweb migrate create NAME` | No default name | Create a timestamped file with `up` and `down` markers; no database needed. |
| `gebweb migrate up` | All pending files | Apply unrecorded versions in ascending version order. |
| `up --target VERSION` | No upper bound | Apply pending versions up to and including this version string. |
| `gebweb migrate down` | One applied file | Roll back applied files in descending version order. |
| `down --steps N` | `1` | Roll back up to `N` applied files; `N` must be a positive integer. |
| `gebweb migrate status` | All local migration files | Print each file as `applied` or `pending`. |
| `DATABASE_URL` | Required except for `create` and help | Select the database connection. |

`create` lowercases the name, changes spaces and hyphens to underscores and
drops other characters outside ASCII letters, digits and underscores. For
example, `"Add Users Table"` becomes `add_users_table`.

Pass only the version prefix to `--target`, not the name or `.sql` suffix:

```sh
gebweb migrate up --target 20260101120000
```

This is an inclusive upper bound, not a requirement that a file with that exact
version exist. `status` lists local files; it does not report ledger entries
whose files have been removed. `down` likewise needs the applied migration's
file and its rollback SQL. Preserve those files in the deployed migration set.

## Database configuration

The CLI reads `DATABASE_URL` directly from its environment; it does not load
`.env`, `services.yaml` or the application's `build(conn)` factory. Arrange for
your shell or deployment system to supply the URL explicitly.

| URL form | CLI driver / interpretation |
|----------|-----------------------------|
| `sqlite:./app.db` or `sqlite://./app.db` | SQLite, relative to the CLI working directory. |
| `sqlite:///var/db/app.db` | SQLite, absolute file path. |
| `sqlite::memory:` | Private SQLite database for this process only. |
| `postgres://user:pass@host:5432/dbname` | PostgreSQL through pgx. |
| `postgresql://user:pass@host:5432/dbname` | Same PostgreSQL driver. |
| `mysql://user:pass@host:3306/dbname` | MySQL URL translated into the driver's connection string. |

The CLI URL is a connection convention of this runner. An app's
`db.connect(driver, connectionString)` receives a driver and connection string
separately; do not assume its configuration is automatically shared with the
CLI. In particular, the `.env` generated by `gebweb new` is loaded by the app's
entry point, not by `gebweb migrate`.

Use SQL for the chosen database. The runner does not translate SQLite
`AUTOINCREMENT`, PostgreSQL types, MySQL DDL or SQL dialect differences.

## Deployment and recovery

- Commit migrations with the code that requires them. Test the complete set
  against a fresh database and an upgrade from the previous application version.
- Have one deployment step own migration execution. Neither runner provides a
  deployment-wide migration lock; do not start competing migration processes.
- For rolling deployments, make the first schema change compatible with both
  old and new application versions. Remove old columns only after old processes
  have stopped using them.
- Keep transactions small, and plan for locks and large data backfills. Moving
  a large table's data during every web-process startup can delay availability.
- Test rollback SQL with representative data, and take backups where data loss
  matters. A `down` block is code, not a backup or a guarantee of reversibility.
- Use one migration ledger for each change. Running the CLI and then applying
  the same `CREATE TABLE` through `conn.migrate` does not make either runner
  aware of the other's version history.

The durable job engine has a separate maintenance migration API,
`gebweb.migrateJobs(conn, opts)`. It is not a general application-schema runner;
see [Background jobs](18-background-jobs.md) before changing an existing job
queue's schema.

## Reference

| Surface | Source of changes | Ledger | Rollback support |
|---------|-------------------|--------|------------------|
| `conn.migrate(migrations)` | Ordered `{id, sql}` values in application code | `geblang_migrations` | No automatic down migration. |
| `gebweb migrate up/down/status` | Versioned SQL files in `./migrations/` | `gebweb_schema_migrations` | Explicit SQL `down` blocks. |

Related chapters: [Getting started](01-getting-started.md),
[Repositories](06-repositories.md), [Database connections](06b-database-connections.md),
[The CLI](15-cli.md), [Background jobs](18-background-jobs.md), and
[Deployment](37-deployment.md).

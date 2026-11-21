# Database connections and concurrency

A Gebweb app shares one database connection pool across every request,
background job and scheduled task. Gebweb does not open or own the pool: your
application creates it at startup, registers it with the DI container, and
closes it at shutdown. This chapter covers that wiring, how many requests and
connections to allow, and what happens when traffic outnumbers connections.

The pool itself is Geblang's `db.Connection`. Its options, defaults and
contention statistics are described in the Geblang manual's database reference
(see "Pool sizing and concurrency"); this chapter covers how a Gebweb app uses
it.

## One pool per process

`db.Connection` is a pool of connections, safe for concurrent use. Each query
borrows a connection and returns it when the query finishes, so concurrent
requests querying the same `db.Connection` run in parallel up to the pool's
`maxOpenConns`. Create it once, before the app starts serving:

```gb
let conn = db.Connection({
    "driver": "postgres",
    "dsn": secrets.requireEnv("DATABASE_URL"),
    "maxOpenConns": 16,
    "connMaxLifetimeMs": 1800000,
});
```

Do not open a connection per request, per repository or per controller. Each
one pays the connect and authentication cost, and the number of server
connections then grows with traffic instead of being bounded by the pool.

## Registering the pool

The container resolves services by class. A native class such as
`db.Connection` is a type name rather than a class value, so it cannot be
registered directly. Wrap the pool in a small application class and register
the wrapper with a factory. Everything that needs the database then depends on
the wrapper and is autowired:

```gb
module app;
import db;
import gebweb;

export class Database {
    db.Connection conn;
    func Database(db.Connection conn) {
        this.conn = conn;
    }
}

export class NoteRepository {
    Database database;
    func NoteRepository(Database database) {
        this.database = database;
    }

    func count(): int {
        let row = this.database.conn.queryOne("SELECT count(*) AS c FROM notes");
        return (row as dict<string, any>)["c"] as int;
    }
}

@Controller("/notes")
export class NoteController {
    NoteRepository notes;
    func NoteController(NoteRepository notes) {
        this.notes = notes;
    }

    @Get("/count")
    func count(): dict<string, any> {
        return {"count": this.notes.count()};
    }
}

export func build(db.Connection conn): gebweb.GebwebApp {
    let application = gebweb.app([]);
    gebweb.register(application, Database, func(): Database { return Database(conn); });
    gebweb.addControllers(application, [NoteController]);
    return application;
}
```

`gebweb.register` binds `Database` to the factory. The container calls it on
the first resolve and returns the same instance afterwards, so every service
shares one wrapper and one pool. `NoteRepository` and `NoteController` are not
registered at all: the container builds them by resolving their constructor
parameters.

Register the pool with the shared lifetime (the default), never with
`registerPerRequest`. A per-request factory would run once per request, and
the pool it captures is shared anyway, so it adds an object per request and
nothing else. See [Dependency injection](06a-dependency-injection.md#lifecycle-model).

A wrapper is also a natural home for app-wide database helpers, and lets tests
substitute a different `Database` with `gebweb.registerInstance`.

### Repositories and transactions

The pool is shared, so the objects around it must hold no per-request state:

- Keep repositories stateless apart from their `Database`. A field that
  records "the current user" or "the current transaction" is shared by every
  concurrent request.
- Keep a transaction local to the operation that needs it. Begin it, pass the
  `db.Transaction` to the collaborators that must take part, and commit or roll
  back before returning.
- Queries made through `database.conn` while a transaction is open use another
  connection and are not part of that transaction.
- Make network calls (HTTP clients, mail, message brokers) before the
  transaction begins or after it ends. A transaction holds its connection for
  its whole lifetime.
- Use `queryOne` for single rows, and close a streaming cursor on early exits.
  An open cursor holds its connection until it is exhausted or closed.

```gb
func transfer(Database database, int from, int to, int amount): void {
    let tx = database.conn.begin(2000);
    try {
        tx.exec("UPDATE accounts SET balance = balance - ? WHERE id = ?", amount, from);
        tx.exec("UPDATE accounts SET balance = balance + ? WHERE id = ?", amount, to);
        tx.commit();
    } catch (Error e) {
        tx.rollback();
        throw e;
    }
}
```

## Requests, connections and queues

Gebweb serves each request on its own goroutine, with no limit unless you set
one. Three independent limits apply between a client and the database:

```
client
  |
  v
server limit       maxConcurrent / queueSize / onOverload    (server option)
  |                at the cap: reject 503, queue, or drop
  v
admission gate     gebweb.useAdmissionControl                (app option)
  |                at the cap: 503 with Retry-After, no queue
  v
handler
  |
  v
database pool      maxOpenConns                              (db.Connection option)
                   at the cap: wait for a free connection
```

### Server limit

The HTTP server's `maxConcurrent` caps handlers in flight. `queueSize` holds
further requests until a slot opens, and `onOverload` chooses what happens
when the queue is full too: `"reject"` (503), `"wait"` (queue without limit)
or `"drop"` (close the connection). Pass them through the entrypoint's
`server` options, or the options of `gebweb.serve`:

```gb
gebweb.cli(application, {
    "port": 8080,
    "server": {"maxConcurrent": 128, "queueSize": 256, "onOverload": "reject"},
});
```

This is the only limit that can queue requests, and the only one that applies
before a request reaches the app. WebSocket connections hold a slot for their
whole lifetime.

### Admission gate

`gebweb.useAdmissionControl(app, {"maxConcurrent": N})` counts requests
inside the dispatcher. Above `N`, a request gets an immediate 503 with
`Retry-After: 1` before any middleware or handler runs; `onOverload` replaces
that response. It never queues. See
[Middleware](09-middleware.md#admission-control).

Use it when you want the app to shed load itself, for example with a custom
overload response, or when the server limit is set by a platform you do not
control. When you set both, keep the admission gate below the server's
`maxConcurrent`, or it never triggers.

### Database pool

Requests that pass both limits and find every connection busy wait in the
pool. An untimed query waits as long as it takes. A timed one
(`queryWithTimeout`, `execWithTimeout`, `begin(timeoutMs)`) counts the wait
against its deadline and throws `TimeoutError`, which surfaces as a 500 unless
you catch it. Mapping it to a 503 tells clients and load balancers to retry:

```gb
import gebweb.errors as errors;

@Get("/{id}")
func show(int id): dict<string, any> {
    ?dict<string, any> row = null;
    try {
        row = this.database.conn.queryWithTimeout(500, "SELECT id, total FROM orders WHERE id = ?", id).first();
    } catch (TimeoutError e) {
        throw errors.HttpException(503, "database busy", "Service Unavailable");
    }
    if (row == null) {
        throw gebweb.notFound("no order ${id}");
    }
    return row as dict<string, any>;
}
```

### Choosing the numbers

Size each limit for what it protects:

| Limit | Protects | Size it from |
|-------|----------|--------------|
| `maxOpenConns` | The database | The server's connection limit, divided by every process that opens a pool (see below) |
| Server `maxConcurrent` | Memory and file descriptors | Per-request memory under load; usually several times `maxOpenConns` |
| `queueSize` | Latency | How long a client should wait: roughly the acceptable wait divided by typical request time, times `maxConcurrent` |
| Database call deadlines | Request latency | Shorter than the client's or load balancer's timeout |

`maxConcurrent` can safely exceed `maxOpenConns`. A request spends part of its
time rendering, serialising and waiting on other services, and many routes do
not touch the database. Set it equal to `maxOpenConns` only when nearly every
request holds a connection for most of its life.

Pick the overload behaviour by what clients should see at the cap. Rejecting
turns a spike into fast 503s that a load balancer or client can retry against
another instance. Queuing turns it into latency, which suits a single instance
whose clients would rather wait.

## Background jobs and workers

`gebweb.useJobs(application, conn)` keeps its job table in the database behind
`conn`. An in-process worker (`{"inProcessWorker": true}`) shares the app's
pool, so its jobs compete with requests for connections. A worker running
`runWorker(application, {"maxConcurrency": 4})` runs up to four jobs at once,
each borrowing connections as its queries run.

A worker started as its own process (`gebweb worker`, or a second container
running the app in worker mode) creates its own `db.Connection` and its own
pool. Count it in the connection budget:

```
total connections = (app instances x app maxOpenConns)
                  + (worker processes x worker maxOpenConns)
                  + scheduler, migrations, admin tools
                  <= database connection limit
```

A worker process usually runs far fewer concurrent operations than a web
process, so give it a smaller pool, sized from its `maxConcurrency`.

## Shutdown

Close the pool after the server and workers have stopped using it. The
entrypoint (`gebweb.cli`) returns once SIGINT or SIGTERM has drained in-flight
requests, so a `defer` in `main` closes the pool at the right moment:

```gb
export func main(list<string> args): int {
    let conn = db.Connection({
        "driver": "postgres",
        "dsn": secrets.requireEnv("DATABASE_URL"),
        "maxOpenConns": 16,
    });
    defer conn.close();
    gebweb.cli(app.build(conn), {"name": "notes"});
    return 0;
}
```

Handlers and repositories never close the shared pool.

## Watching the pool

`conn.stats()` reports the pool's state. Compare `waitCount` and
`waitDurationMs` between samples: steady growth while the database has spare
capacity means the pool is too small or connections are held too long; growth
while the database is saturated means the pool is too large for it, and the
fix is a lower `maxOpenConns` with a lower request limit in front of it. Server
limits are reported by `http.serverStats(handle)`: `active`, `queued` and
`rejected`.

A readiness probe that runs `SELECT 1` (see
[Health checks](32-health-checks.md)) borrows a connection like any other
query. Run it with `queryWithTimeout` so a saturated pool reports not-ready
promptly instead of waiting for a connection.

## SQLite

SQLite has one writer at a time, whatever the pool size. Gebweb apps on
SQLite should:

- Use a file database. A private `:memory:` database always uses a single
  connection, which serialises every request; it is fine for tests.
- Keep `maxOpenConns` modest. More connections let more reads run in
  parallel but cannot speed up writes.
- Use `txLock: "immediate"` when transactions read and then write, and set
  `busyTimeoutMs` together with operation deadlines.

## Testing

Tests usually pass an in-memory SQLite connection to the same `build`
function, so the container wiring is exercised exactly as in production:

```gb
let application = app.build(db.Connection("sqlite", ":memory:"));
let client = gebweb.TestClient(application);
```

The single-connection pool means a test that holds a cursor or transaction
open while issuing another query through the pool waits forever. Close
cursors and finish transactions before the next query, which is the same
discipline production code needs.

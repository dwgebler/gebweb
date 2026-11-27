# Database connections and concurrency

<!-- doctest:strict -->

A Gebweb app shares one database connection pool across all of its requests,
jobs and scheduled tasks. Your entry point opens the pool, `build` hands it to
the repositories, and a shutdown hook closes it. This chapter covers that
wiring, how many requests and connections to allow, what happens when traffic
exceeds them, and SQLite's limits.

The pool is Geblang's `db.Connection`; its options and statistics are
described in the Geblang manual's database reference, under "Pool sizing and
concurrency".

## One pool per process

`db.Connection` is a pool. Each query borrows a connection and returns it when
the query finishes, so concurrent requests using the same `db.Connection` run
in parallel, up to the pool's `maxOpenConns`. Open it once, in `main`:

<!-- doctest:fragment context=script -->
```gb
func build(db.Connection conn): gebweb.GebwebApp {
    return gebweb.app();
}

export func main(list<string> args): int {
    cli.checkArgs(args, {"name": "orders"});
    let conn = db.Connection({
        "driver": "postgres",
        "dsn": sys.getenv("DATABASE_URL") ?? "postgres://localhost/orders",
        "maxOpenConns": 16,
        "connMaxLifetimeMs": 1800000,
    });
    let app = build(conn);
    gebweb.onShutdown(app, "database", func(): void {
        conn.close();
    });
    gebweb.cli(app, {"name": "orders"});
    return 0;
}
```

| Option | Effect |
|--------|--------|
| `maxOpenConns` | The most connections open at once; extra queries wait for one. Unlimited by default. |
| `maxIdleConns` | Idle connections kept for reuse; defaults to `maxOpenConns` when only that is set. |
| `connMaxLifetimeMs` | Replace connections after this age, for databases or proxies that drop old ones. |
| `connMaxIdleTimeMs` | Close connections idle for this long. |

Never open a connection per request, repository or controller: each pays the
connect and authentication cost, and the number of database connections then
grows with traffic instead of staying within the pool.

## Giving the pool to the app

Repositories take the pool in their constructor, and `build` registers one
instance of each. Everything that needs a repository asks for it as a
constructor parameter:

<!-- doctest:run -->
```gb
import db;
import gebweb;
import io;

class OrderRepository {
    db.Connection conn;

    func OrderRepository(db.Connection conn) {
        this.conn = conn;
    }

    func count(): int {
        let row = this.conn.queryOne("SELECT count(*) AS n FROM orders");
        return (row as dict<string, any>)["n"] as int;
    }
}

@Controller("/orders")
class OrderController {
    OrderRepository orders;

    func OrderController(OrderRepository orders) {
        this.orders = orders;
    }

    @Get("/count")
    func count(): dict<string, any> {
        return {"count": this.orders.count()};
    }
}

func build(db.Connection conn): gebweb.GebwebApp {
    let app = gebweb.app();
    gebweb.registerInstance(app, OrderRepository, OrderRepository(conn));
    return app;
}

let conn = db.Connection({"driver": "sqlite", "dsn": ":memory:", "maxOpenConns": 1});
defer conn.close();
conn.exec("CREATE TABLE orders (id INTEGER PRIMARY KEY)");
conn.exec("INSERT INTO orders (id) VALUES (1), (2)");
io.println(gebweb.TestClient(build(conn)).get("/orders/count").text());
io.println("pool size ${conn.stats()["maxOpenConnections"]}");
```

Output:

```text
{"count":2}
pool size 1
```

The repository is shared by every request, which is safe because its only
state is the pool. Never keep per-request state, such as "the current user" or
"the open transaction", in a repository's fields; see
[Give database resources an owner](39-best-practices.md#give-database-resources-an-owner).
`db.Connection` is a native type, so register the class that holds it rather
than the connection itself.

## Transactions

Begin a transaction in the method that owns the operation, pass the
`db.Transaction` to every step that belongs to it, and commit or roll back
before returning. Queries made through the pool while a transaction is open
use a different connection and are not part of it.

<!-- doctest:fragment context=script -->
```gb
class AccountRepository {
    db.Connection conn;

    func AccountRepository(db.Connection conn) {
        this.conn = conn;
    }

    func transfer(int from, int to, int amount): void {
        let tx = this.conn.begin(2000);
        try {
            tx.exec("UPDATE accounts SET balance = balance - ? WHERE id = ?", amount, from);
            tx.exec("UPDATE accounts SET balance = balance + ? WHERE id = ?", amount, to);
            tx.commit();
        } catch (Error e) {
            tx.rollback();
            throw e;
        }
    }
}
```

A transaction holds its connection for its whole life, so keep network calls
(HTTP clients, mail, message brokers) outside it. `begin(2000)` gives it a
two-second budget, which includes waiting for a free connection.

## Requests, connections and queues

Each request runs on its own goroutine, with no limit unless you set one.
Three independent limits sit between a client and the database:

```text
client
  |
  v
server limit     maxConcurrent / queueSize / onOverload   (gebweb.cli "server" options)
  |              at the cap: reject with 503, queue, or drop
  v
admission gate   gebweb.useAdmissionControl                (app setting)
  |              at the cap: 503 with Retry-After, no queue
  v
handler
  |
  v
database pool    maxOpenConns                              (db.Connection option)
                 at the cap: wait for a free connection
```

### Server limit

The server's `maxConcurrent` caps the handlers running at once. `queueSize`
holds further requests until a slot opens. `onOverload` says what happens when
the queue is full too: `"reject"` (a 503), `"wait"` (queue without limit) or
`"drop"` (close the connection). Set them in the `server` options of
`gebweb.cli`:

<!-- doctest:fragment context=app -->
```gb
gebweb.cli(app, {
    "name": "orders",
    "port": 8080,
    "server": {"maxConcurrent": 128, "queueSize": 256, "onOverload": "reject"},
});
```

This is the only limit that queues, and the only one applied before a request
reaches the app. A WebSocket connection holds a slot for its whole life.

### Admission gate

`gebweb.useAdmissionControl(app, {"maxConcurrent": N})` counts requests inside
the app. Above `N`, a request gets an immediate 503 with `Retry-After: 1`
before any middleware or handler runs; an `onOverload` callable returning a
response replaces that answer. It never queues. Use it to shed load with your
own response, or when a platform you do not control sets the server limit.
When you set both, keep the admission limit below the server's
`maxConcurrent`, or it never triggers. See
[Middleware](09-middleware.md#admission-control).

### Waiting for the pool

A request that passes both limits and finds every connection busy waits in
the pool. An untimed query waits as long as it takes. A timed one,
`queryWithTimeout(ms, sql, args...)`, `execWithTimeout` or `begin(ms)`, counts
the wait against its budget and throws `TimeoutError`, which is a 500 unless
you catch it. Answering 503 tells clients and load balancers to retry:

<!-- doctest:fragment context=script -->
```gb
class OrderController extends gebweb.Controller {
    db.Connection conn;

    func OrderController(db.Connection conn) {
        parent();
        this.conn = conn;
    }

    @Get("/orders/{id}")
    func show(int id): dict<string, any> {
        ?dict<string, any> row = null;
        try {
            row = this.conn.queryWithTimeout(500, "SELECT id, total FROM orders WHERE id = ?", id).first();
        } catch (TimeoutError e) {
            throw errors.HttpException(503, "database busy", "Service Unavailable");
        }
        if (row == null) {
            this.notFound("no order ${id}");
        }
        return row as dict<string, any>;
    }
}
```

In a real app the query lives in a repository, which lets the timeout
propagate; the controller decides that a timeout means 503.

### Choosing the numbers

| Limit | Protects | Size it from |
|-------|----------|--------------|
| `maxOpenConns` | The database | The database's connection limit, divided among every process that opens a pool |
| Server `maxConcurrent` | Memory and file descriptors | Memory per request under load; usually several times `maxOpenConns` |
| `queueSize` | Latency | How long a client should wait, divided by the typical request time, times `maxConcurrent` |
| Query and transaction budgets | Request latency | Shorter than the client's or load balancer's timeout |

`maxConcurrent` can safely exceed `maxOpenConns`: requests also spend time
rendering, serialising and calling other services, and many routes never touch
the database. Set them equal only when nearly every request holds a connection
for most of its life.

Choose the overload behaviour by what clients should see at the cap. Rejecting
turns a spike into fast 503s that clients or a load balancer can retry
elsewhere. Queuing turns it into latency, which suits a single instance whose
clients would rather wait.

## Workers and the connection budget

`gebweb.useJobs(app, conn)` keeps the job table in the same database. An
in-process worker (`{"inProcessWorker": true}`) shares the app's pool, so its
jobs compete with requests for connections. A worker run as its own process
(`gebweb worker`) opens its own pool. Count every pool against the database's
limit:

```text
total connections = (app instances x app maxOpenConns)
                  + (worker processes x worker maxOpenConns)
                  + migrations, admin tools and other clients
                  <= database connection limit
```

A worker usually runs far fewer queries at once than a web process, so give it
a smaller pool sized from its job concurrency. See
[Background jobs](18-background-jobs.md).

## Watching the pool

`conn.stats()` reports `openConnections`, `inUse`, `idle`,
`maxOpenConnections`, `waitCount`, `waitDurationMs` and counts of connections
closed for idleness or age. Compare `waitCount` and `waitDurationMs` between
samples. Steady growth while the database has spare capacity means the pool is
too small or connections are held too long; growth while the database is
saturated means the pool is too large for it, and the fix is a smaller pool
with a lower request limit in front of it. `http.serverStats(handle)` reports
the server limit's `active`, `queued` and `rejected` counts.

A readiness probe that runs `SELECT 1` borrows a connection like any query.
Run it with `queryWithTimeout` so a saturated pool reports not ready promptly
instead of waiting; see [Health checks](32-health-checks.md).

## SQLite

SQLite allows one writer at a time, whatever the pool size:

- Use a file database in production. An in-memory database (`:memory:`) is
  private to one connection, so it serialises every request; it suits tests.
- Keep `maxOpenConns` modest. More connections let reads run in parallel but
  cannot speed up writes.
- Turn on `wal: true` so readers do not block the writer, and set
  `txLock: "immediate"` when transactions read and then write, so they wait for
  the write lock at the start rather than failing halfway. `busyTimeoutMs` sets
  how long a writer waits for the lock (five seconds by default).

## Testing

Tests pass an in-memory SQLite connection to the same `build` the entry point
uses, so the wiring is exercised exactly as in production. An in-memory pool
has a single connection, so a test that holds a cursor or transaction open
while issuing another query through the pool waits forever. Close cursors and
finish transactions before the next query, which is the same discipline
production code needs.

## Reference

- `db.Connection({...})` options: `driver`, `dsn`, `maxOpenConns`,
  `maxIdleConns`, `connMaxLifetimeMs`, `connMaxIdleTimeMs`; for SQLite also
  `wal`, `busyTimeoutMs`, `txLock` and the other pragmas in the Geblang
  database reference.
- `conn.begin(timeoutMs = null)`, `queryWithTimeout(ms, sql, args...)`,
  `execWithTimeout(ms, sql, args...)`, `conn.stats()`, `conn.close()`.
- `gebweb.cli(app, {"server": {"maxConcurrent", "queueSize", "onOverload"}})`.
- `gebweb.useAdmissionControl(app, {"maxConcurrent": N, "onOverload": fn})`.
- `http.serverStats(handle)`: `active`, `queued`, `rejected`.

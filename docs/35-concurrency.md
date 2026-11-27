# Concurrency and shared state

<!-- doctest:strict -->

Gebweb handles HTTP requests concurrently. Controllers and singleton services
are reused across requests, so their mutable fields can be accessed by several
requests at once. Keep those services stateless, or protect shared state with
synchronized operations. A local variable is private to its call; an object it
references may still be shared.

## Sharing mutable state safely

This application keeps counters in a `store.Store`. Each increment is atomic,
so concurrent requests cannot overwrite one another's increments. Put it in
`src/metrics_app.gb`:

<!-- doctest:file metrics_app.gb -->
```gb
import gebweb;
import log;
import store;

export class Metrics {
    store.Store hits;

    func Metrics() {
        this.hits = store.Store();
    }

    func record(string route): int {
        return this.hits.incr(route);
    }

    func count(string route): int {
        return (this.hits.get(route) ?? 0) as int;
    }
}

class HitCount {
    int hits;

    func HitCount(int hits) {
        this.hits = hits;
    }
}

@Controller("/metrics")
class MetricsController {
    Metrics counters;

    func MetricsController(Metrics counters) {
        this.counters = counters;
    }

    @Post("/hit")
    func hit(): HitCount {
        return HitCount(this.counters.record("/metrics/hit"));
    }
}

export func build(Metrics counters): gebweb.GebwebApp {
    let app = gebweb.app();
    gebweb.useLogger(app, log.stderr());
    gebweb.registerInstance(app, Metrics, counters);
    return app;
}
```

Register the shared instance before the first request, when controllers are
constructed. This runnable check exercises the route and then calls the same
service from several tasks. Only the parent task modifies the task-handle list,
and it observes every task's result before reading the final count:

<!-- doctest:run -->
```gb
import async;
import gebweb;
import io;
import metrics_app;

let counters = metrics_app.Metrics();
let app = metrics_app.build(counters);
let response = gebweb.TestClient(app).request("POST", "/metrics/hit", null, {});
io.println(response.status);
io.println(counters.count("/metrics/hit"));

list<Task<int>> tasks = [];
for (i in range(1, 8)) {
    tasks.push(async.run(func(): int {
        for (j in range(1, 100)) {
            counters.record("background");
        }
        return 100;
    }));
}
int completed = 0;
for (task in tasks) {
    completed = completed + async.await(task);
}
io.println(completed);
io.println(counters.count("background"));
```

```text
200
1
800
800
```

## What is shared, and what is not

| Lifetime | Examples | Consequence |
|----------|----------|-------------|
| App lifetime | Controllers, default DI services, registered instances | All requests can reach the same object. |
| Request scope | Services registered with `registerPerRequest` and resolved during dispatch | One instance per active request scope. |
| Call lifetime | Handler locals, request wrappers, newly constructed DTOs | Private unless passed to another task or linked to shared state. |
| External resource | Database pool, configured cache or session backend | Use the backend's own concurrency and transaction contract. |

A per-request service injected into a controller at construction can remain
captured for the controller's lifetime. Resolve request-scoped dependencies
within the active request instead; see [DI lifetimes](06a-dependency-injection.md#lifetimes).
A shallow collection copy also keeps links to nested objects. It does not
isolate everything a handler can reach.

## The one rule

Do not mutate an unprotected `dict`, `list`, `set` or object shared by requests
or tasks. Concurrent reads and writes can race, lose updates or crash the
process. Making the service a singleton does not synchronize its methods.
Read-only configuration is safe to share once startup has finished and no
aliases can mutate it.

`store.Store` is suitable for shared data snapshots and counters. Keep
infrastructure handles such as database connections in their owning services;
do not treat storing a handle as a way to synchronize all operations on the
resource it represents. For custom shared object invariants, use a lock around
all relevant reads and writes, with release in a `finally` block.

## Atomic operations and snapshots

Individual Store operations are synchronized. `get` followed by `set` is still
two operations: another request may change the value between them. Use `incr`
for integer counters, `compareAndSet` for conditional replacement, or `update`
for a read-modify-write on one key.

Store values are isolated snapshots for ordinary mutable data. Changing a
retrieved dictionary does not change the stored value until you write it back.
This example puts an entire invariant under one key:

<!-- doctest:run -->
```gb
import io;
import store;

let inventory = store.Store();
inventory.set("stock", {"remaining": 2, "reserved": 0});
let updated = inventory.update("stock", func(any old): any {
    let stock = old as dict<string, int>;
    if (stock["remaining"] > 0) {
        stock["remaining"] = stock["remaining"] - 1;
        stock["reserved"] = stock["reserved"] + 1;
    }
    return stock;
}) as dict<string, int>;
io.println(updated["remaining"]);
io.println(updated["reserved"]);

let snapshot = inventory.get("stock") as dict<string, int>;
snapshot["remaining"] = 99;
io.println((inventory.get("stock") as dict<string, int>)["remaining"]);
```

```text
1
1
1
```

`update` retries its callback when a concurrent change prevents replacement.
Keep the callback free of external effects: no email, payment, enqueue or
response writes inside it. Do not depend on its invocation count. The callback
may run more than once even though only one resulting value is stored.

Several keys do not form a transaction. Neither do `keys()` followed by
individual `get()` calls: that produces a best-effort view, not a consistent
snapshot of the whole store. Store related state under one key when practical,
or use a lock or database transaction for a wider invariant.

## Beyond one process

An in-memory Store belongs to one process. Multiple app instances have separate
counters, and restarting loses the data. Use a database or an appropriate
shared backend for durable or cross-instance state. A cache or session store's
safe individual operations do not automatically make an application-level
read-modify-write atomic. Check the selected backend's contract.

Bound background work and observe its result or failure. Request completion
does not make unobserved tasks finish successfully. Use the framework's
[background jobs](18-background-jobs.md) for durable work, and coordinate task
ownership and shutdown for in-process work. Size request concurrency and the
database pool together; see [database connections](06b-database-connections.md).

## How the framework itself does it

The rate limiter uses a Store update for each bucket's refill-and-consume
operation. `MemoryStorage` uses a Store for its entries. The WebSocket broadcast
Hub locks membership changes and snapshots subscribers under the lock before
sending. These are specific synchronization mechanisms, rather than a promise
that arbitrary application state or callbacks are safe to mutate concurrently.

## Reference

| Operation or pattern | Use |
|----------------------|-----|
| `store.Store().incr(key, delta = 1)` | Atomic integer addition; absent keys start at zero. |
| `getOrSet(key, value)` | Atomically use the existing value or install the supplied value; constructing that argument happens before the call. |
| `compareAndSet(key, expected, next)` | Replace a matching value atomically; null also matches an absent key. |
| `update(key, callback)` | Retry a pure transformation of one key's value. |
| `registerInstance(app, T, instance)` | Share an existing service; does not add synchronization. |
| `registerPerRequest(app, T, factory)` | Resolve a service within the active request scope. |

The Geblang manual covers the language-level Store, locks, atomics, channels
and task primitives. The framework-specific lifetime rules are in
[dependency injection](06a-dependency-injection.md); backend sharing is covered
by [database connections](06b-database-connections.md).

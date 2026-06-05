# Concurrency And Shared State

Gebweb serves each request on its own lightweight goroutine, so requests
run in parallel. That parallelism is what keeps an app responsive under
load, but it means you have to be deliberate about any state that more
than one request can touch at the same time.

The short version: services and singletons are shared across requests on
purpose, and that is fine as long as the shared state is either read-only
or thread-safe. Mutable in-process state shared across requests goes in a
`store.Store` (or a real backing store), never in a plain `dict` or `list`
held by a service.

## What is shared, and what is not

Shared across every request (created once, at startup):

- Controllers and the services injected into them.
- DI singletons registered with `register` / `registerInstance`.
- Repositories, configuration, compiled views, and the parameter store.
- Infrastructure handles: the database pool, the cache store, the logger.
  These are thread-safe by construction, so sharing one connection pool
  across all requests is exactly right.

Private to a single request (never shared, so never a concern):

- The `Request` object and anything you read from it.
- Local variables inside a handler.
- Services registered with `registerPerRequest`, which are constructed
  fresh for each request.

## The one rule

**Do not mutate a plain `dict`, `list`, `set`, or object that is shared
across requests.** Two requests writing the same container at the same
instant can crash the process. This is the same contract the language
itself has (see the Geblang concurrency chapter); a web app just hits it
sooner because requests are concurrent by default.

A stateless service is safe to share. A service that only reads shared
configuration is safe to share. The moment a shared service starts
mutating a container it holds, that container needs protection.

## Sharing mutable state safely

Reach for a `store.Store`: a thread-safe key-value store with atomic
`incr`, `getOrSet`, `compareAndSet`, and `update(key, fn)`. Hold it in a
singleton service and every request can use it safely.

```gb
import gebweb;
import store;

class Metrics {
    store.Store _hits;

    func Metrics() {
        this._hits = store.Store();
    }

    /* Safe to call from any number of concurrent requests. */
    func record(string route): int {
        return this._hits.incr(route);
    }

    func snapshot(): dict<string, any> {
        dict<string, any> out = {};
        for (k in this._hits.keys()) {
            out[k as string] = this._hits.get(k);
        }
        return out;
    }
}
```

Register `Metrics` as a singleton and inject it into controllers; every
request shares the one instance and the counters stay correct under load.

For state that must outlive the process or be shared across instances,
use a real backing store instead: the database, the cache store
(`useCacheStore`), or a session store. Those are the right home for
cross-request state in production.

## How the framework itself does it

The same discipline runs through Gebweb's own internals, so the built-in
features are safe under concurrent load:

- The rate-limit middleware keeps its token buckets in a `Store` and
  refills-and-consumes each bucket with a single atomic `update`, so two
  requests can never both spend the last token.
- `MemoryStorage` is backed by a `Store`.
- The WebSocket broadcast `Hub` guards its subscriber list with a mutex
  and snapshots membership under the lock before sending.

## Reference

- Use a singleton service holding a `store.Store` for shared mutable
  in-process state; use `registerPerRequest` for per-request state.
- See the Geblang manual's concurrency material for the language-level
  primitives (`store.Store`, `async.sync`, `async.atomic`, channels) and
  the full rationale.

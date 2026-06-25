# Caching

Gebweb's response cache is opt-in per route. Register a store on the
app, decorate handlers with `@Cache(ttl)`, and the framework checks
the cache before binding / dispatching the handler. On routes that
carry `@Auth` (or role / permission requirements) authentication runs
BEFORE the cache lookup - a cache hit never bypasses auth - and the
cache key automatically varies on `Authorization` and `Cookie`, so
one user's cached response is never served to another. Anonymous
routes keep the cheap cache-first path.

## Registering a store

```gb
import web.cache as cache;

let store = cache.fileCacheStore("/tmp/app-cache", 3600);
gebweb.useCacheStore(app, store);
```

`store` is any value with `get(key): any` and
`set(key, value): bool` methods. The stdlib provides three drop-in
stores:

| Factory                                          | Backend            |
|--------------------------------------------------|--------------------|
| `gebweb.redisCacheStore(opts)`                   | Redis (shared)     |
| `web.cache.fileCacheStore(directory, ttl)`       | JSON files on disk |
| `web.cache.databaseCacheStore(conn, table, ttl)` | SQL table          |

The store's TTL governs eventual eviction; per-route `@Cache(ttl)`
wraps a tighter TTL envelope inside the stored value (see below).

## Redis cache store

`gebweb.redisCacheStore(opts)` returns a cache store backed by Redis.
Because the store lives outside the process, multiple app instances
sharing the same address share the same cache - a cache hit written by
any instance is served by all others.

Requires geblang 1.28.0+ (uses `redis.Client.setex`/`eval`).

```gb
import gebweb;

let app = gebweb.app(controllers);
gebweb.useCacheStore(app,
    gebweb.redisCacheStore({
        "address":  "localhost:6379",
        "ttl":      300,
        "poolSize": 8,
    })
);
gebweb.serve(app);
```

**Options:**

| Option     | Default | Meaning                                      |
|------------|---------|----------------------------------------------|
| `address`  | required| `"host:port"` of the Redis server.           |
| `ttl`      | `300`   | Store-level TTL in seconds.                  |
| `poolSize` | `8`     | Max concurrent connections to Redis.         |
| `password` | none    | Password for Redis `AUTH`.                   |
| `db`       | `0`     | Redis logical database index (`SELECT`).     |
| `pool`     | none    | Pre-built `gebweb.redisPool(...)` (see below).|
| `logger`   | none    | `func(string): void` for warn messages.      |

**Fail-open behavior:** any Redis error (connection refused, timeout,
command error) is caught and warn-logged. `get` returns null (cache
miss); `set` and `delete` are no-ops. A Redis outage degrades to
serving every request from the origin - the app never surfaces the
Redis error to clients.

## Sharing a Redis connection pool

Both `redisCacheStore` and `redisRateLimit` (see middleware chapter)
open their own connection pool by default. To share one pool between
them - avoiding double the connections to the same server - build a
pool explicitly and pass it as the `pool` option:

```gb
let pool = gebweb.redisPool({
    "address":  "localhost:6379",
    "poolSize": 8,
});

gebweb.useCacheStore(app,
    gebweb.redisCacheStore({"pool": pool, "ttl": 300})
);
gebweb.before(app,
    gebweb.redisRateLimit({"pool": pool, "perSecond": 50, "burst": 100})
);
```

`gebweb.redisPool(opts)` accepts `address`, `poolSize`, `password`,
and `db`. The pool holds up to `poolSize` live connections; a
connection that errors on use is closed and replaced automatically.

## Opting routes in

```gb
class FeedController {
    @Get("/feed")
    @Cache(ttl: 60)
    func feed(): dict<string, any> {
        return {"items": expensiveLookup()};
    }
}
```

The cache key defaults to `METHOD path` (e.g. `GET /feed`). Cache
hits skip every later stage: handler dispatch, validation, even auth.

## Vary on headers

For routes whose response depends on a header (auth identity, tenant,
preferred language):

```gb
@Get("/me")
@Auth
@Cache(ttl: 30)
func me(CurrentUser user): dict<string, any> {
    return {"id": user.id, "name": user.name};
}
```

The cache key becomes `METHOD path key=value;key=value` with vary
headers sorted alphabetically (case-insensitive header lookup).
`@Auth` routes vary on `Authorization` and `Cookie` automatically;
listing extra headers in `vary` adds them on top. Different
credential values get different cache entries.

## Class-level `@Cache`

A class-level decorator applies to every method:

```gb
@Cache(ttl: 10)
class PublicController {
    @Get("/public/banner") func banner(): dict<string, any> { /* ... */ }
    @Get("/public/feed")   func feed():   dict<string, any> { /* ... */ }
}
```

A method-level `@Cache` overrides the class-level one for that route.

## What's NOT cached

- Non-2xx responses are never written to the cache. A 4xx / 5xx is
  surfaced normally; if the next call returns 200, that response
  takes the cache slot.
- Streaming responses (`@WebSocket`, `@Sse`, `gebweb.stream`) are
  passed through. There's no useful "cached stream" semantics.
- `@Cache(ttl: 0)` disables the per-route envelope; entries live for
  the store's TTL only.

## TTL envelope mechanics

A per-route `@Cache(ttl: 60)` writes a stored value shaped as:

```geblang
{"response": <response>, "expiresAt": <now + ttl>}
```

The store wraps THAT in its own envelope and tracks the broader
store-TTL eviction. On read, the framework strips the store envelope,
then checks the route envelope's `expiresAt` and treats expired
entries as misses. This means a store-TTL of 1 hour can host a route
TTL of 30 seconds without storage drift.

## Invalidating after a write

When you write to something that has a cached read, you usually
want to drop the cached read so the next request rebuilds it.
For example: `GET /widgets` is `@Cache`-d, and `POST /widgets`
should drop that cached list.

The cache key is built from the request. A fake `GET` request
shape gives you the same key the framework would have written:

```gb
import gebweb.cache as cache;

@Post("/widgets")
func create(WidgetDto body): dict<string, any> {
    repo.insert(body);
    let key = cache.cacheKey({"method": "GET", "path": "/widgets",
                              "headers": {}, "query": {}}, []);
    this.store.delete(key);
    return {"created": true};
}
```

If the cached route uses `@Cache(vary: ["X-User"])`, pass the
same list as the second argument to `cacheKey` so the key
matches.

## Reference

- `gebweb.useCacheStore(app, store): GebwebApp` - register a cache
  store. `store` is any value with `get(key)` and `set(key, value)`.
- `@Cache(ttl: N)` - opt a route in. `ttl` is the per-route TTL in
  seconds; 0 means defer to the store TTL.
- `@Cache(ttl: N, vary: ["X-User", ...])` - extend the cache key with
  header values. Vary keys are sorted; lookup is case-insensitive.
- Facade stores: `gebweb.redisCacheStore(opts)` (Redis, shared across
  instances; geblang 1.28.0+), `web.cache.fileCacheStore(directory, ttl)`
  (JSON files), `web.cache.databaseCacheStore(conn, table, ttl)` (SQL).

Helper primitives in `gebweb.cache` for custom store integrations:
`findCacheDecorator`, `ttlFromDecorator`, `varyFromDecorator`,
`cacheKey`, `envelope`, `openEnvelope`.

## Cache tags and invalidation

Tag cached responses and drop them by tag after writes:

```gb
@Get("/users/{id}")
@Cache(ttl: 300, tags: ["user:{id}", "users"])
func show(@PathParam("id") string id): dict<string, any> { ... }

/* after updating user 42: */
gebweb.cacheInvalidate(app, "user:42");   /* one user's entries */
gebweb.cacheInvalidate(app, "users");     /* every tagged entry */
```

`{name}` placeholders resolve from the request's path parameters when
the response is cached. The tag index lives in the same cache store,
so invalidation works across processes sharing a Redis store.

---

## Idempotency

The `@Idempotent` decorator makes state-changing endpoints safe to
retry. A duplicate request carrying the same client-supplied key
replays the original response instead of re-running the handler. A
concurrent duplicate is rejected while the first is still in flight.
This is the payment-API safety property (Stripe-style
`Idempotency-Key`).

### Registering a store

```gb
import gebweb;

let app = gebweb.app(controllers);

/* Single-process (in-memory, no external dependency): */
gebweb.useIdempotencyStore(app, gebweb.memoryIdempotencyStore());

/* Multi-instance (Redis, shared across processes): */
gebweb.useIdempotencyStore(app,
    gebweb.redisCacheStore({
        "address":  "localhost:6379",
        "ttl":      86400,
        "poolSize": 4,
    })
);
```

| Factory                           | Backend                | Atomicity guarantee        |
|-----------------------------------|------------------------|----------------------------|
| `gebweb.memoryIdempotencyStore()` | In-process store.Store | Strong (in-process atomic) |
| `gebweb.redisCacheStore(opts)`    | Redis (shared)         | Strong (Lua SET NX atomic) |

The file cache store (`web.cache.fileCacheStore`) can be passed but
its `putIfAbsent` is best-effort (get-then-set, no file lock). Use
the Redis or in-memory store for real concurrency.

### Decorating a handler

```gb
class OrderController {
    @Post("/orders")
    @Idempotent
    func create(OrderDto body): dict<string, any> {
        /* Runs at most once per Idempotency-Key value. */
        return {"id": repo.insert(body)};
    }

    @Post("/payments/{id}/capture")
    @Idempotent(required: true, ttl: 3600)
    func capture(@PathParam("id") string id): dict<string, any> {
        return {"captured": payment.capture(id)};
    }
}
```

**Options:**

| Option    | Default                         | Meaning                                                      |
|-----------|---------------------------------|--------------------------------------------------------------|
| `required`| `false`                         | `true` -> missing key returns 400; `false` -> runs normally. |
| `ttl`     | `86400` (24 h)                  | How long the record is retained, in seconds.                 |
| `header`  | `"Idempotency-Key"`             | Request header carrying the client key.                      |
| `methods` | `["POST", "PUT", "PATCH"]`      | Methods the decorator applies to; others run normally.       |

### The `Idempotency-Key` header

The client sends a unique opaque value (UUID, random hex, etc.) with
each request that should be idempotent:

```
POST /orders
Idempotency-Key: 550e8400-e29b-41d4-a716-446655440000
Content-Type: application/json

{"item": "widget", "qty": 1}
```

The key is scoped per route and method internally, so the same key
value on different endpoints does not collide.

### Request flow

1. If the request method is not in `methods` (e.g. GET), the handler
   runs normally - no idempotency check.
2. If no key header is present and `required: false`, the handler
   runs normally. If `required: true`, 400 is returned.
3. The framework builds a record key from the route, method, and
   client key, and a fingerprint from the method, path, and request
   body.
4. **Atomic claim** via `putIfAbsent`:
   - Claim succeeds (key absent): the handler runs.
     - 2xx response: the response is stored and returned to the
       caller (NOT marked as a replay - it is the original).
     - Non-2xx response or thrown exception: the marker is released
       so the next same-key request re-runs.
   - Claim fails (key present):
     - `in-progress`: 409 ("a request with this idempotency key is
       already in progress").
     - `completed`, same fingerprint: the stored response is returned
       with `Idempotent-Replayed: true`.
     - `completed`, different fingerprint: 422 ("idempotency key
       reused for a different request").

### Responses

| Condition                          | Status | `Idempotent-Replayed` header |
|------------------------------------|--------|------------------------------|
| First call (handler ran)           | as-is  | absent                       |
| Replay (same key + same body)      | as-is  | `true`                       |
| In-progress duplicate              | 409    | absent                       |
| Key reused for different body      | 422    | absent                       |
| `required: true` + no key          | 400    | absent                       |

Non-replay error responses use `application/problem+json`.

### Release on failure

If the handler throws or returns a non-2xx status, the idempotency
marker is deleted before the error propagates. This means a transient
failure (network error, timeout, validation failure) does not lock
the client out - a retry with the same key re-runs the handler.

Only successful (2xx) responses are stored for replay.

### Example: verifying idempotency in tests

```gb
import test;
import gebweb;

class OrderTest extends test.Test {
    gebweb.TestClient client;
    int runs;

    func setUp(): void {
        this.runs = 0;
        let app = gebweb.app([OrderController()]);
        gebweb.useIdempotencyStore(app, gebweb.memoryIdempotencyStore());
        this.client = gebweb.TestClient(app);
    }

    @test
    func replayReturnsSameResponse(): void {
        let hdrs = {"Idempotency-Key": "test-key-1"};
        let a = this.client.request("POST", "/orders", {"item": "widget"}, hdrs);
        a.assertStatus(201);

        let b = this.client.request("POST", "/orders", {"item": "widget"}, hdrs);
        b.assertStatus(201);
        this.assertEquals("true", b.headers["Idempotent-Replayed"]);
        /* Handler ran only once. */
    }
}
```

### Reference

- `gebweb.useIdempotencyStore(app, store): GebwebApp` - register a
  store. Store must implement `get(key)`, `set(key, value, ttl)`,
  `delete(key)`, and `putIfAbsent(key, value, ttl): bool`.
- `@Idempotent` - opt a handler in with defaults (POST/PUT/PATCH,
  TTL 24 h, `Idempotency-Key` header, key optional).
- `@Idempotent(required, ttl, header, methods)` - override any option.
- `gebweb.memoryIdempotencyStore()` - in-process store; strong
  atomicity via `store.Store`; lost on process restart.
- `gebweb.redisCacheStore(opts)` - Redis-backed store; `putIfAbsent`
  uses an atomic Lua `SET NX PX` script; survives restarts and works
  across multiple app instances.

# Caching and idempotency

<!-- doctest:strict -->

Two features keep repeated requests cheap and safe:

- **Response caching** (`@Cache`) answers a repeated read from a stored
  response instead of running the handler again.
- **Idempotency** (`@Idempotent`) makes a retried write, such as a payment a
  client sent twice after a timeout, replay the first response instead of
  running twice.

## Caching responses

Register a cache store, then mark the routes to cache:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import store;
import web.cache as cache;

let calls = store.Store();

@Controller("/catalog")
class CatalogController {
    @Get("")
    @Cache(ttl: 60)
    func list(?string category): dict<string, any> {
        return {"category": category ?? "all", "built": calls.incr("list")};
    }
}

let app = gebweb.app();
gebweb.useCacheStore(app, cache.memoryCacheStore(3600));
let client = gebweb.TestClient(app);
io.println(client.get("/catalog").text());
io.println(client.get("/catalog").text());
io.println(client.get("/catalog?category=tools").text());
```

Output:

```text
{"built":1,"category":"all"}
{"built":1,"category":"all"}
{"built":2,"category":"tools"}
```

The second request was answered from the cache without running the handler.
The cache key includes the method, path and query string, so a different query
is a different entry. When [i18n](30-i18n.md) negotiates a locale, its tag also
partitions entries, including when locale prefixes are stripped before routing.
A cache hit skips binding, validation and the handler; on a route that requires
a user, authentication still runs first.

### Stores

| Store | Use |
|-------|-----|
| `web.cache.memoryCacheStore(ttl)` | One process; lost on restart. |
| `web.cache.fileCacheStore(directory, ttl)` | One process; survives restarts. |
| `web.cache.databaseCacheStore(conn, table, ttl)` | Shared through your database. |
| `gebweb.redisCacheStore(opts)` | Shared by every instance through Redis. |

The store's TTL is how long it keeps any entry; `@Cache(ttl)` sets a shorter
lifetime for one route, so one store can serve routes with different TTLs.
`@Cache(ttl: 0)` uses the store's TTL. A store is any object with
`get(key)` and `set(key, value)`.

`gebweb.redisCacheStore` takes `address`, `ttl` (default 300), `poolSize`,
`password`, `db`, `prefix`, an existing `pool` from `gebweb.redisPool(opts)`
to share connections with `redisRateLimit`, and `logger`. With Redis, an entry
cached by one instance is served by all of them.

### What is cached

- Only 2xx responses. An error is answered normally and not stored.
- Not streams, Server-Sent Events or WebSockets.
- `@Cache(ttl: 60, vary: ["Accept-Language"])` adds the listed request headers
  to the key, so each language gets its own entry. List every header that
  changes the response.
- `@Cache` on a controller class applies to all its routes.

Never cache pages that contain a CSRF token, flash messages or a CSP nonce:
the cached copy would hand one visitor's token to the next.

## Authenticated routes

A cached route that requires a user is cached separately for each user, so
one user's response is never served to another:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import web.cache as cache;

class Account {
    string id;

    func Account(string id) {
        this.id = id;
    }
}

@Controller("/me")
class ProfileController {
    @Get("/feed")
    @Auth
    @Cache(ttl: 60)
    func feed(Account user): dict<string, any> {
        return {"feedFor": user.id};
    }
}

func accountForKey(string key): ?Account {
    return key.startsWith("key-") ? Account(key.substring(4)) : null;
}

let app = gebweb.app();
gebweb.useCacheStore(app, cache.memoryCacheStore(3600));
gebweb.useAuth(app, Account, [gebweb.apiKeyAuthenticator(accountForKey)]);
let client = gebweb.TestClient(app);
io.println(client.request("GET", "/me/feed", null, {"X-API-Key": "key-ada"}).text());
io.println(client.request("GET", "/me/feed", null, {"X-API-Key": "key-lin"}).text());
io.println(client.get("/me/feed").status);
```

Output:

```text
{"feedFor":"ada"}
{"feedFor":"lin"}
401
```

Entries are partitioned by the user's `id` field. A user without one is never
cached. When the id alone does not describe what a user can see, for example
when the same user belongs to several tenants, set the scope:

<!-- doctest:fragment context=script -->
```gb
class Member {
    string id;
    string tenantId;
}

let app = gebweb.app();
gebweb.useUserScope(app, func(any user): string {
    let m = user as Member;
    return "${m.tenantId}|${m.id}";
});
```

Returning an empty string leaves that user uncached. Two users with the same
scope share entries, so the scope must change wherever the response does.

## Invalidating after a write

Tag cached responses, then invalidate the tag when the data changes:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import store;
import web.cache as cache;

let prices = store.Store();

@Controller("/products")
class ProductController extends gebweb.Controller {
    gebweb.GebwebApp app;

    func ProductController(gebweb.GebwebApp app) {
        parent();
        this.app = app;
    }

    @Get("/{id}")
    @Cache(ttl: 300, tags: ["product:{id}", "products"])
    func show(string id): dict<string, any> {
        return {"id": id, "price": prices.get(id) ?? 100};
    }

    @Put("/{id}/price/{price}")
    func reprice(string id, int price): gebweb.Response {
        prices.set(id, price);
        gebweb.cacheInvalidate(this.app, "product:${id}");
        return this.noContent();
    }
}

let app = gebweb.app();
gebweb.useCacheStore(app, cache.memoryCacheStore(3600));
let client = gebweb.TestClient(app);
io.println(client.get("/products/p1").text());
prices.set("p1", 80);
io.println(client.get("/products/p1").text());
client.request("PUT", "/products/p1/price/80", null, {});
io.println(client.get("/products/p1").text());
```

Output:

```text
{"id":"p1","price":100}
{"id":"p1","price":100}
{"id":"p1","price":80}
```

The second read was still the cached 100, because the price changed behind
the cache's back. The write route invalidated the product's tag, so the next
read was rebuilt. `{name}` in a tag is filled from the path parameters, so
`product:{id}` tags each product separately and `products` tags them all.
Always change data through a route or service that invalidates.

With Redis, invalidation applies to every instance sharing that Redis. With a
memory or file store it only reaches the process that ran it, so other
instances keep stale entries until their TTL; use Redis when you run more
than one instance.

## Idempotent writes

A client that times out does not know whether its request was processed. If
it retries a payment, the payment must not run twice. `@Idempotent` lets the
client send an `Idempotency-Key` header; a retry with the same key gets the
first response replayed:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import store;

let charges = store.Store();

class Charge {
    int amount;
}

@Controller("/payments")
class PaymentController extends gebweb.Controller {
    @Post("")
    @Idempotent(required: true)
    func charge(@Body Charge input): gebweb.Response {
        int n = charges.incr("count");
        return this.created({"charge": "ch_${n}", "amount": input.amount}, "/payments/ch_${n}");
    }
}

let app = gebweb.app();
gebweb.useIdempotencyStore(app, gebweb.memoryIdempotencyStore());
let client = gebweb.TestClient(app);
let key = {"Idempotency-Key": "order-1001"};

let first = client.request("POST", "/payments", {"amount": 500}, key);
let retry = client.request("POST", "/payments", {"amount": 500}, key);
let changed = client.request("POST", "/payments", {"amount": 900}, key);
let missing = client.post("/payments", {"amount": 500});
io.println("${first.status} ${first.text()}");
io.println("${retry.status} ${retry.text()} replayed=${retry.headers["Idempotent-Replayed"]}");
io.println("${changed.status} ${missing.status} charges=${charges.get("count")}");
```

Output:

```text
201 {"amount":500,"charge":"ch_1"}
201 {"amount":500,"charge":"ch_1"} replayed=true
422 400 charges=1
```

| Situation | Response |
|-----------|----------|
| First request with a key | The handler runs; a 2xx response is stored. |
| Same key, same request | The stored response, with `Idempotent-Replayed: true`. |
| Same key while the first is still running | 409. |
| Same key, different body, path or query | 422. |
| No key and `required: true` | 400. |
| The handler fails or answers non-2xx | Nothing is stored, so a retry runs again. |

| Option | Default | Meaning |
|--------|---------|---------|
| `required` | `false` | Answer 400 when the key header is missing. |
| `ttl` | `86400` | Seconds a completed response is kept. |
| `header` | `"Idempotency-Key"` | The header carrying the key. |
| `methods` | `["POST", "PUT", "PATCH"]` | Methods it applies to; others run normally. |

Keys are scoped to the route and method. On a route that requires a user they
are also scoped to that user, so another user's identical key is a separate
request; on public routes all clients share keys, so clients must use random,
unguessable values such as UUIDs. Use `gebweb.memoryIdempotencyStore()` for
one process and `gebweb.redisCacheStore(opts)` for several: both claim a key
atomically, so two concurrent duplicates never both run. A file cache store
cannot guarantee that.

Idempotency guarantees the handler runs once per key, not that its effects
are applied once if it fails halfway; keep writes inside a transaction as in
[Repositories](06-repositories.md).

## Reference

| API | Purpose |
|-----|---------|
| `gebweb.useCacheStore(app, store)` | Register the response cache store. |
| `@Cache(ttl: N, vary: [...], tags: [...])` | Cache a route or a controller's routes. |
| `gebweb.cacheInvalidate(app, tag)` | Drop every entry with this tag. |
| `gebweb.useUserScope(app, fn)` | Partition cached entries and idempotency keys per user. |
| `gebweb.redisCacheStore(opts)` / `gebweb.redisPool(opts)` | Redis-backed store and shared connection pool. |
| `gebweb.useIdempotencyStore(app, store)` | Register the idempotency store. |
| `gebweb.memoryIdempotencyStore()` | In-process idempotency store. |
| `@Idempotent(required, ttl, header, methods)` | Replay retried writes by key. |
| `import gebweb.cache` | Helpers for custom stores: `cacheKey`, `envelope`, `openEnvelope` and the decorator readers. |

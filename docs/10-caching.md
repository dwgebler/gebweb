# Caching

Gebweb's response cache is opt-in per route. Register a store on the
app, decorate handlers with `@Cache(ttl)`, and the framework checks
the cache before binding / dispatching the handler. Cache hits
short-circuit auth too - pair `@Cache` with `vary: ["Authorization"]`
for per-user gated routes.

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
| `web.cache.redisCacheStore(client, prefix, ttl)` | Redis              |
| `web.cache.fileCacheStore(directory, ttl)`       | JSON files on disk |
| `web.cache.databaseCacheStore(conn, table, ttl)` | SQL table          |

The store's TTL governs eventual eviction; per-route `@Cache(ttl)`
wraps a tighter TTL envelope inside the stored value (see below).

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
@Cache(ttl: 30, vary: ["Authorization"])
func me(CurrentUser user): dict<string, any> {
    return {"id": user.id, "name": user.name};
}
```

The cache key becomes `METHOD path key=value;key=value` with vary
headers sorted alphabetically (case-insensitive header lookup).
Different `Authorization` values get different cache entries.

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
- Stdlib stores: `web.cache.{redisCacheStore, fileCacheStore,
  databaseCacheStore}`.

Helper primitives in `gebweb.cache` for custom store integrations:
`findCacheDecorator`, `ttlFromDecorator`, `varyFromDecorator`,
`cacheKey`, `envelope`, `openEnvelope`.

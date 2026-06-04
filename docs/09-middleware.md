# Middleware

Middleware sits before or after the route adapter in the request
pipeline. Use it for cross-cutting concerns - CORS, security headers,
request IDs, logging, compression, rate limiting - without coupling
the concerns to any one handler.

## Registration

```gb
import gebweb.middleware as mw;

gebweb.use(app, mw.securityHeaders({}));
gebweb.use(app, mw.cors({"origin": "https://example.com"}));
gebweb.use(app, mw.requestId());
gebweb.use(app, mw.requestLog({}));
```

`gebweb.use(app, callable)` registers a response-phase middleware
(`func(gebweb.Request, Response): Response`). `gebweb.after` is an alias.
`gebweb.before(app, callable)` registers a request-phase middleware
(`func(gebweb.Request): ?Response` - return `null` to continue, or a
`Response` to short-circuit). Both phases receive the rich `Request`;
response-phase middleware build the new response with the immutable
`resp.withStatus`/`withHeader`/`withBody` builders. Type the parameter
`any` if you don't need the `Request` accessors.

For convenience the facade also re-exports the built-in factories:
`gebweb.cors(opts)`, `gebweb.securityHeaders(opts)`,
`gebweb.requestId()`, `gebweb.requestLog(opts)`,
`gebweb.compress(opts)`, `gebweb.rateLimit(opts)`.

## Built-in middleware

### CORS

```gb
gebweb.use(app, gebweb.cors({
    "origin": "https://app.example.com",
    "credentials": true,
    "methods": ["GET", "POST", "PUT", "DELETE"],
    "headers": ["Content-Type", "Authorization"],
    "maxAge": 86400,
}));
```

Defaults are permissive (`origin: "*"`); tighten as needed. A
preflight `OPTIONS` request is handled automatically.

### Security headers

```gb
gebweb.use(app, gebweb.securityHeaders({
    "extra": {"Strict-Transport-Security": "max-age=31536000"},
}));
```

Sets `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, and
`Referrer-Policy: strict-origin-when-cross-origin` by default. The
`extra` option merges on top.

### Request ID

```gb
gebweb.use(app, gebweb.requestId());
```

Adds an `X-Request-ID` header (an opaque random ID) to every response
that doesn't already have one.

### Request log

```gb
gebweb.use(app, gebweb.requestLog({}));
```

Emits a JSON log entry per request via `log.info`. Pair with the
stdlib `log` module's sink configuration to route entries to a file,
collector, or stderr.

### Response compression

```gb
gebweb.use(app, gebweb.compress({
    "minBytes": 1024,
    "level": 6,
}));
```

Gzips response bodies when the client advertises
`Accept-Encoding: gzip` and the body is over the `minBytes` threshold
(default 1 KiB). Adds `Content-Encoding: gzip` and `Vary:
Accept-Encoding`.

### Rate limit

```gb
gebweb.before(app, gebweb.rateLimit({
    "rate": 100,
    "burst": 200,
    "keyFn": func(dict<string, any> req): string {
        return ((req["headers"] as dict<string, any>)["X-Client-Id"] ?? "") as string;
    },
}));
```

Token-bucket rate-limit per client key. Default key is the remote
address. Exhaustion returns 429 with a `Retry-After` header.

## Per-route overrides

Decorate a single handler method (or its enclosing controller
class) with `@RateLimit(perSecond, burst?)` or `@Cors({...})` to
apply policy to that route only. The decorators are recognised
when the framework wires the route, so they sit alongside the
routing decorators on the same method:

```gb
class AuthController {
    @Post("/login")
    @RateLimit(5, 10)
    func login(LoginDto body): dict<string, any> { /* ... */ }
}

@Cors({"allowOrigins": ["https://admin.example.com"]})
class AdminController {
    @Get("/admin/users")
    func list(): list<User> { /* ... */ }
}
```

`@RateLimit` is a token-bucket cap scoped to one route; key by
remote address. `@Cors` applies an opts dict identical to the
app-wide `gebweb.cors(opts)` factory, just to that route. Decorate
the controller class to apply either policy to every route in it.

Pair with the app-wide registrations: the per-route decorators
layer on top. For example, a global `gebweb.rateLimit({"rate":
100})` caps the whole app, and `@RateLimit(5, 10)` further caps
`/login` to 5 rps.

## Composing

Middleware composes in registration order. The last `use`-registered
middleware runs first when shaping the response (responses flow back
through the stack in reverse). Pair `before` short-circuits with
`use` shaping for end-to-end pipelines.

## Custom middleware

```gb
gebweb.use(app, func(dict<string, any> req, dict<string, any> res): dict<string, any> {
    let headers = (res["headers"] ?? {}) as dict<string, any>;
    headers["X-Powered-By"] = "Gebweb";
    res["headers"] = headers;
    return res;
});
```

A `before` middleware returns either `null` (continue) or a response
dict (short-circuit). A `use` / `after` middleware always returns the
(possibly modified) response.

## Reference

- `gebweb.use(app, middleware): GebwebApp` - response-phase.
- `gebweb.before(app, middleware): GebwebApp` - request-phase
  short-circuit.
- `gebweb.after(app, middleware): GebwebApp` - alias of `use`.

Built-in factories (re-exported on `gebweb` and on
`gebweb.middleware`):

| Name              | Signature                                  |
|-------------------|--------------------------------------------|
| `cors`            | `(opts: dict): callable`                   |
| `securityHeaders` | `(opts: dict): callable`                   |
| `requestId`       | `(): callable`                             |
| `requestLog`      | `(opts: dict): callable`                   |
| `compress`        | `(opts: dict): callable`                   |
| `rateLimit`       | `(opts: dict): callable` - typically `before` |

## ETag and conditional GET (1.1.0)

`gebweb.useEtag(app, opts)` registers a response-phase middleware
that hashes every eligible 2xx body with SHA-256 and emits a weak
ETag. Subsequent requests with a matching `If-None-Match` header
return 304 with an empty body.

```gb
gebweb.useEtag(app);                          // default config
gebweb.useEtag(app, {"minBytes": 256});       // skip very small bodies
gebweb.useEtag(app, {"maxBytes": 65536});     // skip large bodies
gebweb.useEtag(app, {"weak": false});         // strong ETags (rarely useful)
```

The middleware skips error responses, empty bodies, and bodies
outside the configured `minBytes` / `maxBytes` window
(`0` / `1048576` by default).

## Server-Timing (1.1.0)

`gebweb.useServerTiming(app)` registers a before-middleware that
primes a per-request timing list and a response-phase middleware
that emits the `Server-Timing` header populated from that list.

```gb
gebweb.useServerTiming(app);

class WidgetController {
    @Get("/widget")
    func widget(dict<string, any> request): dict<string, any> {
        let row = gebweb.measureTiming(request, "db.query",
            func(): any { return widgetRepo.find("w1"); });
        gebweb.recordTiming(request, "render", 4);
        return row as dict<string, any>;
    }
}
```

`measureTiming(request, label, fn)` times the callable and
appends the duration; `recordTiming(request, label, ms)` appends
a pre-measured value. Labels are sanitised - non-token characters
become underscores - so they fit the spec without surprising the
browser dev-tools waterfall.

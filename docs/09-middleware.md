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
(`func(request, response): response`). `gebweb.after` is an alias.
`gebweb.before(app, callable)` registers a request-phase middleware
(`func(request): any` - return `null` to continue, or a response dict
to short-circuit).

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

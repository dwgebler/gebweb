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
`gebweb.compress(opts)`, `gebweb.rateLimit(opts)`,
`gebweb.abuseGuard(opts)`, `gebweb.waf(opts)`.

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

Token-bucket rate-limit per client key. The default key is the
engine-resolved client IP, which only honours `X-Forwarded-For` from
peers listed in the server's `trustedProxies` option, so a direct
client cannot spoof its key. Exhaustion returns 429 with a
`Retry-After` header. Buckets idle long enough to refill fully are
swept lazily, so limiter memory stays bounded under client churn.

### Distributed rate limit (Redis)

`gebweb.redisRateLimit(opts)` is a token-bucket rate limiter whose
bucket state lives in Redis, so every app instance drains and refills
the same bucket. Use it in multi-instance deployments where the
in-memory `rateLimit` would give each instance its own independent
cap.

Requires geblang 1.28.0+ (uses `redis.Client.eval`).

```gb
gebweb.before(app, gebweb.redisRateLimit({
    "address":   "localhost:6379",
    "perSecond": 50,
    "burst":     100,
    "keyFn": func(any req): string {
        return (req as dict<string, any>)["ip"] as string;
    },
}));
```

The bucket semantics match the in-memory `rateLimit`: up to `burst`
requests may be served in a burst; the bucket refills at `perSecond`
tokens per second. Exhaustion returns 429 with a `Retry-After: 1`
header and a `application/problem+json` body. The default client key
is the trusted-proxy-aware client IP (same as `rateLimit`).

**Options:**

| Option      | Default        | Meaning                                          |
|-------------|----------------|--------------------------------------------------|
| `address`   | required       | `"host:port"` of the Redis server.               |
| `perSecond` | `10`           | Refill rate in tokens per second.                |
| `burst`     | `perSecond * 2`| Maximum tokens (bucket capacity).                |
| `keyFn`     | client IP      | `func(request): string` - bucket key per client. |
| `failOpen`  | `true`         | `true` -> allow on Redis error; `false` -> 503.  |
| `message`   | `"rate limit exceeded"` | `detail` field in the 429 response.   |
| `prefix`    | `"rl:"`        | Key prefix in Redis.                             |
| `poolSize`  | `8`            | Max concurrent connections.                      |
| `password`  | none           | Redis `AUTH` password.                           |
| `db`        | `0`            | Redis logical database index.                    |
| `pool`      | none           | Pre-built `gebweb.redisPool(...)` (shared pool). |
| `logger`    | none           | `func(string): void` for warn messages.          |

**Fail-open / fail-closed:** a Redis error is caught and warn-logged.
With `failOpen: true` (default) the request is allowed through; the
limiter degrades gracefully when Redis is unreachable. With
`failOpen: false` the middleware returns 503 (`application/problem+json`).

**vs. in-memory `rateLimit`:** `rateLimit` maintains per-instance
in-memory buckets - each pod has its own count. `redisRateLimit`
shares one bucket across all instances; the limit is enforced
fleet-wide. The trade-off is a Redis round-trip per request
(typically under 1 ms on a local network).

### Abuse guard

```gb
gebweb.before(app, gebweb.abuseGuard({}));
```

Detects credential-scanner and exploit bots and bans the offending
client IP, short-circuiting all its further requests with 403 before
they reach routing. A request whose path matches a built-in list of
unambiguous probe patterns - credential files (`/.aws/credentials`,
`.git-credentials`, `.s3cfg`, `.netrc`), VCS exposure (`/.git/`),
`/.env`, `/wp-admin`, `/phpmyadmin`, path traversal, and similar -
counts against that client; once it crosses `threshold` (default 1, so
the first probe bans) the client is blocked for `banSeconds` (default
3600). Rate-limiting still routes each probe; this drops the source
outright, so a scanner hammering your service costs almost nothing.

```gb
gebweb.before(app, gebweb.abuseGuard({
    "banSeconds": 3600,
    "threshold": 1,
    "badPaths": ["/rest/users", "/admin/config"],   // merged with the built-ins
    "allowIps": ["203.0.113.10"],                    // never banned
    "onBan": func(string ip): void { log.warn("banned " + ip); },
}));
```

The identity key is the same trusted-proxy-aware client IP as
`rateLimit` (override with `keyFn`); an empty/unidentifiable key is
never banned, and ban records are swept once they lapse so memory
stays bounded.

### WAF (`gebweb.waf`)

```gb
gebweb.before(app, gebweb.waf({
    "rules": ["sqli", "xss", "rce", "traversal"],
    "mode": "block",
}));
```

A Web Application Firewall that inspects each inbound request for
common attack signatures and policy violations before routing. Returns
`null` to pass the request through, or a 403 `application/problem+json`
response to short-circuit it. Register with `gebweb.before`.

**Check order:** allowIps (bypass) -> denyIps -> blockedMethods ->
blockedHeaders -> maxBodyBytes -> blockUserAgents -> content signatures
(query string, then body, then header values). The first match decides.

**Options:**

| Option | Default | Meaning |
|--------|---------|---------|
| `rules` | all four | List of rule sets to enable: `"sqli"`, `"xss"`, `"rce"`, `"traversal"`. |
| `allowIps` | `[]` | IPs/CIDRs that bypass the WAF entirely (checked first). |
| `denyIps` | `[]` | IPs/CIDRs blocked outright. |
| `blockUserAgents` | `[]` | Case-insensitive substrings matched against `User-Agent`. |
| `maxBodyBytes` | `0` (off) | Reject requests whose body exceeds this size in bytes. |
| `allowedMethods` | `[]` (all) | If non-empty, only these HTTP methods are allowed. |
| `blockedHeaders` | `[]` | Header names whose presence is rejected. |
| `mode` | `"block"` | `"block"` (return 403) or `"log"` (detect-only, never blocks). |
| `onBlock` | `null` | `func(req, match)` called on every match (in both modes). |
| `ban` | `null` | `{threshold, banSeconds}`: escalate repeat offenders to a timed IP ban. |
| `message` | `"forbidden"` | The `detail` field in the 403 problem response. |
| `keyFn` | client IP | `func(req): string` override for the ban/deny key. |
| `logger` | `null` | Optional logger; falls back to the framework request logger. |

**Rule sets:**

- `"sqli"` - SQL injection signatures (UNION SELECT, tautologies, DDL, sleep, xp_cmdshell, etc.).
- `"xss"` - Cross-site scripting signatures (`<script>`, `javascript:`, event handlers, `document.cookie`, etc.).
- `"rce"` - Remote code execution signatures (shell commands, subshell expansion, system/exec calls, etc.).
- `"traversal"` - Path traversal signatures (`../`, `%2e%2e`, `/etc/passwd`, `/proc/self`, `file://`, etc.).

Pass a subset to `rules` to enable only those sets:

```gb
gebweb.before(app, gebweb.waf({
    "rules": ["sqli", "traversal"],
    "mode": "block",
}));
```

**IP allow/deny lists** accept plain IP addresses and IPv4/IPv6 CIDR
notation. `allowIps` is checked first - an allowlisted client skips all
WAF checks. A malformed entry is logged once and never matches (a typo
cannot take the app down):

```gb
gebweb.before(app, gebweb.waf({
    "allowIps": ["10.0.0.0/8", "2001:db8::/32", "192.168.1.5"],
    "denyIps":  ["203.0.113.0/24"],
    "mode": "block",
}));
```

**`onBlock` hook** fires on every match in both `block` and `log`
modes. The `match` argument is a dict:

```gb
{
    "rule":   "sqli",              /* rule set name, or "ip", "method", etc. */
    "target": "query",             /* "ip", "method", "header", "body", "query", "userAgent" */
    "value":  "union select ...",  /* matched snippet */
}
```

```gb
gebweb.before(app, gebweb.waf({
    "mode": "block",
    "onBlock": func(any req, dict<string, any> match): void {
        log.warn("WAF block", {"rule": match["rule"], "target": match["target"]});
    },
}));
```

**`mode: "log"`** records every match (via `onBlock` and the logger)
but never blocks the request. Use this to trial new rules against real
traffic before enforcing:

```gb
gebweb.before(app, gebweb.waf({
    "rules": ["sqli", "xss", "rce", "traversal"],
    "mode": "log",
    "onBlock": func(any req, dict<string, any> match): void {
        log.info("WAF trial match", match);
    },
}));
```

Switch `mode` to `"block"` once the false-positive rate is acceptable.

**Ban escalation** (optional): reuse the `abuseGuard`-style ban store
to escalate repeat offenders. A WAF-blocked request increments the
client's score; at `threshold` it is banned for `banSeconds` and
subsequent requests short-circuit before any inspection. The ban store
is independent of any `abuseGuard` instance:

```gb
gebweb.before(app, gebweb.waf({
    "ban": {"threshold": 5, "banSeconds": 3600},
}));
```

**Composing with `abuseGuard`:** register both middlewares with
`gebweb.before`; they compose and are independent. `abuseGuard` catches
well-known probe paths (credential files, admin panels, VCS dirs);
`waf` catches content-level attacks in the query string, body, and
headers. An IP allowlisted in one is not automatically allowlisted in
the other.

```gb
gebweb.before(app, gebweb.abuseGuard({}));
gebweb.before(app, gebweb.waf({}));
```

**Caveat:** the content-signature rules can produce false positives on
free-text or technical fields that legitimately contain SQL or shell
fragments (for example, a developer tools API or a code-hosting
webhook). Trial new rule sets with `mode: "log"` before enforcing, and
disable a noisy set by removing it from `rules`.

### Conditional GET (ETag)

```gb
gebweb.use(app, gebweb.etag());
```

Tags 200 GET/HEAD responses with a weak content-hash ETag and answers
matching `If-None-Match` revalidations with an empty 304, cutting
repeat-view bandwidth. Responses that already carry an ETag are left
alone.

### Request context: correlation ids

```gb
gebweb.useRequestContext(app);
```

Every request gets a correlation id - the inbound `X-Request-Id`
header when present, a generated one otherwise. Handlers read it via
`req.requestId()`, the response echoes it, and
`gebweb.requestLogger(req)` returns a logger whose entries all carry
the id:

```gb
@Get("/orders/{id}")
func show(gebweb.Request req, @PathParam("id") int id): dict<string, any> {
    let logger = gebweb.requestLogger(req);
    logger.info("loading order", {"order": id});
    ...
}
```

Propagate the id to outbound calls and job payloads explicitly:
`http.request(url).withHeader("X-Request-Id", req.requestId())`.

## Per-route overrides

Decorate a single handler method (or its enclosing controller
class) with `@RateLimit(perSecond, burst?)`, `@Cors({...})`, or
`@MaxBody(bytes)` to apply policy to that route only. The decorators are recognised
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
| `rateLimit`       | `(opts: dict): callable` - typically `before` (per-instance) |
| `redisRateLimit`  | `(opts: dict): callable` - `before`; distributed (geblang 1.28.0+) |
| `abuseGuard`      | `(opts: dict): callable` - register with `before` |
| `waf`             | `(opts: dict): callable` - register with `before` |

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

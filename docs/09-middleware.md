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

Middleware also runs for requests served by `useStaticAssets`, so a gate
that denies anonymous requests denies anonymous asset requests too.
See [Asset pipeline](26-assets.md#middleware) to exempt the asset prefix
or turn middleware off for assets.

For convenience the facade also re-exports the built-in factories:
`gebweb.cors(opts)`, `gebweb.securityHeaders(opts)`,
`gebweb.requestId()`, `gebweb.requestLog(opts)`,
`gebweb.compress(opts)`, `gebweb.rateLimit(opts)`,
`gebweb.abuseGuard(opts)`, `gebweb.waf(opts)`.

## Guaranteed request cleanup

Response middleware does not run after every exit (for example an unexpected
handler error or a route miss). Release request-local resources with a dispatcher
finalizer instead:

```gb
gebweb.onRequestEnd(app, func(dict<string, any> request): void {
    releaseRequestResources();
});
```

Finalizers receive the raw request dictionary and run in reverse registration
order after dispatch, including static responses, middleware short circuits,
404s, admission rejections and exceptions. They cannot modify the response.
An exception in a finalizer is reported to stderr; remaining finalizers still
run and the original dispatch response or exception is preserved. Register
callbacks during startup, before serving concurrent requests. `TestClient`
and `dispatcher` exercise the same lifecycle as the HTTP server.

## Built-in middleware

### CORS

```gb
gebweb.use(
    app,
    gebweb.cors({
        "origin": "https://app.example.com",
        "credentials": true,
        "methods": ["GET", "POST", "PUT", "DELETE"],
        "headers": ["Content-Type", "Authorization"],
        "maxAge": 86400,
    })
);
```

Defaults are permissive (`origin: "*"`); tighten as needed. A
preflight `OPTIONS` request is handled automatically.

### Security headers

```gb
gebweb.use(
    app,
    gebweb.securityHeaders({"extra": {"Strict-Transport-Security": "max-age=31536000"}})
);
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
gebweb.use(
    app,
    gebweb.compress({
        "minBytes": 1024,
        "level": 6,
    })
);
```

Gzips response bodies when the client advertises
`Accept-Encoding: gzip` and the body is over the `minBytes` threshold
(default 1 KiB). Adds `Content-Encoding: gzip` and `Vary:
Accept-Encoding`.

### Rate limit

```gb
gebweb.before(
    app,
    gebweb.rateLimit({
        "perSecond": 100,
        "burst": 200,
        "keyFn": func(gebweb.Request req): string { return req.header("X-Client-Id") ?? req.clientIp(); },
    })
);
```

Token-bucket rate-limit per client key. The default key is the
engine-resolved client IP, which only honours `X-Forwarded-For` from
peers listed in the server's `trustedProxies` option, so a direct
client cannot spoof its key. IPv6 clients are keyed by their /64
network (`2001:db8:1:2::/64`), because a single host usually controls
a whole /64 and could otherwise dodge the limit by rotating addresses
(a link-local zone such as `%eth0` is ignored);
`gebweb.middleware.rateLimitClientKey(ip)` returns the default key if a
custom `keyFn` wants to build on it. Exhaustion returns 429 with a
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
gebweb.before(
    app,
    gebweb.redisRateLimit({
        "address":   "localhost:6379",
        "perSecond": 50,
        "burst":     100,
        "keyFn": func(any req): string { return (req as dict<string, any>)["ip"] as string; },
    })
);
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
| `keyFn`     | client IP, IPv6 by /64 | `func(request): string` - bucket key per client. |
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
gebweb.before(app, gebweb.abuseGuard({"banSeconds": 3600, "threshold": 1, "badPaths": ["/rest/users", "/admin/config"], "allowIps": ["203.0.113.10"], "onBan": func(string ip): void {
    log.warn("banned " + ip);
}}));
```

The identity key is the same trusted-proxy-aware client IP as
`rateLimit`, so an IPv6 client is banned by its /64 network and
`onBan` receives that network (`2001:db8:1:2::/64`); `allowIps` still
match exact addresses. A custom `keyFn` is used as returned. An
empty/unidentifiable key is never banned, and ban records are swept once they lapse so memory
stays bounded. The 403 response uses the generic `about:blank` Problem
Details type.

Pass a `net.IpSet` as `blocklist` (and the same set as the server's
`ipBlocklist`) to push bans down to the connection: the first probe
still gets a 403, and from then on the banned client's connections are
closed as soon as they arrive, before TLS, HTTP parsing or any gebweb
code. See [Connection blocklist](#connection-blocklist).

### Connection blocklist

The WAF and abuse guard answer an offending request with a 403, but the
request has already cost a connection, a TLS handshake, a parsed body
and middleware dispatch. A `net.IpSet` shared between the middleware and
the server moves repeat offenders out of that path entirely:

```gb
import net;

let blocklist = net.ipSet(["203.0.113.0/24"]);   # static entries are fine too
gebweb.before(app, gebweb.abuseGuard({"blocklist": blocklist}));
gebweb.before(app, gebweb.waf({"ban": {"threshold": 3, "banSeconds": 900}, "blocklist": blocklist}));
gebweb.serve(app, ":8080", {"ipBlocklist": blocklist});
```

The server closes a blocked client's connection as soon as it is
accepted, and re-checks every request before reading its body, so a
client banned while it holds a kept-alive connection is cut off on its
next request. Behind a load balancer, list it in `trustedProxies` so the
check uses the forwarded client IP rather than the proxy's. Bans expire
from the set after `banSeconds`; keys from a custom `keyFn` that are not
IP addresses are never added.

### Admission control

```gb
gebweb.useAdmissionControl(app, {"maxConcurrent": 200});
```

Bounds the number of requests executing through the dispatcher at once. While
`maxConcurrent` requests are in flight, any further request is
rejected with a 503 (and `Retry-After: 1`) before any handler runs, so
a traffic spike sheds load instead of exhausting memory. Unlike the
per-client `rateLimit`, this is one shared gate.

The gate wraps the whole dispatcher: it acquires a slot at the top of every
request (static assets and `before`-middleware short-circuits included) and
releases it in a `finally`, so an uncaught error anywhere in the request still
frees the slot. `maxConcurrent` must be a positive integer (a non-positive value
throws at wiring time). For a hard whole-server limit that also covers
connections before they reach the application, use the HTTP server's
`maxConcurrent` option.

Customise the rejection with `onOverload`, a `(): response` callable:

```gb
gebweb.useAdmissionControl(app, {
    "maxConcurrent": 200,
    "onOverload": func(): any { return http.jsonResponse({"error": "server busy"}, 503); },
});
```

Choose `maxConcurrent` from a measurement of per-request
memory under load, not a guess: too low rejects healthy traffic, too
high defeats the protection.

### WAF (`gebweb.waf`)

```gb
gebweb.before(
    app,
    gebweb.waf({
        "rules": ["sqli", "xss", "rce", "traversal"],
        "mode": "block",
    })
);
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
| `exclusions` | `[]` | Entries exempting paths, fields or headers from the content signatures; see below. |
| `ban` | `null` | `{threshold, banSeconds}`: escalate repeat offenders to a timed IP ban. |
| `blocklist` | `null` | A `net.IpSet`; each ban is also added to it for `banSeconds`, so a server using the same set as `ipBlocklist` drops the client at connect time. |
| `message` | `"forbidden"` | The `detail` field in the 403 problem response. |
| `keyFn` | client IP | `func(req): string` override for the ban/deny key. By default bans key IPv6 clients by their /64, while `allowIps` / `denyIps` match the exact address. |
| `logger` | `null` | Optional logger; falls back to the framework request logger. |

**Rule sets:**

- `"sqli"` - SQL injection signatures (UNION SELECT, tautologies, DDL, sleep, xp_cmdshell, etc.).
- `"xss"` - Cross-site scripting signatures (`<script>`, `javascript:`, event handlers, `document.cookie`, etc.).
- `"rce"` - Remote code execution signatures (shell commands, subshell expansion, system/exec calls, etc.).
- `"traversal"` - Path traversal signatures (`../`, `%2e%2e`, `/etc/passwd`, `/proc/self`, `file://`, etc.).

Signatures are matched against the query string, the body and the header
values. The query string and `application/x-www-form-urlencoded` bodies are
also URL-decoded, repeatedly (up to three layers), so an encoded or
double-encoded payload is caught; a malformed escape cannot stop the
decoding. Other bodies, such as JSON, are scanned as sent.

Pass a subset to `rules` to enable only those sets:

```gb
gebweb.before(
    app,
    gebweb.waf({
        "rules": ["sqli", "traversal"],
        "mode": "block",
    })
);
```

**IP allow/deny lists** accept plain IP addresses and IPv4/IPv6 CIDR
notation. `allowIps` is checked first - an allowlisted client skips all
WAF checks. A malformed entry is logged once and never matches (a typo
cannot take the app down):

```gb
gebweb.before(
    app,
    gebweb.waf({
        "allowIps": ["10.0.0.0/8", "2001:db8::/32", "192.168.1.5"],
        "denyIps":  ["203.0.113.0/24"],
        "mode": "block",
    })
);
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
is independent of any `abuseGuard` instance. Blocked requests use the
generic `about:blank` Problem Details type:

```gb
gebweb.before(app, gebweb.waf({"ban": {"threshold": 5, "banSeconds": 3600}}));
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
disable a noisy set by removing it from `rules`, or exempt just the
affected paths and fields with `exclusions`.

**Exclusions** exempt known free-text inputs from the content
signatures while the rest of the request stays scanned:

```gb
gebweb.before(
    app,
    gebweb.waf({
        "exclusions": [
            {"paths": ["/admin"]},
            {"fields": ["password", "password_confirm"]},
            {"paths": ["/contact"], "fields": ["message"]},
            {"headers": ["Referer"], "rules": ["xss"]},
        ],
    })
);
```

Each entry takes any of these keys:

| Key | Meaning |
|-----|---------|
| `paths` | Path prefixes the entry applies to, matched on `/` boundaries (`"/admin"` covers `/admin` and `/admin/pages`, not `/administrator`). Omitted: every path. |
| `fields` | Query, form (`application/x-www-form-urlencoded`) and JSON field names whose values are not scanned. JSON keys match at any depth, and the whole value under the key is exempt. Names are case-sensitive. |
| `headers` | Header names (case-insensitive) whose values are not scanned. |
| `rules` | Limit the entry to these rule sets; the other sets still apply. Omitted: all rule sets. |

An entry with `paths` but no `fields` or `headers` exempts the whole
request on those paths. Field names are always scanned, only the values
are exempt. Exclusions affect the content signatures only: IP lists,
bans, method, header, body-size and user-agent checks still apply on an
excluded path. Multipart bodies are still scanned as a whole. A
malformed entry (unknown key or rule set, a path not starting with `/`,
or an entry with only `rules`) raises `ValueError` when `gebweb.waf` is
called.

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
    func login(LoginDto body): dict<string, any> {} /* ... */
}

@Cors({"allowOrigins": ["https://admin.example.com"]})
class AdminController {
    @Get("/admin/users")
    func list(): list<User> {} /* ... */
}
```

`@RateLimit` is a token-bucket cap scoped to one route; key by
remote address. `@Cors` applies an opts dict identical to the
app-wide `gebweb.cors(opts)` factory, just to that route. Decorate
the controller class to apply either policy to every route in it.

Pair with the app-wide registrations: the per-route decorators
layer on top. For example, a global `gebweb.rateLimit({"perSecond":
100})` caps the whole app, and `@RateLimit(5, 10)` further caps
`/login` to 5 rps. `@RateLimit` keys clients the same way as
`rateLimit`: by client IP, and IPv6 clients by their /64 network.

## Composing

Middleware composes in registration order. The last `use`-registered
middleware runs first when shaping the response (responses flow back
through the stack in reverse). Pair `before` short-circuits with
`use` shaping for end-to-end pipelines.

## Custom middleware

```gb
import http;

# Response phase: receives the request and the response, returns the
# response to send (built with the immutable with* builders).
gebweb.use(app, func(gebweb.Request req, http.Response res): http.Response {
    if (res.status() >= 500) {
        io.println("server error on ${req.method()} ${req.path()}");
    }
    return res.withHeader("X-Powered-By", "Gebweb");
});

# Request phase: return null to continue, or a response to short-circuit.
gebweb.before(app, func(gebweb.Request req): ?http.Response {
    if (req.header("X-Maintenance") == "1") {
        return http.response("Down for maintenance", 503);
    }
    return null;
});
```

Both phases receive the rich `gebweb.Request` (not a dict), and the
response phase receives an `http.Response`: read it with `status()`,
`header(name)` and `body()`, and change it with `withStatus`,
`withHeader` and `withBody`, which return a new response. A `before`
middleware returns either `null` (continue) or a response
(short-circuit). A `use` / `after` middleware always returns the
(possibly modified) response.

## Reference

- `gebweb.use(app, middleware): GebwebApp` - response-phase.
- `gebweb.before(app, middleware): GebwebApp` - request-phase
  short-circuit.
- `gebweb.after(app, middleware): GebwebApp` - alias of `use`.
- `gebweb.useAdmissionControl(app, opts): GebwebApp` - bounds
  concurrent routed requests (`maxConcurrent`, optional
  `onOverload`); registers the gate + release in one call.

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
that hashes eligible 2xx bodies with SHA-256 and emits a weak
ETag. Subsequent GET/HEAD requests with a matching `If-None-Match` header
return 304 with an empty body. Other request methods retain their status
and body, and can still receive an ETag.

```gb
gebweb.useEtag(app);
gebweb.useEtag(app, {"minBytes": 256});
gebweb.useEtag(app, {"maxBytes": 65536});
gebweb.useEtag(app, {"weak": false});
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
        let row = gebweb.measureTiming(
            request,
            "db.query",
            func(): any { return widgetRepo.find("w1"); }
        );
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

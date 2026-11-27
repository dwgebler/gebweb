# Middleware

<!-- doctest:strict -->

Middleware handles concerns that apply across routes: CORS, security
headers, request ids, logging, compression, rate limiting, blocking attacks.
It runs in two phases around routing and the handler:

- **Request phase** (`gebweb.before`): runs before routing, in registration
  order. Each function receives the request and returns `null` to continue or
  a response to answer the request right there.
- **Response phase** (`gebweb.use`, alias `gebweb.after`): runs after the
  handler, in reverse registration order. Each function receives the request
  and the response and returns the response to send.

## Writing middleware

<!-- doctest:run -->
```gb
import gebweb;
import http;
import io;

@Controller("/status")
class StatusController {
    @Get("")
    func status(): dict<string, any> {
        return {"ok": true};
    }
}

func maintenance(gebweb.Request req): ?http.Response {
    if (req.header("X-Maintenance") == "1") {
        return http.response("Down for maintenance", 503);
    }
    return null;
}

func tagFirst(gebweb.Request req, http.Response res): http.Response {
    return res.withHeader("X-Order", "${res.header("X-Order") ?? ""}first");
}

func tagSecond(gebweb.Request req, http.Response res): http.Response {
    return res.withHeader("X-Order", "${res.header("X-Order") ?? ""}second,");
}

let app = gebweb.app();
gebweb.before(app, maintenance);
gebweb.use(app, tagFirst);
gebweb.use(app, tagSecond);

let client = gebweb.TestClient(app);
let ok = client.get("/status");
io.println("${ok.status} ${ok.headers["X-Order"]}");
let down = client.request("GET", "/status", null, {"X-Maintenance": "1"});
io.println("${down.status} ${down.text()} ${down.headers["X-Order"]}");
```

Output:

```text
200 second,first
503 Down for maintenance second,first
```

- The request-phase `maintenance` answered the second request itself, so the
  route never ran, but response-phase middleware still applied to its answer.
- The response phase runs in reverse: `tagSecond`, registered last, ran
  first.
- Responses are immutable: `withStatus`, `withHeader` and `withBody` return a
  changed copy. Read them with `status()`, `header(name)` and `body()`.

Middleware also runs for requests served by
[`useStaticAssets`](26-assets.md#middleware), so a gate that rejects
anonymous requests rejects anonymous asset requests too.

### Cleanup that always runs

Response middleware does not run when a handler throws an unexpected error or
when no route matches. Release request-owned resources in a request
finalizer instead:

<!-- doctest:fragment context=app -->
```gb
gebweb.onRequestEnd(app, func(dict<string, any> request): void {
    let handle = request["uploadTempFile"];
    if (handle != null) {
        io.remove(handle as string);
    }
});
```

Finalizers receive the raw request dictionary and run in reverse
registration order after every request: static files, middleware
short-circuits, 404s, admission rejections and exceptions included. They
cannot change the response. An exception in one is logged and the others still
run. Register them during startup.

## Built-in middleware

| Middleware | Phase | Purpose |
|------------|-------|---------|
| `gebweb.cors(opts)` | `use` | Cross-origin headers and preflight answers |
| `gebweb.securityHeaders(opts)` | `use` | Basic security headers; see [Security headers](27-security.md) for `useSecurity` |
| `gebweb.requestId()` | `use` | Echo or generate `X-Request-ID` |
| `gebweb.useRequestContext(app)` | both | Correlation id on the request, response and logs |
| `gebweb.requestLog(opts)` | `use` | One log entry per request |
| `gebweb.compress(opts)` | `use` | Gzip response bodies |
| `gebweb.useEtag(app, opts)` | `use` | ETags and 304 revalidation |
| `gebweb.useServerTiming(app)` | both | `Server-Timing` header |
| `gebweb.rateLimit(opts)` | `before` | Per-client token bucket, per process |
| `gebweb.redisRateLimit(opts)` | `before` | Per-client token bucket shared through Redis |
| `gebweb.abuseGuard(opts)` | `before` | Ban clients that probe for credentials and admin panels |
| `gebweb.waf(opts)` | `before` | Block injection and traversal payloads |
| `gebweb.useAdmissionControl(app, opts)` | around dispatch | Shed load above a concurrency limit |

The factories are also available from `gebweb.middleware`.

### CORS

Browsers block a page from calling an API on another origin unless the API
allows it with CORS headers. For requests with a JSON body or custom headers,
the browser first sends an `OPTIONS` preflight; `gebweb.cors` answers it:

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/api")
class OrdersApi {
    @Post("/orders")
    func create(): dict<string, any> {
        return {"created": true};
    }
}

let app = gebweb.app();
gebweb.use(
    app,
    gebweb.cors({
        "origin": "https://app.example.com",
        "methods": ["GET", "POST"],
        "headers": ["Content-Type", "Authorization"],
        "credentials": true,
        "maxAge": 600,
    })
);

let client = gebweb.TestClient(app);
let pre = client.request("OPTIONS", "/api/orders", null, {
    "Origin": "https://app.example.com",
    "Access-Control-Request-Method": "POST",
});
io.println(
    "${pre.status} ${pre.headers["Access-Control-Allow-Origin"]} max-age=${pre.headers["Access-Control-Max-Age"]}"
);
let post = client.request("POST", "/api/orders", {}, {"Origin": "https://app.example.com"});
io.println("${post.status} ${post.headers["Access-Control-Allow-Methods"]}");
```

Output:

```text
204 https://app.example.com max-age=600
200 GET,POST
```

| Option | Default | Meaning |
|--------|---------|---------|
| `origin` | `"*"` | The allowed origin, sent as `Access-Control-Allow-Origin`. |
| `methods` | `GET,POST,PUT,PATCH,DELETE,OPTIONS` | Allowed methods. |
| `headers` | `Content-Type,Authorization` | Allowed request headers. |
| `credentials` | `false` | Send `Access-Control-Allow-Credentials: true`, allowing cookies. |
| `maxAge` | none | Seconds the browser may cache a preflight answer. |

With `credentials: true`, set `origin` to a specific origin: browsers refuse
credentials with `*`. For CORS on some routes only, see
[Per-route overrides](#per-route-overrides).

### Security headers

`gebweb.securityHeaders({})` adds `X-Content-Type-Options: nosniff`,
`X-Frame-Options: DENY` and `Referrer-Policy: strict-origin-when-cross-origin`;
`extra` adds or replaces headers. For a full policy including CSP with
per-request nonces and HSTS, use `gebweb.useSecurity` from
[Security headers](27-security.md) instead.

### Request ids and correlation

`gebweb.requestId()` echoes an incoming `X-Request-ID`, or generates one, on
every response. `gebweb.useRequestContext(app)` goes further: the id is also
available to handlers and attached to their log entries:

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/orders")
class OrderController {
    @Get("/{id}")
    func show(gebweb.Request req, int id): dict<string, any> {
        return {"order": id, "requestId": req.requestId()};
    }
}

let app = gebweb.app();
gebweb.useRequestContext(app);
let r = gebweb.TestClient(app).request("GET", "/orders/7", null, {"X-Request-Id": "trace-abc"});
io.println("${r.text()} ${r.headers["X-Request-Id"]}");
```

Output:

```text
{"order":7,"requestId":"trace-abc"} trace-abc
```

`gebweb.requestLogger(req)` returns a logger whose entries carry the id. Pass
the id on to outbound calls and job payloads yourself, for example with
`http.request(url).withHeader("X-Request-Id", req.requestId())`.

### Request log

`gebweb.requestLog({})` logs one structured entry per request through the
`log` module; `{"logger": gebweb.logger(app)}` sends it through the framework
logger. Installing it replaces the one-line-per-request log written in the
dev environment; see [Deployment](37-deployment.md#logging).

### Compression

`gebweb.compress({"minBytes": 1024})` gzips response bodies when the client
accepts gzip and the body is at least `minBytes` (default 1024), adding
`Content-Encoding: gzip` and `Vary: Accept-Encoding`. Images, audio, video,
archives and `application/octet-stream` are skipped; `skipContentTypes` adds
more content-type prefixes to skip. Files sent with `gebweb.file` are streamed
and not compressed; precompress static assets instead
([Assets](26-assets.md)).

### ETags

`gebweb.useEtag(app, opts)` tags 2xx responses with a SHA-256 ETag and answers
a GET or HEAD whose `If-None-Match` matches with an empty 304. Options:
`minBytes` (default 0) and `maxBytes` (default 1048576) bound which bodies are
hashed, and `weak` (default `true`) chooses weak or strong tags. Responses
that already carry an ETag are left alone. `gebweb.etag(opts)` is the same as
a middleware to register with `gebweb.use`.

### Server timing

`gebweb.useServerTiming(app)` adds a `Server-Timing` header that browser
developer tools show as a request waterfall. Handlers add entries through the
raw request:

<!-- doctest:fragment context=controller -->
```gb
@Get("/report")
func report(gebweb.Request req): dict<string, any> {
    let rows = gebweb.measureTiming(req.toDict(), "db.query", func(): any { return [1, 2, 3]; });
    gebweb.recordTiming(req.toDict(), "render", 4);
    return {"rows": rows};
}
```

`measureTiming(request, label, fn)` times a function and records the result;
`recordTiming(request, label, ms)` records a value you measured. Characters a
header label cannot contain become underscores.

### Rate limiting

`gebweb.rateLimit(opts)` gives each client a token bucket: up to `burst`
requests at once, refilled at `perSecond`. An empty bucket answers 429 with
`Retry-After`:

<!-- doctest:fragment context=app -->
```gb
gebweb.before(app, gebweb.rateLimit({"perSecond": 20, "burst": 40}));
```

| Option | Default | Meaning |
|--------|---------|---------|
| `perSecond` | `10` | Refill rate. |
| `burst` | `perSecond * 2` | Bucket size. |
| `keyFn` | client IP | `func(gebweb.Request): string` choosing the bucket. |
| `message` | `"rate limit exceeded"` | The 429 `detail`. |

The default key is the client IP as resolved through `trustedProxies`, so a
direct client cannot spoof it with `X-Forwarded-For`; IPv6 clients share a
bucket per /64 network, since one host usually controls a whole /64.
`gebweb.middleware.rateLimitClientKey(ip)` returns that default key for a
custom `keyFn` to build on. Idle buckets are swept, so memory stays bounded.

Each process keeps its own buckets. With several instances, use
`gebweb.redisRateLimit`, which keeps the buckets in Redis so the limit applies
across the fleet, at the cost of a Redis round trip per request:

<!-- doctest:fragment context=app -->
```gb
gebweb.before(
    app,
    gebweb.redisRateLimit({"address": "localhost:6379", "perSecond": 50, "burst": 100})
);
```

| Option | Default | Meaning |
|--------|---------|---------|
| `address` | required | Redis `host:port`. |
| `perSecond`, `burst`, `keyFn`, `message` | as `rateLimit` | |
| `failOpen` | `true` | On a Redis error, allow the request (`true`) or answer 503 (`false`). |
| `prefix` | `"rl:"` | Redis key prefix. |
| `pool` | none | A shared `gebweb.redisPool(...)`. |
| `poolSize`, `password`, `db` | `8`, none, `0` | Connection settings when no `pool` is given. |
| `logger` | none | `func(string): void` for warnings. |

### Abuse guard

Credential scanners and exploit bots request paths no real client asks for.
`gebweb.abuseGuard` bans a client after such a probe, before routing:

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/")
class HomeController {
    @Get("")
    func home(): string {
        return "welcome";
    }
}

let app = gebweb.app();
gebweb.before(app, gebweb.abuseGuard({"banSeconds": 3600}));
let client = gebweb.TestClient(app);
io.println(client.get("/").status);
io.println(client.get("/.env").status);
io.println(client.get("/").status);
```

Output:

```text
200
403
403
```

The probe for `/.env` banned the client, so its next ordinary request was
refused too. The built-in list covers credential files (`/.aws/credentials`,
`.git-credentials`, `.netrc`), VCS directories (`/.git/`), `/.env`, admin
panels such as `/wp-admin` and `/phpmyadmin`, path traversal and similar.

| Option | Default | Meaning |
|--------|---------|---------|
| `threshold` | `1` | Probes before a ban. |
| `banSeconds` | `3600` | Ban length. |
| `badPaths` | `[]` | More probe paths to treat the same way. |
| `allowIps` | `[]` | Addresses never banned. |
| `keyFn` | client IP | As for `rateLimit`; IPv6 is banned per /64. |
| `onBan` | none | `func(string key): void`, called when a client is banned. |
| `blocklist` | none | A `net.IpSet` that bans are also added to; see below. |
| `message` | `"forbidden"` | The 403 `detail`. |

### Connection blocklist

A banned client still costs a connection, a TLS handshake and a parsed
request before the 403. Share a `net.IpSet` between the abuse guard or WAF and
the server, and the server drops banned clients as soon as they connect:

<!-- doctest:fragment context=app -->
```gb
let blocklist = net.ipSet(["203.0.113.0/24"]);
gebweb.before(app, gebweb.abuseGuard({"blocklist": blocklist}));
gebweb.before(app, gebweb.waf({"ban": {"threshold": 3, "banSeconds": 900}, "blocklist": blocklist}));
gebweb.cli(app, {"name": "orders", "server": {"ipBlocklist": blocklist}});
```

The server also checks each request on a kept-alive connection, so a client
banned mid-connection is cut off at its next request. Behind a load balancer,
list it in `trustedProxies` so the forwarded client address is checked. Bans
leave the set after `banSeconds`.

### Admission control

`gebweb.useAdmissionControl(app, {"maxConcurrent": 200})` answers 503 with
`Retry-After: 1`, before any middleware or handler, while 200 requests are
already in progress. Unlike `rateLimit` it is one shared limit, protecting the
process rather than limiting a client. `onOverload`, a function returning a
response, replaces the 503:

<!-- doctest:fragment context=app -->
```gb
gebweb.useAdmissionControl(app, {
    "maxConcurrent": 200,
    "onOverload": func(): any { return http.jsonResponse({"error": "server busy"}, 503); },
});
```

The slot is released even when the request fails. Choose `maxConcurrent` from
measured memory per request under load. How it relates to the server's own
limit and the database pool is covered in
[Database connections](06b-database-connections.md#requests-connections-and-queues).

### Web application firewall

See the dedicated [WAF chapter](27a-waf.md) for configuration, runnable
examples, exclusions and operational limits.

`gebweb.waf(opts)` inspects each request for attack payloads and answers 403
Problem Details on a match:

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/search")
class SearchController {
    @Get("")
    func search(string q): dict<string, any> {
        return {"q": q};
    }
}

let app = gebweb.app();
gebweb.before(app, gebweb.waf({"rules": ["sqli", "xss", "traversal"]}));
let client = gebweb.TestClient(app);
io.println(client.get("/search?q=garden+chairs").status);
io.println(client.get("/search?q=1%27+UNION+SELECT+password+FROM+users--").status);
io.println(client.get("/search?q=%3Cscript%3Ealert(1)%3C%2Fscript%3E").status);
```

Output:

```text
200
[waf] rule=sqli target=query ip=127.0.0.1 method=GET path=/search
403
[waf] rule=xss target=query ip=127.0.0.1 method=GET path=/search
403
```

Each block is also logged with the rule, where the payload was found and the
client address; `logger` and `onBlock` redirect or extend that. Rule sets: `sqli` (SQL injection), `xss` (script injection), `rce` (shell and
command execution) and `traversal` (path traversal). Signatures are matched
against the query string, the body and header values; query strings and form
bodies are URL-decoded up to three times, so encoded payloads are caught. JSON
bodies are scanned as sent.

| Option | Default | Meaning |
|--------|---------|---------|
| `rules` | all four | Rule sets to apply. |
| `mode` | `"block"` | `"block"` answers 403; `"log"` only reports matches, for trialling rules. |
| `allowIps` | `[]` | Addresses or CIDRs that skip the WAF entirely. |
| `denyIps` | `[]` | Addresses or CIDRs always blocked. |
| `allowedMethods` | `[]` (all) | When set, only these methods are allowed. |
| `blockedHeaders` | `[]` | Header names whose presence is blocked. |
| `blockUserAgents` | `[]` | Case-insensitive `User-Agent` substrings to block. |
| `maxBodyBytes` | `0` (off) | Block bodies larger than this. |
| `exclusions` | `[]` | Inputs exempt from the signatures; see below. |
| `onBlock` | none | `func(any request, dict<string, any> match)`, called on every match in both modes; `match` has `rule`, `target` and `value`. |
| `ban` | none | `{"threshold": n, "banSeconds": s}`: ban repeat offenders. |
| `blocklist` | none | A `net.IpSet` that bans are added to. |
| `keyFn` | client IP | Key for bans; IPv6 per /64. |
| `message` | `"forbidden"` | The 403 `detail`. |
| `logger` | Print to stdout | Callback receiving the formatted match line. |

Checks run in this order, and the first match decides: `allowIps`, `denyIps`,
`allowedMethods`, `blockedHeaders`, `maxBodyBytes`, `blockUserAgents`, then the
signatures. A malformed address entry is logged once and never matches.

Signature rules can match legitimate text, such as a code snippet in a support
form. Trial new rules with `mode: "log"`, then exempt the affected inputs:

<!-- doctest:fragment context=app -->
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

| Exclusion key | Meaning |
|---------------|---------|
| `paths` | Path prefixes, matched on `/` boundaries; omitted means every path. |
| `fields` | Query, form and JSON field names whose values are not scanned; JSON keys match at any depth. |
| `headers` | Header names whose values are not scanned. |
| `rules` | Limit the exclusion to these rule sets. |

An entry with `paths` alone exempts whole requests on those paths. Exclusions
affect only the signatures; IP lists, bans and the other checks still apply.
A malformed entry raises `ValueError` when `gebweb.waf` is called. The WAF and
the abuse guard are independent: register both to catch both probe paths and
payloads.

## Per-route overrides

`@RateLimit(perSecond, burst)`, `@Cors({...})` and `@MaxBody(bytes)` apply a
policy to one route, or to every route of a controller when placed on the
class:

<!-- doctest:fragment context=script -->
```gb
class LoginInput {
    string email;
    string password;
}

@Controller("/auth")
class AuthController {
    @Post("/login")
    @RateLimit(5, 10)
    func login(@Body LoginInput input): dict<string, any> {
        return {"ok": true};
    }
}

@Controller("/admin")
@Cors({"origin": "https://admin.example.com"})
class AdminController {
    @Get("/users")
    func users(): list<string> {
        return [];
    }
}
```

- A method decorator replaces the class one for that route.
- A class `@RateLimit` gives each route its own bucket at that rate.
- `@Cors` takes the same options as `gebweb.cors`; each `@Cors` route also
  answers its preflight unless the app declares an `@Options` route for it.
- Class decorators are not inherited; a subclass repeats them.
- They add to app-wide middleware: a global `rateLimit` of 100 per second and
  `@RateLimit(5, 10)` on `/login` both apply.

## Middleware, events and authentication

Middleware shapes requests and responses; events observe the request
lifecycle (`request.start`, `request.response`, `request.error`,
`request.finish`) without changing them. Plugins can register both during
`install`; see [Events](19-events.md#framework-lifecycle-events) and
[Plugins](16-plugins.md).

Middleware that needs the user can call `gebweb.authenticate(app, request)`.
The decision is shared with the route's gates and policies, so authentication
runs once per request. The result alone enforces nothing: answer with a
response, or rely on route gates, to require a user.

## Reference

| API | Purpose |
|-----|---------|
| `gebweb.before(app, fn)` | Request phase: `fn(gebweb.Request): ?http.Response`. |
| `gebweb.use(app, fn)` / `gebweb.after(app, fn)` | Response phase: `fn(gebweb.Request, http.Response): http.Response`. |
| `gebweb.onRequestEnd(app, fn)` | Finalizer after every request: `fn(dict<string, any> request)`. |
| `gebweb.useAdmissionControl(app, opts)` | Concurrency limit: `maxConcurrent`, `onOverload`. |
| `gebweb.useRequestContext(app)` / `gebweb.requestLogger(req)` | Correlation ids. |
| `gebweb.useEtag(app, opts)` / `gebweb.useServerTiming(app)` | ETags; timing header. |
| `gebweb.measureTiming(request, label, fn)` / `gebweb.recordTiming(request, label, ms)` | Server timing entries; pass `req.toDict()`. |
| Factories | `cors`, `securityHeaders`, `requestId`, `requestLog`, `compress`, `etag`, `rateLimit`, `redisRateLimit`, `abuseGuard`, `waf`, on `gebweb` and `gebweb.middleware`. |
| Decorators | `@RateLimit(perSecond, burst)`, `@Cors(opts)`, `@MaxBody(bytes)`. |

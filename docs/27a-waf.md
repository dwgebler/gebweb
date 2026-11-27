# Web application firewall

<!-- doctest:strict -->

`gebweb.waf(opts)` creates before-middleware that checks request constraints
and content signatures. A match normally returns 403 Problem Details before
the controller runs. Use it alongside authentication, authorization,
parameterized SQL, output escaping and [browser security headers](27-security.md).
Signatures catch known patterns; they cannot establish that input is safe.

## Start with observable rules

This application installs security headers before the WAF, logs matched rule
names and verifies that ordinary requests still reach the controller:

<!-- doctest:file waf_app.gb -->
```gb
import gebweb;

@Controller("/search")
class WafSearch {
    @Get("")
    func search(string q): dict<string, any> {
        return {"query": q};
    }
}

export func build(dict<string, any> opts): gebweb.GebwebApp {
    let app = gebweb.app();
    gebweb.useSecurity(app);
    gebweb.before(app, gebweb.waf(opts));
    return app;
}
```

<!-- doctest:run -->
```gb
import gebweb;
import io;
import waf_app;

let app = waf_app.build({
    "onBlock": func(any request, dict<string, any> hit): void {
        io.println(hit["rule"]);
    },
    "logger": func(string line): void {},
});
let client = gebweb.TestClient(app);
io.println(client.get("/search?q=garden+chairs").status);
let blocked = client.get("/search?q=1+OR+1%3D1");
io.println(blocked.status);
io.println(blocked.headers["Content-Type"]);
io.println((blocked.json() as dict<string, any>)["detail"]);
io.println(blocked.headers["X-Content-Type-Options"]);
```

```text
200
sqli
403
application/problem+json
forbidden
nosniff
```

For deployment, call the setup from your normal `build(conn)` and serve through
[the standard entry point](01-getting-started.md). Construct middleware once
at setup, not inside a handler: scanners and ban state belong to that instance.

## Options

| Option | Default | Meaning |
|--------|---------|---------|
| `rules` | `["sqli", "xss", "rce", "traversal"]` | Active signature sets; an empty list disables signatures. Use these exact names. |
| `mode` | `"block"` | `"log"` reports matches and lets the request continue; use `"block"` to enforce. |
| `allowIps` | `[]` | IPv4/IPv6 addresses or CIDRs that bypass all WAF checks, including bans. |
| `denyIps` | `[]` | Addresses or CIDRs that produce a match unless allowlisted. |
| `allowedMethods` | `[]` | Empty allows all methods; otherwise only listed methods pass. Names are uppercased. |
| `blockedHeaders` | `[]` | Match header presence, case-insensitively. |
| `blockUserAgents` | `[]` | Case-insensitive substrings of `User-Agent` to match. |
| `maxBodyBytes` | `0` | Optional body byte limit; zero disables it. Exceeding it is a WAF match, normally 403. |
| `exclusions` | `[]` | Scoped signature exemptions; see below. |
| `message` | `"forbidden"` | The 403 Problem Details `detail`. |
| `logger` | Print to stdout | `func(string line): void` receiving a formatted match or malformed-IP message. |
| `onBlock` | None | `func(any request, dict<string, any> hit): void`, called on a match in both modes. |
| `ban` | None | `{"threshold": n, "banSeconds": s}`; both keys are required. |
| `blocklist` | None | Optional shared `net.IpSet` to receive escalated bans. |
| `keyFn` | `request.clientIp()` | Custom identity used by IP checks and bans; defaults group IPv6 bans by /64. |

Allowlisting is a bypass, not a requirement that every client appear in the
list. Invalid IP/CIDR entries are logged during construction and ignored.
Provide valid, positive ban settings and use the documented option/rule names.

## What gets inspected

Checks stop at the first match, in this order:

1. An allowlisted identity bypasses the middleware.
2. An existing local ban rejects immediately.
3. Deny IP, allowed method, blocked header, body size, then user-agent checks.
4. Signatures against query text, body text, then string-valued headers.

| Set | Example patterns |
|-----|------------------|
| `sqli` | `UNION SELECT`, `OR 1=1`, `DROP TABLE` |
| `xss` | `<script`, `javascript:`, selected event-handler attributes |
| `rce` | Shell command separators, command substitution, `/bin/sh` |
| `traversal` | `../`, encoded dot pairs, `/etc/passwd`, `file://` |

The query string and URL-encoded form body are checked raw and after up to
three URL-decoding steps. Other text bodies and header values are scanned
without that URL decoding. With field exclusions, JSON is parsed and walked;
without them its text representation is scanned. Raw `bytes` bodies count
toward the size limit but are not content-signature scanned. This is not a
multipart/file-upload parser, HTML sanitizer, SQL parser or recursive decoder
for every possible encoding. The request path itself is not signature-scanned;
use the [abuse guard](09-middleware.md#abuse-guard) for known probe paths.

Use application/server body caps to protect resource consumption. WAF checking
happens after a request is available. For dictionary/list bodies, its size
check uses the JSON serialization supplied to the scanner, not an original
wire representation with whitespace preserved.

## Trial rules and collect diagnostics

Start a new policy in log mode, observe legitimate traffic, then enforce it:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import store;
import waf_app;

let hits = store.Store();
let app = waf_app.build({
    "mode": "log",
    "logger": func(string line): void {
        io.println(line.startsWith("[waf] rule="));
    },
    "onBlock": func(any request, dict<string, any> hit): void {
        hits.set("lastRule", hit["rule"]);
    },
});
let response = gebweb.TestClient(app).get("/search?q=1+OR+1%3D1");
io.println(response.status);
io.println(hits.get("lastRule"));
```

```text
true
200
sqli
```

The default log line includes `rule`, `target`, client identity, method and path;
it does not print the matched value. To send it through an application logger,
pass a callback that calls your configured logger's method. WAF logging is not
automatically routed through `gebweb.logger(app)`.

`hit` contains `rule`, `target` and `value`. Signature values are truncated to
200 characters, but can still contain credentials or personal data. Record
only what your diagnostic policy allows. Hooks run synchronously and errors
propagate, so keep them bounded and reliable. `onBlock` also runs in log mode;
its name does not mean a rejection happened. Already-banned requests return
403 before new-match hooks/logging.

## Narrow exclusions for legitimate content

A support form may legitimately contain code examples. Exempt only its text
field from XSS signatures while retaining SQL/command/traversal checks and all
request constraints:

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/support")
class SupportMessage {
    @Post("")
    func submit(): dict<string, any> {
        return {"accepted": true};
    }
}

let app = gebweb.app();
gebweb.before(
    app,
    gebweb.waf({
        "logger": func(string line): void {},
        "exclusions": [{"paths": ["/support"], "fields": ["message"], "rules": ["xss"]}],
    })
);
let client = gebweb.TestClient(app);
io.println(client.request("POST", "/support", {"message": "<script>example</script>"}, {}).status);
io.println(client.request("POST", "/support", {"subject": "<script>example</script>"}, {}).status);
io.println(client.request("POST", "/support", {"message": "1 OR 1=1"}, {}).status);
```

```text
200
403
403
```

| Exclusion key | Meaning |
|---------------|---------|
| `paths` | Path prefixes on slash boundaries: `/support` covers itself and `/support/...`, not `/supporting`. Omitted applies everywhere. |
| `fields` | Exact query/form/JSON field names. JSON keys match at any depth and exempt the value subtree. Field names themselves remain scanned. |
| `headers` | Case-insensitive header names whose values are exempt. |
| `rules` | Named sets to omit; omitted or empty means all active sets. |

Every value is a list of strings. An entry needs paths, fields or headers.
Unknown exclusion keys, unknown excluded rule names and invalid entry shapes
raise `ValueError` during construction. Multiple matching entries combine
their exemptions. A paths-only entry exempts all signatures on those paths;
it does not exempt IP rules, bans or other request constraints.

Field exemptions apply to query/form pairs and parsed JSON, not arbitrary raw
text or multipart file contents. Exempting a field does not make it safe to
render as HTML or execute. Continue validating and escaping it at its use site.

## Client identity and bans

Use the engine-resolved client IP. Behind a reverse proxy, configure only the
actual proxy addresses in `server.trustedProxies`; see
[Deployment](37-deployment.md#options-and-precedence). Do not trust an arbitrary
caller-supplied header as the identity. A custom `keyFn` changes the value used
by IP lists as well as ban keys; non-IP account identifiers cannot match CIDRs.

This in-process test uses the TestClient's loopback address and no sleeps:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import waf_app;

let app = waf_app.build({
    "ban": {"threshold": 2, "banSeconds": 60},
    "logger": func(string line): void {},
});
let client = gebweb.TestClient(app);
io.println(client.get("/search?q=1+OR+1%3D1").status);
io.println(client.get("/search?q=1+OR+1%3D1").status);
io.println(client.get("/search?q=ordinary").status);
```

```text
403
403
403
```

Each matching request in block mode adds one offence; reaching the threshold
starts the ban. There is no configurable offence time window. State belongs
to this middleware instance, is not shared across processes and disappears on
restart. IPv6 identities use a /64 ban key by default; a custom `keyFn` uses its
returned key directly. Log mode does not accumulate offences.

For connection-level rejection, share a blocklist with the server:

<!-- doctest:fragment context=app -->
```gb
let blocklist = net.ipSet();
gebweb.before(
    app,
    gebweb.waf({
        "ban": {"threshold": 3, "banSeconds": 900},
        "blocklist": blocklist,
    })
);
gebweb.cli(app, {"name": "support", "server": {"ipBlocklist": blocklist}});
```

Keys that are not valid IPs/CIDRs cannot enter that blocklist. The server's
blocklist operates before WAF middleware: a WAF allowlist cannot override an
address already rejected by the server. Use a new deployment/configuration to
trial changes; there is no public WAF reset or distributed-ban API.

## Assets and middleware order

Register browser security setup before WAF before-middleware so a rejected
response can still carry its nonce policy. Register gates deliberately: an
earlier before-middleware response prevents later WAF checks.

[Assets](26-assets.md#middleware) run these app middleware chains by default.
`middleware: false` bypasses them for asset URLs. The abuse guard independently
checks probe paths; install it too when needed. Review allowlists, exclusions
and proxy trust as part of deployment rather than treating the WAF as the only
access boundary.

## Reference

`gebweb.waf(opts)` wraps `gebweb.waf.waf(opts)` and returns a callable for
`gebweb.before`. The `gebweb.waf` module also exposes `ipInCidr`,
`ipMatchesAny` and `networkKey`. For the wider security setup, start with
[Security](27-security.md).

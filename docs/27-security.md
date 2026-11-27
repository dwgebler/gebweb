# Security

<!-- doctest:strict -->

Security setup combines independent controls. Browser headers constrain how a
page is used, authentication identifies a caller, authorization controls access,
and request limits bound resource use. A [WAF](27a-waf.md) adds signature and
request filtering; it does not replace those controls or input validation.

| Concern | Guide |
|---------|-------|
| Authentication and route authorization | [Authentication](40-authentication.md) |
| Cross-site form submissions | [CSRF](23-csrf.md) |
| Session cookies and form data | [Flash and sessions](24-flash.md), [Forms](25-forms.md) |
| Payload filtering, IP rules and bans | [Web application firewall](27a-waf.md) |
| Rate limits, probe blocking and admission | [Middleware](09-middleware.md) |
| Static files and untrusted uploads | [Assets](26-assets.md), [Storage](21-storage.md) |
| TLS, trusted proxies and server limits | [Deployment](37-deployment.md) |

## Defaults

`gebweb.useSecurity(app)` installs response headers and, when requested by CSP,
a request nonce and its `cspNonce` view-context value. Configure it before
before-middleware gates that might reject a request, so the nonce is available
even when a gate answers early.

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app([], {"docs": false});
gebweb.useSecurity(app);
let response = gebweb.TestClient(app).get("/missing");
io.println(response.status);
io.println(response.headers["X-Content-Type-Options"]);
io.println(response.headers["X-Frame-Options"]);
io.println(response.headers["Referrer-Policy"]);
io.println(response.headers.contains("Content-Security-Policy"));
io.println(response.headers.contains("Strict-Transport-Security"));
```

```text
404
nosniff
DENY
strict-origin-when-cross-origin
false
false
```

| Option | Default | Meaning |
|--------|---------|---------|
| `contentTypeOptions` | `true` | Emit `X-Content-Type-Options: nosniff`; false suppresses it. |
| `frameOptions` | `"DENY"` | `X-Frame-Options` value; an empty string suppresses it. |
| `referrerPolicy` | `"strict-origin-when-cross-origin"` | `Referrer-Policy` value; an empty string suppresses it. |
| `csp` | Omitted | Directive dictionary; omitted means no CSP header. |
| `cspReportOnly` | `false` | Use the report-only CSP header instead of enforcement. |
| `hsts` | Omitted | HSTS settings; omitted means no HSTS header. |

Suppressing a header stops this middleware adding it; it does not remove a
header already supplied elsewhere. For example, a same-origin framing policy:

<!-- doctest:fragment context=app -->
```gb
gebweb.useSecurity(app, {"frameOptions": "SAMEORIGIN", "referrerPolicy": "no-referrer"});
```

## Request body limits

The default application limit is 10 * 1024 * 1024 bytes. A positive
`useMaxBodyBytes` value is also passed to the HTTP server unless server options
override it. The server limit protects buffering at the transport boundary;
route-level checks run after the request is available to the framework.

This example accepts a small upload and rejects one above a tighter route cap:

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/upload")
class SmallUpload {
    @Post("")
    @MaxBody(4)
    func upload(): dict<string, any> {
        return {"accepted": true};
    }
}

let app = gebweb.app();
gebweb.useMaxBodyBytes(app, 1024);
let client = gebweb.TestClient(app);
io.println(client.request("POST", "/upload", "four", {}).status);
io.println(client.request("POST", "/upload", "larger", {}).status);
```

```text
200
413
```

`useMaxBodyBytes(app, 0)` disables the app cap. A route cap can tighten a
positive app/server cap; it cannot make an already rejected request acceptable.
The WAF's optional body limit is separate and produces a WAF match/403; use
server limits to bound the bytes accepted from a connection. In-process
app/route cap checks inspect string bodies. A raw `bytes` body passed directly
to `TestClient` does not exercise those checks; live HTTP supplies a string.
Use string test bodies and real HTTP tests to verify those caps.

## Content Security Policy

CSP keys use camelCase, rendered as kebab-case: `defaultSrc` becomes
`default-src`. Values are lists of strings. The exact string `"'nonce'"`
expands to a fresh `"'nonce-<value>'"` source expression on each request.

Create `templates/secure.html`:

<!-- doctest:file templates/secure.html -->
```html
<h1>Secure page</h1>
<script nonce="{{ cspNonce }}">document.title = 'Ready';</script>
```

This program checks that the template nonce matches its response header and
that another request gets a different nonce, without printing random values:

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/secure")
class SecurePage extends gebweb.Controller {
    @Get("")
    func page(gebweb.Request request): gebweb.Response {
        return this.view(request, "secure.html", {});
    }
}

let app = gebweb.app();
gebweb.useViews(app, "templates");
gebweb.useSecurity(app, {
    "csp": {
        "defaultSrc": ["'self'"],
        "scriptSrc": ["'self'", "'nonce'"],
        "objectSrc": ["'none'"],
        "baseUri": ["'self'"],
    },
});
let client = gebweb.TestClient(app);
let first = client.get("/secure");
let second = client.get("/secure");
string policy = first.headers["Content-Security-Policy"] as string;
string nonce = first.text().split('nonce="')[1].split('"')[0];
io.println(!nonce.isEmpty());
io.println(policy.contains("'nonce-" + nonce + "'"));
io.println(policy != second.headers["Content-Security-Policy"]);
```

```text
true
true
true
```

Keep template escaping enabled. A nonce authorizes the tagged script; it does
not sanitize data interpolated into JavaScript. Prefer external scripts and
safe data serialization. Do not mix independently cached nonce-bearing HTML
with a freshly generated CSP header: the nonce in the body and header must
travel together. A browser reusing a complete cached response reuses that
response's policy too; it does not compare the nonce with another request.

Use `"'none'"` for a directive that must deny all sources. This formatter omits
empty directive lists; they do not emit a bare directive such as `sandbox`.

### Using the nonce in templates

The `cspNonce` context value is available to templates rendered through the
application view engine. It is empty when the configured CSP does not request
a nonce. Put `nonce="{{ cspNonce }}"` on inline scripts or styles authorized
by the corresponding directive.

### Report-only mode

Trial a policy before enforcing it:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app([], {"docs": false});
gebweb.useSecurity(app, {
    "csp": {"defaultSrc": ["'self'"], "reportUri": ["/csp-report"]},
    "cspReportOnly": true,
});
let response = gebweb.TestClient(app).get("/missing");
io.println(response.headers.contains("Content-Security-Policy-Report-Only"));
io.println(response.headers.contains("Content-Security-Policy"));
```

```text
true
false
```

Report-only does not enforce the policy. `reportUri` only names a destination;
it does not install a reporting endpoint or persist browser reports. Implement
that endpoint or use a reporting service.

## HSTS

Enable HSTS only for a deployed HTTPS host. Gebweb emits the configured header
without checking the request scheme; TLS may terminate at a trusted proxy.

<!-- doctest:fragment context=app -->
```gb
gebweb.useSecurity(app, {"hsts": {"maxAge": 31536000, "includeSubdomains": true, "preload": false}});
```

| HSTS key | Default when `hsts` is present | Meaning |
|----------|--------------------------------|---------|
| `maxAge` | `31536000` | Browser policy lifetime in seconds. |
| `includeSubdomains` | `false` | Include `includeSubDomains`. Enable only when every subdomain supports HTTPS. |
| `preload` | `false` | Include `preload`; this does not submit the domain to a browser preload list. |

Browsers remember HSTS. Plan HTTPS coverage and recovery before enabling it,
especially for subdomains and preload deployments.

## Asset responses

Assets run application middleware by default, including security headers and
WAF checks. `middleware: false` on `useStaticAssets` bypasses those chains;
lifecycle notifications still run. Serve untrusted uploads from a separate
origin with suitable content types and disposition. CSP on an HTML page does
not make arbitrary same-origin uploaded content safe.

## Overriding the defaults

Register `useSecurity` once during setup. Configure its options rather than
stacking multiple installations. After-middleware runs in reverse registration
order; if another middleware also writes a header, ordering determines which
value survives.

## Lower-level middleware factory

`gebweb.securityHeaders(opts)` is a separate lightweight middleware factory for
the basic headers. It does not mint CSP nonces or publish view context. Prefer
`useSecurity` when configuring CSP or HSTS.

## Reference

Use `gebweb.useSecurity(app, opts = {})` for browser headers and
`gebweb.useMaxBodyBytes(app, maxBytes)` for the app body cap. Use the dedicated
[WAF chapter](27a-waf.md) for payload rules and IP controls; authentication,
CSRF and cookie policies remain separate setup steps.

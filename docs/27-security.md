# Security headers

`gebweb.useSecurity(app, opts)` mounts the browser-security response
headers most apps want: a conservative default set plus opt-in CSP
(with per-request nonce expansion) and opt-in HSTS.

A single call wires three things:

1. A response-phase middleware that writes the headers.
2. A request-phase middleware that mints a fresh CSP nonce.
3. A view-context injector that publishes the nonce as `cspNonce`
   so templates can stamp it onto inline `<script>` / `<style>` tags.

## Defaults

The defaults are conservative for an HTML app:

```gb
import gebweb;

let app = gebweb.app([HomeController()]);
gebweb.useSecurity(app, {});
```

Every response gets:

- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: strict-origin-when-cross-origin`

CSP and HSTS stay off unless asked. That keeps dev environments
ergonomic; you opt in per environment.

## Content Security Policy

CSP is described as a dict of camelCase directive names mapped to
lists of source expressions. The framework lowers camelCase to
kebab-case at render time (`defaultSrc` becomes `default-src`).

```gb
gebweb.useSecurity(app, {
    "csp": {
        "defaultSrc": ["'self'"],
        "scriptSrc":  ["'self'", "'nonce'"],
        "styleSrc":   ["'self'", "'unsafe-inline'"],
        "imgSrc":     ["'self'", "data:"],
        "connectSrc": ["'self'"]
    }
});
```

The literal string `'nonce'` inside any directive list expands to
`'nonce-<value>'` with a fresh value minted per request. The
unexpanded form is what you write; the framework substitutes the
real value before the header is written.

### Using the nonce in templates

The current request's nonce is auto-injected into every rendered
template as `cspNonce`:

```twig
<script nonce="{{ cspNonce }}">
  console.log("hello from inline script");
</script>
```

Each request gets a different nonce, so cached HTML rendered with
yesterday's nonce won't execute today. CSP plus per-request nonce
is the strongest defense against reflected and stored XSS short of
banning inline script entirely.

### Report-only mode

While tightening a CSP, switch to report-only so violations log but
don't break the page:

```gb
gebweb.useSecurity(app, {
    "csp": { "defaultSrc": ["'self'"] },
    "cspReportOnly": true
});
```

The header name switches to `Content-Security-Policy-Report-Only`.
Add a `report-uri` or `report-to` directive to receive violation
reports.

## HSTS

HSTS (Strict-Transport-Security) tells browsers to never load this
host over plain HTTP for the configured duration. It's powerful and
**sticky** - once a browser remembers the header, it ignores
unencrypted requests to your domain for the full `max-age`. Don't
enable it in development.

```gb
gebweb.useSecurity(app, {
    "hsts": {
        "maxAge": 31536000,           // 1 year
        "includeSubdomains": true,
        "preload": false
    }
});
```

Set `preload: true` only if you intend to submit the domain to the
HSTS preload list and have audited every subdomain.

## Overriding the defaults

Suppress any default header by passing an empty string for its key,
or set a custom value:

```gb
gebweb.useSecurity(app, {
    "frameOptions":       "SAMEORIGIN",                  // allow framing from same origin
    "referrerPolicy":     "no-referrer",                 // tighter
    "contentTypeOptions": false                          // suppress nosniff
});
```

## Lower-level middleware factory

`gebweb.securityHeaders({})` is the underlying middleware factory and
can be used directly when you only want the three default headers
without CSP / HSTS / nonce wiring. Prefer `gebweb.useSecurity` for
new code: it owns the per-request nonce, publishes `cspNonce` to
the view context, and centralises the configuration. The factory
is exposed for advanced setups that want to compose the middleware
into a custom pipeline.

## Asset responses

Static assets served via `gebweb.useStaticAssets` short-circuit the
router before middleware runs, so they receive only their own
`Content-Type` and `Cache-Control` headers - not the security
headers. That is intentional: assets are the resources CSP guards,
and applying CSP to a JavaScript file's response wouldn't do
anything meaningful. If you serve untrusted user uploads, place
them behind a separate route under a different origin instead of
relying on response headers.

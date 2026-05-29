# CSRF Protection

`gebweb.useCsrf(app, opts)` activates Cross-Site Request Forgery
protection for every state-changing request. Once enabled, every
POST / PUT / PATCH / DELETE handler requires a valid token unless
the route is declared `@CsrfExempt`. CSRF is opt-in: gebweb apps
that never call `useCsrf` keep the current pass-through behaviour.

```gb
import gebweb;

let app = gebweb.app([HomeController(), AdminController()]);
gebweb.useCsrf(app, {"secret": env.get("CSRF_SECRET")});
```

## How the token flows

1. On every safe request (GET / HEAD / OPTIONS) the framework
   ensures a `geb_csrf` cookie is set on the response. The cookie
   value is a JWT signed with `opts.secret` carrying a random
   per-session payload.
2. The current token value is auto-injected into view contexts
   under the key `csrf`, so templates can echo it back to the form
   without manual threading.
3. On any state-changing request the framework reads the token
   from the cookie and compares it (constant-time) against the
   value submitted in either the `X-CSRF-Token` header or the
   `_csrf` form field. Mismatch returns `403 CSRF token invalid`.

## Server-rendered forms

The canonical pattern places a hidden field in every form:

```html
<form method="post" action="/posts">
    <input type="hidden" name="_csrf" value="{{ csrf }}">
    <textarea name="body"></textarea>
    <button>Publish</button>
</form>
```

`{{ csrf }}` is the auto-injected token. No helper call needed.

## Ajax / fetch

For non-form submissions, send the token in the `X-CSRF-Token`
header:

```html
<script>
const token = "{{ csrf }}";
fetch("/posts", {
    method: "POST",
    headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": token
    },
    body: JSON.stringify({title: "..."})
});
</script>
```

## Exempting routes

Webhooks, callbacks signed by an external provider, and API
endpoints authenticated by a different mechanism (API keys, OAuth
bearer tokens) typically should not require a CSRF token. Annotate
either the handler or the controller class with `@CsrfExempt`:

```gb
class StripeController {
    @Post("/webhooks/stripe")
    @CsrfExempt
    func handleEvent(dict<string, any> req): dict<string, any> {
        /* verify Stripe-Signature header separately */
        return {"status": 200};
    }
}

@CsrfExempt
class ApiController {
    @Post("/api/v1/users")
    func create(...): dict<string, any> { ... }
}
```

## Configuration options

```gb
gebweb.useCsrf(app, {
    "secret":        "...",         /* required: token-signing secret */
    "cookieName":    "geb_csrf",    /* override the cookie name */
    "cookieOptions": {
        "SameSite":  "Lax",         /* "Strict" / "Lax" / "None" */
        "Secure":    true,          /* HTTPS-only */
        "HttpOnly":  true,          /* not readable by JS */
        "MaxAge":    86400,         /* seconds */
        "Domain":    "example.com"
    }
});
```

Defaults: cookie name `geb_csrf`, `SameSite=Lax`, `HttpOnly=true`,
no Secure flag (set it explicitly for HTTPS deployments), no
Max-Age (session cookie).

## How `@CsrfExempt` is wired

The decorator check happens at route-binding time inside
`gebweb.app(controllers)`. Routes with the decorator never get the
CSRF wrapper; routes without it always do, even before `useCsrf`
is called. The wrapper is a no-op when `app.csrfConfig` is null,
so the runtime cost for un-enabled apps is one field read per
request.

This means `useCsrf` can be called either before or after route
registration with the same effect.

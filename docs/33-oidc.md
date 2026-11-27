# OIDC and OAuth2 sign-in

<!-- doctest:strict -->

`gebweb.useOidc` mounts authorization-code callbacks with PKCE. The callback
exchanges a code, resolves provider claims, calls your user resolver and saves
its dictionary through the configured session store. Register session handling
before OIDC; OIDC sign-in and session-backed route authentication are separate
pieces of setup.

## Wiring

Put this application in `src/oidc_app.gb`. It owns the app reference used by
the login controller, applies a session cookie policy on every save, and
allows only the configured public host:

<!-- doctest:file oidc_app.gb -->
```gb
import gebweb;
import log;
import web.session as session;

class BrowserSessions {
    session.FileSessionStore inner;
    string cookieName = "geb_sid";
    bool secure;

    func BrowserSessions(string directory, bool secure) {
        this.inner = session.fileSessionStore(directory, 3600);
        this.secure = secure;
    }

    func load(dict<string, any> request): dict<string, any> {
        return this.inner.load(request);
    }

    func save(dict<string, any> response, dict<string, any> data, dict<string, any> options): dict<string, any> {
        let policy = options.copy();
        policy["httpOnly"] = true;
        policy["sameSite"] = "Lax";
        policy["secure"] = this.secure;
        return this.inner.save(response, data, policy);
    }

    func clear(dict<string, any> response, dict<string, any> request): dict<string, any> {
        return this.inner.clear(response, request);
    }
}

class SignedInUser {
    string id;

    func SignedInUser(string id) {
        this.id = id;
    }
}

@Controller("")
class AuthController extends gebweb.Controller {
    gebweb.GebwebApp app;

    func AuthController(gebweb.GebwebApp app) {
        parent();
        this.app = app;
    }

    @Get("/login/{provider}")
    func login(string provider, gebweb.Request request): dict<string, any> {
        let names = gebweb.getParameter(this.app, "auth.providers") as list<string>;
        if (!names.contains(provider)) {
            this.notFound("Unknown sign-in provider");
        }
        let flow = gebweb.oidcLoginUrl(this.app, provider, request.toDict());
        string cookie = "geb_oidc="
            + (flow["stateCookie"] as string)
            + "; Path=/; HttpOnly; SameSite=Lax; Max-Age=600";
        if (gebweb.getParameter(this.app, "auth.secure") as bool) {
            cookie = cookie + "; Secure";
        }
        return {
            "status": 302,
            "headers": {"Location": flow["url"], "Set-Cookie": cookie},
            "body": "",
        };
    }

    @Get("/account")
    @Auth
    func account(SignedInUser user): dict<string, any> {
        return {"id": user.id};
    }
}

export func build(string directory, string stateSecret, list<gebweb.OidcProvider> providers, bool secureCookies = true): gebweb.GebwebApp {
    let app = gebweb.app();
    gebweb.useLogger(app, log.stderr());
    gebweb.parameter(app, "auth.secure", secureCookies);
    gebweb.parameter(
        app,
        "auth.providers",
        providers.map(func(gebweb.OidcProvider provider): string { return provider.name; })
    );
    gebweb.before(app, func(gebweb.Request request): any {
        string host = request.host();
        if (host.isEmpty()) {
            host = request.header("Host") ?? "";
        }
        return host == "app.example.test" ? null : gebweb.html("Unexpected host", 400);
    });
    let sessions = BrowserSessions(directory, secureCookies);
    gebweb.useSessionAuth(app, sessions, SignedInUser, func(dict<string, any> data): ?SignedInUser {
        return data.contains("id") ? SignedInUser(data["id"] as string) : null;
    });
    gebweb.useOidc(app, {
        "providers": providers,
        "stateSecret": stateSecret,
        "successRedirect": "/account",
        "failureRedirect": "/login",
        "userResolver": func(dict<string, any> claims, gebweb.OidcProvider provider): dict<string, any> {
            return {"id": provider.name + ":" + (claims["sub"] as string)};
        },
    });
    return app;
}
```

Replace the example host with your application-owned public host policy. The
provider name/subject combination is an example identity namespace: in a real
application, look up a local account by a stable provider/issuer and subject.
Do not link accounts or grant roles solely from a display name or an email
address whose verification you have not established.

The resolver must return a session-data dictionary or null to reject the user.
`useSessionAuth` reconstructs the typed local user on later requests. OIDC does
not register an authenticator automatically. Its callbacks capture the session
store present at `useOidc` time; configure the store first. Without one, a
successful callback redirects but does not persist sign-in.

## Starting the flow

This complete program checks the login redirect without contacting Google.
The client ID/secret are placeholders; URL generation does not validate them:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import oidc_app;

let directory = io.tempDir("manual-oidc-");
try {
    let app = oidc_app.build(directory, "example-only-state-secret", [
        gebweb.oidcGoogle("example-client", "example-secret"),
    ]);
    let client = gebweb.TestClient(app);
    let response = client.request("GET", "/login/google", null, {"Host": "app.example.test"});
    string location = response.headers["Location"] as string;
    string cookie = response.headers["Set-Cookie"] as string;
    io.println(response.status);
    io.println(location.startsWith("https://accounts.google.com/"));
    io.println(location.contains("response_type=code"));
    io.println(location.contains("code_challenge_method=S256"));
    io.println(location.contains("%2Fauth%2Fgoogle%2Fcallback"));
    io.println(
        cookie.contains("; HttpOnly") && cookie.contains("; Secure") && cookie.contains("; SameSite=Lax")
    );
    io.println(client.request("GET", "/account", null, {"Host": "app.example.test"}).status);
} finally {
    io.remove(directory);
}
```

```text
302
true
true
true
true
true
401
```

`oidcLoginUrl(app, name, requestDict)` returns `{url, stateCookie}`. Send the
URL as `Location` and set the returned value as the `geb_oidc` cookie; the
helper does not set that cookie itself. Register the exact callback URI with
the identity provider: by default `/auth/<provider.name>/callback` at your
public origin.

Login and code exchange use the engine-resolved request scheme/host. For TLS
termination, configure only your actual reverse proxies in `trustedProxies`;
raw forwarding headers from other callers do not set the origin. This does not
authorize arbitrary Host values: validate allowed hosts at your edge/app, as
above. See [Deployment](37-deployment.md#options-and-precedence).

`TestClient` defaults to an HTTP origin and does not enforce browser cookie
rules; the program tests response construction. Use HTTPS and Secure cookies
for a deployed browser flow. Provider integration tests must exercise your
actual public callback URI, session store and proxy setup.

## Options

| `useOidc` option | Default | Meaning |
|------------------|---------|---------|
| `providers` | Required | List of provider values; use distinct stable names. |
| `stateSecret` | Required | Server-owned HMAC secret for state cookies. Load a strong nonempty value from secrets configuration. |
| `userResolver` | Required | `func(dict<string, any> claims, gebweb.OidcProvider provider): ?dict<string, any>`. |
| `callbackPathPrefix` | `"/auth"` | Prefix for mounted GET callback routes. |
| `successRedirect` | `"/"` | Application-controlled post-login location. |
| `failureRedirect` | `"/login"` | Application-controlled failure location; receives an `error` query parameter. |

Create the callback configuration once during setup. Keep login/callback paths
reachable through app middleware; an app-wide authentication requirement must
allow the unauthenticated sign-in flow. Set success/failure destinations in
application configuration rather than accepting arbitrary redirect targets
from request input.

For production, construct providers using configured credentials before calling
the example builder from your normal `build(conn)` function. For example, this
module is checked without reading real secrets or contacting a provider:

<!-- doctest:file live_oidc.gb -->
```gb
import db;
import gebweb;
import oidc_app;
import secrets;

export func build(db.Connection conn): gebweb.GebwebApp {
    return oidc_app.build(
        secrets.requireEnv("APP_SESSION_DIRECTORY"),
        secrets.requireEnv("OIDC_STATE_SECRET"),
        [
            gebweb.oidcGoogle(
                secrets.requireEnv("GOOGLE_CLIENT_ID"),
                secrets.requireEnv("GOOGLE_CLIENT_SECRET")
            ),
        ]
    );
}
```

Use the [standard entry point](01-getting-started.md), persistent
application-owned session storage and appropriate shutdown/resource ownership.
See [Session cookie policy](24-flash.md) and [Authentication](40-authentication.md).

## Failed sign-in

Handled failures clear the state cookie and redirect with a short error code.
These cases require no provider network access:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import oidc_app;

let directory = io.tempDir("manual-oidc-fail-");
try {
    let app = oidc_app.build(directory, "example-only-state-secret", [
        gebweb.oidcGoogle("example-client", "example-secret"),
    ]);
    let client = gebweb.TestClient(app);
    for (query in ["error=access_denied", "", "code=example&state=wrong"]) {
        let response = client.request("GET", "/auth/google/callback?" + query, null, {
            "Host": "app.example.test",
        });
        io.println(response.status);
        io.println(response.headers["Location"]);
        io.println((response.headers["Set-Cookie"] as string).startsWith("geb_oidc=;"));
    }
} finally {
    io.remove(directory);
}
```

```text
302
/login?error=access_denied
true
302
/login?error=invalid_request
true
302
/login?error=invalid_state
true
```

| `error` | Cause |
|---------|-------|
| Provider code / `provider_error` | Provider returned `error`. Only up to 64 lowercase letters/underscores pass through; other values become `provider_error`. |
| `invalid_request` | Missing `code` or `state` query field. |
| `invalid_state` | State cookie missing, malformed, mismatched or incorrectly signed. |
| `token_exchange_failed` | Token endpoint request/response failed. |
| `invalid_id_token` | OIDC token decoding, signature or claim validation failed. |
| `userinfo_failed` | GitHub profile resolution failed, including an invalid profile ID. |
| `user_rejected` | Your user resolver returned null. |

These are 302 redirects, not callback 400 responses. An existing failure-page
query uses `&error=...`. Details are warnings through the framework logger as
`oidc sign-in failed (<provider>, <code>): <detail>` with corresponding fields.
Upstream error bodies may appear in those logs; restrict access and apply your
redaction policy. An exception from the application user resolver propagates
as an application error; it is not converted into `user_rejected`.

## Provider presets

These are the presets shipped with the framework:

| Factory | Configuration |
|---------|---------------|
| `oidcGoogle(clientId, clientSecret, opts = {})` | Google endpoints/issuer/JWKS; default scopes `openid`, `profile`, `email`. |
| `oidcGithub(clientId, clientSecret, opts = {})` | GitHub OAuth2 endpoints; default scopes `read:user`, `user:email`. |
| `oidcGeneric(opts)` | Explicit provider configuration; no automatic discovery-document fetch. |

The Google/GitHub preset options override `scopes`; they do not override all
endpoint fields. Generic provider options are:

| Field | Default / requirement |
|-------|-----------------------|
| `name`, `authEndpoint`, `tokenEndpoint`, `clientId` | Required. Names form callback paths. |
| `clientSecret` | Empty string. |
| `flavor` | `"oidc"`; `"github"` selects the separate profile flow. |
| `issuer` | Empty, but required by OIDC claim validation. |
| `jwksUri` | Empty; configure the provider's trusted key-set URL for signature verification. |
| `scopes` | `["openid", "profile", "email"]` |
| `userinfoEndpoint` | Empty; used by the GitHub flavor, not for generic OIDC profile enrichment. |

Keep endpoints and issuer in trusted application configuration. Use HTTPS for
provider traffic. The facade does not expose a separate per-provider HTTP
timeout/retry policy.

GitHub does not use the ID-token path here. The adapter fetches the configured
profile endpoint with the access token and synthesizes `sub` from the profile
ID as a string, plus `name`, `preferred_username`, `email` and `picture`.
Some profile fields can be empty/null; it does not call a separate emails
endpoint or establish email verification from that synthesized dictionary.

## ID token validation

For the OIDC flavor, `resolveClaims` requires a nonempty ID token. With a
`jwksUri`, it verifies the signature against the configured provider key set
before returning validated claims. Key sets are cached across requests; a
verification failure triggers one refetch attempt for that resolution call.
The Google preset configures its JWKS URL.

Without `jwksUri`, the implementation only decodes the token and validates
claims; it does not verify its signature. Configure signature verification and
trusted HTTPS endpoints for production rather than relying on this fallback.

| Claim | Enforced check |
|-------|----------------|
| `iss` | Nonempty string equal to the configured, nonempty issuer. |
| `sub` | Nonempty string. |
| `aud` | Client ID string, or a list containing it. |
| `azp` | Required for several audiences; when non-null, must equal the client ID. |
| `exp` | Integer Unix seconds strictly later than validation time. |

The lower-level claim validator is useful for deterministic checks, but does
not verify signatures. This example uses a fixed clock value:

<!-- doctest:run -->
```gb
import gebweb;
import gebweb.oidc as oidc;
import io;

let provider = gebweb.oidcGeneric({
    "name": "example",
    "issuer": "https://identity.example.test",
    "authEndpoint": "https://identity.example.test/auth",
    "tokenEndpoint": "https://identity.example.test/token",
    "clientId": "example-client",
});
let claims = {"iss": provider.issuer, "sub": "user-123", "aud": "example-client", "exp": 2000};
oidc.validateIdTokenClaims(provider, claims, 1000);
io.println("claims accepted");
try {
    oidc.validateIdTokenClaims(provider, claims, 2000);
} catch (RuntimeError error) {
    io.println("expired claims rejected");
}
```

```text
claims accepted
expired claims rejected
```

## Limitations

- The state cookie contains the random state and PKCE verifier with an HMAC.
  It is signed, not encrypted. Keep it HttpOnly, Secure and short-lived.
- Cookie `Max-Age` controls normal browser retention. The signed payload has no
  timestamp or server-side one-use record; `verifyState` does not enforce age.
  Clearing the cookie does not invalidate a separately retained copy.
- One `geb_oidc` cookie is shared by these flows. Starting another sign-in can
  replace it and make an earlier callback fail. The flow does not generate or
  validate an OIDC `nonce` claim.
- Successful callbacks save the user resolver's dictionary. Tokens are not
  passed to that resolver or automatically persisted. There is no built-in
  refresh-token, provider logout or account-linking workflow.
- `TestClient` has no browser cookie jar. Carry updated `Set-Cookie` values
  explicitly in integration tests; do not assume a redirect signs in the next
  request without a session cookie.

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.useOidc(app, opts)` | Register providers and callback routes; return the app. |
| `gebweb.oidcLoginUrl(app, name, requestDict)` | Generate `{url, stateCookie}`. |
| `gebweb.oidcGoogle` / `oidcGithub` / `oidcGeneric` | Construct provider values. |
| `gebweb.OidcProvider` | Alias of the provider class. |
| `gebweb.oidc.verifyState(cookie, state, secret)` | Recover verifier on a valid state/signature; empty string otherwise. |
| `gebweb.oidc.exchangeCode(provider, code, redirectUri, verifier)` | Lower-level token exchange. |
| `gebweb.oidc.resolveClaims(provider, tokens)` | Resolve/validate OIDC or GitHub claims. |
| `gebweb.oidc.validateIdTokenClaims(provider, claims, nowSec)` | Claim checks only. |

See [Authentication](40-authentication.md), [CSRF](23-csrf.md),
[Sessions](24-flash.md), [Security](27-security.md) and
[Deployment](37-deployment.md).

# OIDC and OAuth2 sign-in

`useOidc` mounts the authorization-code-with-PKCE flow against one
or more identity providers. Provider presets exist for Google,
GitHub, and any standards-compliant OIDC issuer. On a successful
callback the framework writes the resolved user into the session
store.

## Wiring

```gb
import gebweb;

let store = session.fileSessionStore("/tmp/app-sessions", 86400);

let app = gebweb.app([HomeController()]);
gebweb.useSession(app, store);
gebweb.useOidc(app, {"providers": [gebweb.oidcGoogle(env.read("GOOGLE_CLIENT_ID"), env.read("GOOGLE_CLIENT_SECRET")), gebweb.oidcGithub(env.read("GITHUB_CLIENT_ID"), env.read("GITHUB_CLIENT_SECRET"))], "stateSecret": env.read("OIDC_STATE_SECRET"), "successRedirect": "/dashboard", "failureRedirect": "/login", "userResolver": func(dict<string, any> claims, gebweb.OidcProvider provider): dict<string, any> {
    return {
        "id":    claims["sub"] as string,
        "email": (claims["email"] ?? "") as string,
        "name":  (claims["name"] ?? "") as string,
        "via":   provider.name,
    };
}});
```

The framework mounts `/auth/google/callback` and
`/auth/github/callback` automatically. Override the prefix with
`opts.callbackPathPrefix` (default `"/auth"`).

## Failed sign-in

When sign-in does not complete, the callback clears the state cookie and
redirects to `opts.failureRedirect` (default `"/login"`) with an `error`
query parameter, so the login page can show a message:

| `error` | Cause |
|---------|-------|
| `access_denied` and other provider codes | The provider redirected back with an error, for example when the user declined. Codes other than lowercase letters and `_` arrive as `provider_error`. |
| `invalid_request` | The callback had no `code` or no `state`. |
| `invalid_state` | The state cookie was missing, expired or did not match. |
| `token_exchange_failed` | The token endpoint could not be reached or returned an error. |
| `invalid_id_token` | The ID token failed [validation](#id-token-validation). |
| `userinfo_failed` | GitHub only: the profile request failed. |
| `user_rejected` | Your `userResolver` returned `null`. |

The URL carries only the code. The detail (for example which claim
failed) is written to the server's error output as
`gebweb oidc sign-in failed (<provider>, <code>): <detail>`. If
`failureRedirect` already has a query string, `&error=...` is appended.

An exception thrown by your `userResolver` is not treated as a failed
sign-in: it propagates like any other handler error, so a bug in the
resolver is not hidden behind the login page.

## Starting the flow

Build the login URL on demand and redirect the browser:

```gb
class AuthController {
    @Get("/login/google")
    func login(dict<string, any> request): dict<string, any> {
        let r = gebweb.oidcLoginUrl(request["app"], "google", request);
        return {
            "status": 302,
            "headers": {
                "Location": r["url"],
                "Set-Cookie": "geb_oidc="
                    + r["stateCookie"]
                    + "; Path=/; HttpOnly; Secure; SameSite=Lax; Max-Age=600",
            },
            "body": "",
        };
    }
}
```

`oidcLoginUrl` returns `{url, stateCookie}`. The state cookie
carries the PKCE verifier alongside the random state value with
an HMAC signature; the callback handler verifies it before
exchanging the code.

## Provider presets

```gb
gebweb.oidcGoogle(clientId, clientSecret);
gebweb.oidcGithub(clientId, clientSecret);
gebweb.oidcGeneric({
    "name": "keycloak",
    "issuer": "https://keycloak.example.com/realms/main",
    "authEndpoint":     "https://keycloak.example.com/realms/main/protocol/openid-connect/auth",
    "tokenEndpoint":    "https://keycloak.example.com/realms/main/protocol/openid-connect/token",
    "userinfoEndpoint": "https://keycloak.example.com/realms/main/protocol/openid-connect/userinfo",
    "jwksUri":          "https://keycloak.example.com/realms/main/protocol/openid-connect/certs",
    "clientId":     "myclient",
    "clientSecret": "...",
    "scopes":       ["openid", "profile", "email"],
});
```

Pass `{"scopes": [...]}` as the third argument to the Google /
GitHub presets to override the default scope list.

GitHub is OAuth2 only - it does not issue an ID token. The
framework hits `https://api.github.com/user` with the access
token and synthesises a claims dict (`sub`, `name`,
`preferred_username`, `email`, `picture`). Other providers run
the OIDC path described below.

## ID token validation

Before the user resolver runs, the ID token from the token endpoint
must pass every check below, or the callback fails and no one is
signed in:

- `iss` is present and equals the provider's `issuer`. A provider
  configured without an `issuer` cannot sign anyone in.
- `sub` is present and not empty.
- `aud` is the provider's `clientId`, or a list that contains it. A
  list with more than one audience also needs an `azp` claim, and
  any `azp` must equal the `clientId`.
- `exp` is a number and lies in the future. A missing or zero `exp`
  is rejected.

### Signature verification

When the provider has a `jwksUri`, the token's signature is verified
against the keys published there before the claims are checked. The
Google preset sets Google's key URL; for a generic provider, set
`jwksUri` to the `jwks_uri` value from the provider's
`/.well-known/openid-configuration` document. The keys are fetched
once and cached; when a token does not verify against the cached set
(for example after the provider rotates its keys) they are fetched
again once before the token is rejected. A token signed with any
other key, an HMAC token or an unsigned token is rejected.

Without a `jwksUri`, the token is not signature-checked. It is still
safe to accept in this flow only because Gebweb receives it directly
from the provider's `tokenEndpoint` over TLS, so keep that endpoint
on `https://` and set `jwksUri` whenever the provider publishes one.

## Limitations

- The state cookie carries the PKCE verifier. If a request
  arrives on the callback path without the cookie (different
  browser, expired cookie), the callback returns 400. Keep the
  cookie's `Max-Age` short - 5 to 10 minutes is plenty.
- The user resolver runs once per callback. There's no built-in
  refresh-token flow; if you need long-lived sessions, persist
  the refresh token in your user record and renew on demand.

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.useOidc(app, opts)` | Mount providers and the callback routes. |
| `gebweb.oidcLoginUrl(app, providerName, request)` | Build `{url, stateCookie}` for redirect. |
| `gebweb.oidcGoogle(clientId, clientSecret, opts)` | Google provider preset. |
| `gebweb.oidcGithub(clientId, clientSecret, opts)` | GitHub preset (OAuth2 with synthesised claims). |
| `gebweb.oidcGeneric(opts)` | Configurable OIDC preset. |
| `gebweb.OidcProvider` | Provider value object. |
| `gebweb.oidc.verifyState(cookie, state, secret)` | Cookie/state verification helper. |
| `gebweb.oidc.exchangeCode(provider, code, redirectUri, verifier)` | Token-endpoint exchange. |
| `gebweb.oidc.resolveClaims(provider, tokens)` | Verified, validated claims for both flavours. |
| `gebweb.oidc.validateIdTokenClaims(provider, claims, nowSec)` | The ID token claim checks on their own; throws on the first failure. |

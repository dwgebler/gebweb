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
gebweb.useOidc(app, {
    "providers": [
        gebweb.oidcGoogle(env.read("GOOGLE_CLIENT_ID"), env.read("GOOGLE_CLIENT_SECRET")),
        gebweb.oidcGithub(env.read("GITHUB_CLIENT_ID"), env.read("GITHUB_CLIENT_SECRET"))
    ],
    "stateSecret": env.read("OIDC_STATE_SECRET"),
    "successRedirect": "/dashboard",
    "userResolver": func(dict<string, any> claims, gebweb.OidcProvider provider): dict<string, any> {
        return {
            "id":    claims["sub"] as string,
            "email": (claims["email"] ?? "") as string,
            "name":  (claims["name"] ?? "") as string,
            "via":   provider.name
        };
    }
});
```

The framework mounts `/auth/google/callback` and
`/auth/github/callback` automatically. Override the prefix with
`opts.callbackPathPrefix` (default `"/auth"`).

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
                "Set-Cookie": "geb_oidc=" + r["stateCookie"] +
                              "; Path=/; HttpOnly; Secure; SameSite=Lax; Max-Age=600"
            },
            "body": ""
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
    "clientId":     "myclient",
    "clientSecret": "...",
    "scopes":       ["openid", "profile", "email"]
});
```

Pass `{"scopes": [...]}` as the third argument to the Google /
GitHub presets to override the default scope list.

GitHub is OAuth2 only - it does not issue an ID token. The
framework hits `https://api.github.com/user` with the access
token and synthesises a claims dict (`sub`, `name`,
`preferred_username`, `email`, `picture`). Other providers run
the OIDC path: the ID token's `iss`, `aud`, and `exp` claims are
validated.

## Limitations in 1.1

- ID-token signature verification against the provider's JWKS is
  not implemented. The token endpoint response is trusted as
  authentic because Gebweb POSTs the code over TLS to the
  provider's `tokenEndpoint`. The `iss` / `aud` / `exp` claims
  are still validated. Full JWKS signature verification is
  scoped for 1.2.
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
| `gebweb.oidc.resolveClaims(provider, tokens)` | Claims extractor that handles both flavours. |

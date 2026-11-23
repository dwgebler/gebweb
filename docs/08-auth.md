# Authentication and roles

Choose authentication to fit the client: sessions for a browser application,
JWT bearer tokens for an API with token issuers and consumers, or API keys for
service accounts. Authentication identifies a user; roles, permissions and
policies decide what that user may do.

For a complete password-login example with tests and deployment instructions,
see [the authentication tutorial](40-authentication.md). It explains where the
key comes from, how tokens are issued, and how an account is loaded again.

## Composable authenticators

`useAuth` registers one user type and an ordered list of authenticators. Built-in
and custom implementations share the same contract:

| Result status | Meaning | Chain behavior |
|---------------|---------|----------------|
| `absent` | No credentials for this authenticator. | Try the next authenticator. |
| `rejected` | Credentials were supplied but rejected. | Stop and deny. |
| `authenticated` | Credentials identify an accepted user. | Stop and use that user. |

This prevents an invalid bearer token from silently falling back to a different
identity. Put authenticators in intentional order. If both a session and a token
are supplied, the first non-absent result wins. Prefer separate apps or explicit
custom selection rules where endpoints have different identity trust domains.

```gb
gebweb.useAuth(app, CurrentUser, [
    gebweb.jwtAuthenticator(verificationKey, fromClaims, {
        "allowedAlgs": ["RS256"],
        "issuer": "https://issuer.example",
        "audience": "my-api",
    }),
    gebweb.apiKeyAuthenticator(fromApiKey),
]);
```

The callbacks return your registered user type or `null`. They should load the
account, reject disabled/deleted accounts and obtain current roles. A valid
signature alone does not establish that an account still exists or is allowed
access. A JWT resolver receives verified claims; an API-key resolver receives
the key. Never log those raw credentials.

Authentication runs lazily for protected routes. To resolve an optional user on
a public route or before-middleware, call `gebweb.authenticate(app, request)`.
It returns an `AuthResult`; `request.user()` then returns its user or `null`.
The route gate, policy helpers and repeated explicit calls share the result,
including an absent or rejected result. A new request makes a fresh decision.

### JWT issuance and verification

`jwtIssue(key, claims, ttlSeconds, opts = {})` signs with HS256 unless `opts.alg`
selects another algorithm. Supported families are HS256/384/512, RS256/384/512,
ES256/384/512 and EdDSA. Unsigned `none` tokens are never supported here.

Use a randomly generated shared secret for HMAC; for asymmetric signing pass a
private PEM key and verify with its public PEM, a JWK or a JWKS dictionary.
The framework does not fetch a URL found in a token. Load verification keys
from trusted configuration. See the tutorial for key generation and rotation.

`jwtVerify(key, token, opts = {})` returns claims or `null` for an invalid token.
It checks an existing `exp` and `nbf`; it does not require expiry unless asked.
Integer Unix seconds are used for `exp`, `nbf` and `iat`. Invalid claim types
are rejected rather than cast. Options:

| Option | Meaning |
|--------|---------|
| `allowedAlgs` | Nonempty list of acceptable algorithms. Always configure for authentication. |
| `issuer` | Require an exact `iss` match. |
| `audience` | Require `aud` to equal this string or contain it in a string list. |
| `requiredClaims` | Claims that must be present and non-null. |
| `leewaySeconds` | Nonnegative tolerance for expiry and not-before checks; default 0. |
| `maxAgeSeconds` | Require `iat` and reject tokens older than this limit or issued in the future beyond tolerance. |

Issuance adds `iat` and, for a positive TTL, `exp`. Explicit claims override
these values. Use a positive TTL in applications. `gebweb.jwt.verifyAt` accepts
an explicit current time for deterministic tests. `decode` only inspects a
token; never use decoded claims as proof of authentication.

`jwtAuthenticator` requires `allowedAlgs` at registration and defaults
`requiredClaims` to `["sub", "exp"]`. It reads Authorization case-insensitively
and accepts a case-insensitive Bearer scheme. A malformed Authorization header
is rejected. It does not accept tokens in URLs or query parameters.

### Custom authenticators

Implement `gebweb.authentication.Authenticator` with `name(): string` and
`authenticate(gebweb.Request): AuthResult`, or adapt a callable:

```gb
import gebweb.authentication as auth;

let custom = auth.CallbackAuthenticator(
    "custom-header",
    func(gebweb.Request request): auth.AuthResult {
        let credential = request.header("X-Custom-Credential");
        if (credential == null) {
            return auth.AuthResult("absent");
        }
        let user = resolveCredential(credential as string);
        return user == null ? auth.AuthResult("rejected") : auth.AuthResult("authenticated", user);
    }
);
gebweb.useAuth(app, CurrentUser, [custom]);
```

Names must be nonempty and unique within a chain. Imported implementations and
inherited methods are supported. Return a rejection for invalid credentials;
throw an error for an infrastructure failure so a database outage cannot become
an authentication fallback.

### Failure responses and security schemes

By default a protected route returns 401 Problem Details. A JWT chain also sets
`WWW-Authenticate: Bearer`. Configure `challenge` to override that header, or
`onFailure` for a browser redirect:

```gb
gebweb.useAuth(app, CurrentUser, authenticators, {"onFailure": func(gebweb.Request request, auth.AuthResult result): any {
    return http.response({
        "status": 302,
        "headers": {"Location": "/login"},
        "body": "",
    });
}});
```

The failure callback handles authentication failure on protected controller and
resource routes, not authorization denial. Roles and policies still return 403.
Only redirect to a fixed or validated local destination.

OpenAPI lists the standard JWT/API-key/session schemes for composed built-ins.
Custom API-key header and session cookie names are reflected automatically.
For custom credentials, provide `securitySchemes` matching your flow through
`useAuth` options or the third argument to `CallbackAuthenticator`. Without
scheme metadata, protected operations carry `x-authentication-required: true`. `defaultScheme` selects one;
omitting it lists the configured alternatives. Documentation of alternatives
cannot express every credential precedence rule; the configured chain is
still authoritative.

### Sessions and API keys in a chain

`sessionAuthenticator(store, fromSession, cookieName = "geb_sid")` resolves a
session through the supplied store. The store's own configuration must agree
with the cookie name. Register `useSession(app, store)` too when using session
helpers, flash messages or forms.

`apiKeyAuthenticator(resolveByKey, opts = {})` reads `X-API-Key`. Set `header`
to change it; Bearer fallback is disabled unless `bearer: true` is set. Store
high-entropy API keys securely, with rotation/revocation and account lookup;
prefer storing a hash of the key over its original plaintext.

## Existing registration APIs

`useAuthenticator(app, userClass, callback)`, `useSessionAuth` and `useApiKeyAuth`
remain supported. Each replaces the previous authentication configuration;
use `useAuth` to compose strategies. Their callbacks return a user or `null`.
They also share their decision within a request. Existing `useApiKeyAuth`
retains its Bearer fallback behavior.

For browser sessions, use the request and response session helpers rather than
constructing cookies manually:

```gb
import web.session as session;

let sessions = session.fileSessionStore("/tmp/app-sessions", 3600);
gebweb.useSessionAuth(app, sessions, CurrentUser, fromSession);
```

Login must verify credentials, rotate the session and set an HttpOnly cookie
with suitable Secure, SameSite and expiry settings. Logout must invalidate the
server-side session as well as expire the cookie. Protect browser login,
logout and other state-changing requests with CSRF validation. See
[CSRF](23-csrf.md) and the [bookmarks tutorial](38-building-bookmarks.md) for
request/response helper examples. Cookie-backed authentication requires CSRF
protection even if the cookie carries a JWT.

## API-key authenticator

For service-to-service traffic, an API key in the request header
is usually simpler than a JWT. Use `gebweb.useApiKeyAuth` to
accept an `X-API-Key` header (or `Authorization: Bearer <key>`)
and look the user up by key:

```gb
class ServiceAccount {
    string id;
    list<string> roles;
    func ServiceAccount(string id, list<string> roles) {
        this.id = id;
        this.roles = roles;
    }
}

let keys = {
    "ada-key":   ServiceAccount("ada", ["owner"]),
    "carla-key": ServiceAccount("carla", ["admin"]),
};

gebweb.useApiKeyAuth(app, ServiceAccount, func(string key): ?ServiceAccount {
    if (!keys.contains(key)) {
        return null;
    }
    return keys[key] as ServiceAccount;
});
```

The header name is matched case-insensitively. A request with no header
or an unknown key gets a 401. From there
the rest of the auth surface (`@Auth`, `@RequiresRole`, user
injection) works the same as with JWT.

## Gating routes

`@Auth` on a method (or its enclosing class) requires authentication:

```gb
class UserController {
    @Get("/me")
    @Auth
    func me(CurrentUser user): dict<string, any> {
        return {"id": user.id, "name": user.name};
    }
}

@Auth
class AccountController {
    @Get("/account/balance")
    func balance(CurrentUser user): dict<string, any> {
        return {"user": user.id, "balance": 100};
    }
}
```

A class-level `@Auth` gates every method. A failed authentication
becomes a 401 Unauthorized.

A route gated by `@Auth`, `@RequiresRole`, `@RequiresPermission` or
`@Can` never runs without an authenticator. If none is configured when
the request arrives, the response is a 500 Internal Server Error and
the handler is not called. The authenticator may still be registered
after `gebweb.app(...)`; the check happens per request.

`@RequiresRole("admin", "owner")` requires at least one matching role:

```gb
@Get("/admin/users")
@Auth
@RequiresRole("admin", "owner")
func listUsers(CurrentUser user): list<dict<string, any>> {
    return [{"requester": user.id}];
}
```

Role mismatches yield 403 Forbidden. `@RequiresRole` implies `@Auth`,
so you can drop the explicit `@Auth` when a role is required.

## User injection

Any handler parameter whose declared type matches the registered user
class receives the resolved user. The injection matches the resolved class identity, including import aliases; the parameter can appear anywhere in the signature.

## Role extraction

The default extractor reads `.roles` off the user value as
`list<string>`. Override it when the user stores roles under a
different field or computes them on demand:

```gb
gebweb.useRoleExtractor(app, func(any user): list<string> { return (user as CurrentUser).roles; });
```

## Permissions (finer than roles)

Roles are coarse ("admin", "editor"). Permissions are fine
("widgets.write", "users.invite"). Gate a route with
`@RequiresPermission("widgets.write")` and the framework checks
the user's `permissions` field for that string:

```gb
class WidgetController {
    @Post("/widgets")
    @RequiresPermission("widgets.write")
    func create(WidgetDto body): dict<string, any> {} /* ... */
}
```

If your user model stores permissions somewhere other than a
`permissions` field, for example looked up from a database,
register a permission extractor:

```gb
gebweb.usePermissions(
    app,
    func(any user): list<string> { return permissionsFor((user as CurrentUser).id); }
);
```

`@RequiresPermission` can decorate a whole controller class to
gate every route in it. Every named permission is required; class and handler
requirements are cumulative, including repeated permission decorators.

## OpenAPI security schemes

`useAuthenticator` registers a default `bearerAuth` (HTTP / bearer /
JWT) entry in `components.securitySchemes`. `useSessionAuth` registers
`cookieAuth` instead. Add additional schemes - or replace the default
- with `useSecurityScheme`:

```gb
gebweb.useSecurityScheme(
    app,
    "apiKey",
    {
        "type": "apiKey",
        "in": "header",
        "name": "X-API-Key",
    },
    true
);
```

Pass `setAsDefault: true` to attach this scheme to every gated
operation in the generated spec.

## Reference

- `gebweb.useAuthenticator(app, userClass, authenticator): GebwebApp`
- `gebweb.useSessionAuth(app, store, userClass, fromSession): GebwebApp`
- `gebweb.useApiKeyAuth(app, userClass, resolveByKey): GebwebApp`
- `gebweb.useRoleExtractor(app, extractor): GebwebApp` - overrides
  the default `.roles` read.
- `gebweb.usePermissions(app, extractor): GebwebApp` - overrides
  the default `.permissions` read.
- `gebweb.useUserScope(app, extractor): GebwebApp` - overrides the
  default `.id` read that partitions cached responses and idempotency
  records per user (see [Caching](10-caching.md#authenticated-routes)).
- `gebweb.useSecurityScheme(app, name, definition, setAsDefault): GebwebApp`
- `gebweb.useAuth(app, userClass, authenticators, opts = {}): GebwebApp`
- `gebweb.authenticate(app, request): AuthResult`
- `gebweb.jwtAuthenticator(key, fromClaims, opts): Authenticator`
- `gebweb.apiKeyAuthenticator(resolveByKey, opts = {}): Authenticator`
- `gebweb.sessionAuthenticator(store, fromSession, cookieName = "geb_sid"): Authenticator`
- `gebweb.jwtIssue(key, claims, ttlSeconds, opts = {}): string`
- `gebweb.jwtVerify(key, token, opts = {}): ?dict<string, any>`
- Decorators: `@Auth`, `@RequiresRole("a", "b", ...)`,
  `@RequiresPermission("widgets.write", ...)`.
- User injection: any handler parameter typed as the registered
  user class.

## Exposing public keys: JWKS

Apps that issue asymmetric JWTs can publish their public keys so
consumers verify without sharing secrets:

```gb
gebweb.useJwks(app, [
    {"pem": crypt.publicKey(currentKey), "kid": "2026-06"},
    {"pem": crypt.publicKey(previousKey), "kid": "2026-01"},
]);
```

This mounts `/.well-known/jwks.json` (override with `{"path":
"..."}`). Keep retiring keys in the set during rotation so tokens
signed before the cutover keep verifying. Consumers verify with the
fetched document directly: `crypt.jwtVerify(token, jwksDict)` selects
the key by the token's `kid`.

## Authorization policies

`@Auth` / `@RequiresRole` answer "may this user reach this endpoint".
Policies answer the per-resource question "may this user perform this
action on *this* row" (e.g. edit a post only if they own it). A policy
class declares one method per action, each tagged `@Policy("TypeName")`
and receiving the authenticated user and the subject:

```gb
class DocPolicy {
    @Policy("Doc")
    func update(CurrentUser user, Doc doc): bool {
        return user.id == doc.ownerId;
    }

    @Policy("Doc")
    func delete(CurrentUser user, Doc doc): bool {
        return user.id == doc.ownerId;
    }
}
```

Policy classes are found automatically: when the app starts handling
requests, the component sweep discovers every class declaring a
`@Policy` method and registers it (built through the DI container), so
no wiring call is needed. Just define the class.

If you want to register a policy explicitly (or trigger the whole sweep
yourself), `gebweb.registerPolicies(app, [DocPolicy()])` and
`gebweb.discover(app)` both still work and are idempotent.

In a handler, after loading the subject, gate the action with
`gebweb.authorize`, which resolves the user via the registered
authenticator, runs the matching policy, and throws a `403` when it
denies or when no policy covers the action:

```gb
@Patch("/docs/:id")
func update(CurrentUser user, gebweb.Request request, string id): dict<string, any> {
    let doc = docRepo.find(id);
    gebweb.authorize(app, request, "update", doc);
    return {"authorized": true};
}
```

`gebweb.can(app, request, action, subject)` is the non-throwing
variant, returning a bool (useful for hiding a UI action).

### Gating `@ApiResource` routes

`@Auth`, `@RequiresRole`, `@RequiresPermission` and `@Can` on an
`@ApiResource` class gate every generated route for that resource, with
the same 401 / 403 responses as a controller route:

```gb
@RequiresRole("admin")
@ApiResource("/users")
class User {
    string id;
    string name;

    static func repositoryClass(): any {
        return UserRepository;
    }
}
```

### Per-row enforcement for `@ApiResource`

When a policy is registered for a resource's entity type, the
`@ApiResource` auto-CRUD routes enforce it automatically: the framework
loads the row and runs the policy for the action - `view` on read,
`update` on PUT/PATCH, `delete` on DELETE - returning `403` when denied.

Enforcement is opt-in per action: a resource with no policy, or an
action with no matching policy method, is not gated, so existing
resources keep working and you gate only the actions you write a method
for. A `view` method gates reads; omit it to leave reads open.

### Type-level gating with `@Can`

`authorize` answers "may this user act on *this* row" and needs the
subject loaded first. Some actions have no row yet: creating a `Doc`,
or listing them. `@Can("action", "Type")` gates those at the type level,
declaratively, before the handler runs. The policy method takes only the
user (no subject):

```gb
class DocController {
    @Post("/docs")
    @Can("create", "Doc")
    func create(CreateDoc body): dict<string, any> {
        /* reached only when the create policy granted it */
        return {"created": true};
    }
}

class DocPolicy {
    @Policy("Doc")
    func create(CurrentUser user): bool {
        return user.roles.contains("editor");
    }
}
```

`@Can` implies authentication (the policy needs the resolved user), so
an unauthenticated request gets a `401` and a denied or unpoliced action
gets a `403`. Unlike the per-row hook, an explicit `@Can` expects a
decision: a missing policy method denies rather than passing through. A
class-level `@Can` gates every route in the controller.

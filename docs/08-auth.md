# Authentication, JWTs and authorization

Choose authentication to fit the client: sessions for a browser application,
JWT bearer tokens for an API with token issuers and consumers, or API keys for
service accounts. Authentication identifies a user; roles, permissions and
policies decide what that user may do.

For a complete password-login example with tests and deployment instructions,
see [the authentication tutorial](40-authentication.md). It explains where the
key comes from, how tokens are issued, and how an account is loaded again.

Use the [RSA walkthrough](#rsa-jwts-issue-verify-and-authenticate-a-request) to
issue and consume a token, [route gates](#gating-routes) to choose endpoint
requirements, and [policies](#authorization-policies) for application decisions
such as record ownership.

## RSA JWTs: issue, verify and authenticate a request

With RSA, the token issuer signs with a **private key** and API servers verify
with its **public key**. An API that only verifies tokens does not need the
private key. Anyone who obtains the private key can issue tokens; sharing the
public key does not give that ability.

The following three scripts form a runnable example. They are also available
under `examples/rsa-auth` in the framework source. Run them from that directory,
or copy them into a project with Gebweb installed.

### 1. Generate a key pair once

Save this as `generate_keys.gb`:

```gb
import crypt;
import io;

let privatePem = crypt.generateRsaKey(3072);
io.writeText("jwt-private.pem", privatePem);
io.writeText("jwt-public.pem", crypt.publicKey(privatePem));
```

Run it with restrictive file permissions on Unix/Linux:

```sh
umask 077
geblang generate_keys.gb
```

This creates a 3072-bit RSA private key and its matching public key as PEM text.
Keep `jwt-private.pem` out of source control and distribute only
`jwt-public.pem` to services that verify tokens. Generate keys during setup,
not on every server start: replacing the key invalidates tokens signed by the
previous key unless verifiers retain that old public key during rotation.

You can use an existing RSA PEM key pair instead. The JWT functions take the
**PEM contents**, so load a file with `io.readText`; passing the filename itself
is not the same thing as passing a key.

### 2. Issue a token with the private key

Save this as `issue_token.gb`:

```gb
import gebweb;
import io;

let privatePem = io.readText("jwt-private.pem");
let publicPem = io.readText("jwt-public.pem");
let token = gebweb.jwtIssue(
    privatePem,
    {
        "sub": "user-42",
        "iss": "example-issuer",
        "aud": "example-api",
    },
    900,
    {"alg": "RS256", "kid": "rsa-key-1"}
);

let claims = gebweb.jwtVerify(publicPem, token, {
    "allowedAlgs": ["RS256"],
    "issuer": "example-issuer",
    "audience": "example-api",
    "requiredClaims": ["sub", "exp"],
});
assert(claims != null);
assert((claims as dict<string, any>)["sub"] == "user-42");
io.println(token);
```

The account identifier is fixed here to make the cryptographic flow easy to
run. In a login handler, issue this token only after verifying the credentials
and loading an active account; use that account's id as `sub`. Do not expose an
endpoint that signs whatever subject or roles a caller requests. The
[password-login tutorial](40-authentication.md) covers that preceding step.

The call to `jwtIssue` does the following:

- Signs the claims with the private PEM using `alg: "RS256"`.
- Adds `iat` for the current Unix time and `exp` 900 seconds later.
- Includes the account id (`sub`), issuer (`iss`) and intended API (`aud`).
- Labels the signing key with `kid: "rsa-key-1"` for key selection during rotation.

**An RSA key does not automatically select RS256.** Issuance defaults to HS256,
so explicitly pass `{"alg": "RS256"}` when signing with an RSA key.
Verification uses `allowedAlgs`, not the issuance option `alg`.

The second half of the script verifies the signature using the public key,
requires subject and expiry, and checks the expected issuer and audience. A
successful signature check alone does not check that a token was issued for
your API. The helper returns `null` if verification or a configured claim check
fails. The script prints only the token after both assertions succeed:

```sh
geblang issue_token.gb > token.txt
```

A signed token is readable, not encrypted. Keep passwords and private key
material out of its claims.

### 3. Verify tokens and resolve a user in an API

Save this as `api.gb`. This server reads only the public key:

```gb
import gebweb;
import io;

class CurrentUser {
    string id;
    list<string> roles;
    func CurrentUser(string id, list<string> roles) {
        this.id = id;
        this.roles = roles;
    }
}

class AccountController {
    @Get("/me")
    @Auth
    func me(CurrentUser user): dict<string, any> {
        return {"id": user.id, "roles": user.roles};
    }

    @Get("/admin")
    @RequiresRole("admin")
    func admin(): dict<string, any> {
        return {"message": "administration access granted"};
    }
}

let publicPem = io.readText("jwt-public.pem");
let app = gebweb.app([AccountController()]);
gebweb.useAuth(app, CurrentUser, [
    gebweb.jwtAuthenticator(
        publicPem,
        func(dict<string, any> claims): ?CurrentUser {
            if (claims["sub"] != "user-42") {
                return null;
            }
            return CurrentUser("user-42", ["editor"]);
        },
        {
            "allowedAlgs": ["RS256"],
            "issuer": "example-issuer",
            "audience": "example-api",
        }
    ),
]);
gebweb.useJwks(app, [{"pem": publicPem, "kid": "rsa-key-1"}]);
gebweb.serve(app, "0.0.0.0:8081");
```

Start it in one terminal:

```sh
geblang api.gb
```

In another terminal in the same directory:

```sh
curl -i http://localhost:8081/me
curl -i -H "Authorization: Bearer $(cat token.txt)" http://localhost:8081/me
curl -i -H "Authorization: Bearer $(cat token.txt)" http://localhost:8081/admin
```

The results are 401, 200 and 403 respectively:

1. `/me` has `@Auth`, so a request without credentials is rejected with 401.
2. The bearer token is verified with the public key and the configured RS256,
   issuer, audience and expiry policy. Only then does the resolver run.
3. The resolver recognizes `user-42` and returns a `CurrentUser` with the
   server-side `editor` role. The framework injects that object into `me`.
4. `/admin` requires `admin`. The same user is authenticated but lacks that role,
   so access is denied with 403 before the admin handler runs.

The example recognizes one account. Replace that lookup with your repository:
find the account by the verified `sub`, reject missing or disabled accounts,
and load its current roles/permissions. Returning `null` rejects the token even
when its signature is valid. Do not trust unverified claims or let the request
choose its roles.

The example binds to all interfaces so it can also be reached from a host
outside a Linux/WSL guest. Use HTTPS for deployed bearer-token APIs.

## Composable authenticators

`useAuth` registers one user type and an ordered list of authenticators. Built-in
and custom implementations share the same contract:

| Result status | Meaning | Chain behavior |
|---------------|---------|----------------|
| `absent` | No credentials for this authenticator. | Try the next authenticator. |
| `rejected` | Credentials were supplied but rejected. | Remember the rejection and try the next authenticator. |
| `authenticated` | Credentials identify an accepted user. | Stop and use that user. |

The first authenticator to accept wins. When none accepts, the request is
`rejected` if any authenticator rejected (the result names the first one to
reject, and `auth.rejected` fires) and `absent` otherwise. A rejected credential
never grants anything by itself: the next authenticator must positively
authenticate its own credential, so a stale session cookie or an expired token
does not block a valid API key, and a JWT authenticator and a Bearer API-key
authenticator can share the `Authorization` header. Put authenticators in
intentional order when one request may carry several valid credentials. Prefer
separate apps or explicit custom selection rules where endpoints have different
identity trust domains.

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
from trusted configuration. The RSA example above shows key generation; the JWKS section below explains rotation.

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

`sessionAuthenticator(store, fromSession)` resolves a session through the
supplied store. The session cookie is the store's `cookieName` (`geb_sid`
unless the store was created with another name, for example
`session.fileSessionStore(dir, ttl, "app_sid")`); the authenticator and the
OpenAPI `cookieAuth` scheme both use it. Register `useSession(app, store)` too when using session
helpers, flash messages or forms.

`apiKeyAuthenticator(resolveByKey, opts = {})` reads `X-API-Key`. Set `header`
to change it; Bearer fallback is disabled unless `bearer: true` is set. With
`bearer: true`, an `Authorization` header using another scheme (such as
`Basic`) counts as absent, while a malformed Bearer value is rejected. Store
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

A gate is a check that must pass before a protected handler runs. Choose the
check according to the decision you need to make:

| Gate | Question it answers | Example |
|------|---------------------|---------|
| `@Auth` | Is this request authenticated? | Any signed-in account may read `/me`. |
| `@RequiresRole("admin", "owner")` | Does the user have at least one named role? | Administrators or owners may enter an admin area. |
| `@RequiresPermission("docs.write", "docs.publish")` | Does the user have every named permission? | Publishing requires both capabilities. |
| `@Can("create", "Doc")` | Does a type-level policy permit this action? | The user may create a document before a document exists. |
| `authorize(app, request, "update", doc)` | Does a policy permit this action on this particular object? | The user owns the document being edited. |

Roles describe broad categories of users. Permissions describe capabilities.
Policies evaluate a decision that depends on application data. An `editor`
role might allow entry to a document-management area while an ownership policy
still prevents that editor from changing someone else's document.

### What runs before a handler

For a matched protected route, the normal order is:

1. Request middleware runs and may return an early response.
2. The configured authenticator resolves a user. A missing or rejected identity
   produces 401; the handler is not called.
3. Required roles, permissions and `@Can` policies are checked. Each category
   present must pass. A denial produces 403; the handler is not called.
4. Handler arguments are bound and validated. The resolved user is injected
   into a parameter typed as the registered user class.
5. The handler runs. It may load an object, return 404 when absent, and call
   `authorize` before changing that object.

Decorators do not create an authenticator. Register one with `useAuth` or an
existing registration helper. A protected route with no authenticator is an
application configuration error: it returns 500, rather than admitting the
request. Authentication can be configured after `gebweb.app(...)`, provided it
is configured before the request is handled.

### Protect one route or a whole controller

```gb
class UserController {
    @Get("/me")
    @Auth
    func me(CurrentUser user): dict<string, any> {
        return {"id": user.id};
    }
}

@Auth
class AccountController {
    @Get("/account")
    func account(CurrentUser user): dict<string, any> {
        return {"id": user.id};
    }

    @Post("/account/export")
    @RequiresPermission("account.export")
    func exportAccount(CurrentUser user): dict<string, any> {
        return {"owner": user.id};
    }
}
```

`@Auth` on `AccountController` protects every routed method in it. The export
route additionally needs `account.export`. Put login and other public routes
in a separate controller when a whole controller is protected.

`@RequiresRole`, `@RequiresPermission` and `@Can` already imply authentication;
adding `@Auth` beside them is optional. A typed `CurrentUser` parameter by itself
does not protect a route.

### Combining class and method gates

Different kinds of gates are cumulative. For example:

```gb
@RequiresRole("editor", "admin")
class PublishingController {
    @Post("/publish")
    @RequiresPermission("docs.write", "docs.publish")
    @Can("create", "Doc")
    func publish(CurrentUser user): dict<string, any> {
        return {"publisher": user.id};
    }
}
```

This route requires an authenticated user who has either `editor` or `admin`,
has **both** permissions, and passes the `Doc.create` policy. Passing only one
of those categories is not enough.

Role names from class and method decorators form **one any-of list**, including
repeated decorators. A class `@RequiresRole("editor")` plus a method
`@RequiresRole("admin")` accepts either role; the method decorator does not
narrow the class requirement to admin. Put an admin-only route in a separate
controller, or express a stricter decision in a policy.

Permissions behave differently: every listed permission is required, including
those from class, method and repeated decorators. Multiple `@Can` declarations
must also all pass. These combination rules are why a role gate and an ownership
policy should be treated as separate decisions.

### Understand the response

| Situation | Result |
|-----------|--------|
| Protected route, no credentials or rejected credentials | 401 Unauthorized, or the configured `onFailure` response. |
| Identity accepted, required role/permission/policy denied | 403 Forbidden. |
| No matching policy for an explicit `@Can` or `authorize` check | 403 Forbidden. |
| Handler looks up a missing object and throws `NotFoundError` | 404 Not Found. |
| Protected route, no authenticator configured | 500 configuration error. |

An authentication failure response, such as redirecting a browser to `/login`,
does not turn a role or policy denial into a login failure. A logged-in user
who lacks access still gets 403.

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

Roles are broad categories such as `editor` or `admin`. Permissions are
capabilities such as `docs.write` and `docs.publish`. Every permission named in
a permission gate must be held:

```gb
class PublishingController {
    @Post("/publish")
    @RequiresPermission("docs.write", "docs.publish")
    func publish(): dict<string, any> {
        return {"published": true};
    }
}
```

The default permission extractor reads `user.permissions` as `list<string>`.
If your user does not expose that field, it returns an empty list and permission
gates deny access. The RSA example's `CurrentUser` has roles only; add and populate
a permissions field, or configure an extractor after registering authentication.

For example, map those roles to capabilities:

```gb
gebweb.usePermissions(app, func(any value): list<string> {
    let user = value as CurrentUser;
    if (user.roles.contains("admin")) {
        return ["docs.write", "docs.publish"];
    }
    if (user.roles.contains("editor")) {
        return ["docs.write"];
    }
    return [];
});
```

With this mapping, an editor fails `/publish` with 403 because `docs.publish`
is missing. An admin has both permissions and passes. In an application, an
extractor can instead query the account's current capabilities or return data
already loaded by the authenticator.

`@RequiresPermission` can decorate a controller class as well as a handler.
Requirements are cumulative: a class requirement for `docs.write` plus a handler
requirement for `docs.publish` requires both. Repeated permission decorators
also combine with AND, unlike the any-of role list.

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
- `gebweb.sessionAuthenticator(store, fromSession): Authenticator`
- `gebweb.jwtIssue(key, claims, ttlSeconds, opts = {}): string`
- `gebweb.jwtVerify(key, token, opts = {}): ?dict<string, any>`
- Decorators: `@Auth`, `@RequiresRole("a", "b", ...)`,
  `@RequiresPermission("widgets.write", ...)`.
- User injection: any handler parameter typed as the registered
  user class on a gated route.

## Exposing public keys: JWKS

The RSA API above publishes its verification key at
`/.well-known/jwks.json` through this call, before `gebweb.serve`:

```gb
gebweb.useJwks(app, [{"pem": publicPem, "kid": "rsa-key-1"}]);
```

A JWKS is a JSON document describing public verification keys. The issuer
puts a `kid` (key id) in each token header; verification selects the matching
key from the set. A key id is a label, not proof of trust. Fetch keys from your
configured issuer endpoint, not a URL supplied by the token.

After loading that trusted document as `jwksDict`, use the same claim policy as
with a public PEM key:

```gb
let claims = gebweb.jwtVerify(jwksDict, token, {
    "allowedAlgs": ["RS256"],
    "issuer": "example-issuer",
    "audience": "example-api",
    "requiredClaims": ["sub", "exp"],
});
```

You can pass the same JWKS dictionary to `jwtAuthenticator` instead of a PEM
string. Neither helper fetches the document for you. A consumer must manage
trusted retrieval, refresh and error handling. Signature verification through
`crypt.jwtVerify` alone does not apply the framework's expiry and claim policy.

For rotation, publish both the new and retiring public keys with different key
ids, start signing with the new private key and its new `kid`, and retain the
old public key until its tokens have expired, including clock tolerance.
Refresh consumer key sets during that rollout. Never publish private PEMs.
Override the publishing URL with the `path` option if needed.

## Authorization policies

A policy is an ordinary class containing methods that return `bool`. It keeps
an authorization decision close to the application rules it depends on, rather
than scattering ownership and role comparisons across handlers.

### How a policy is named and called

Consider these two methods:

```gb
class DocPolicy {
    @Policy("Doc")
    func create(CurrentUser user): bool {
        return user.roles.contains("editor");
    }

    @Policy("Doc")
    func update(CurrentUser user, Doc doc): bool {
        return user.id == doc.ownerId;
    }
}
```

The decorator's string names the subject type, `Doc`. The **method name** names
the action, `create` or `update`. Put `@Policy("Doc")` on each policy method;
`DocPolicy` is a descriptive class name, not how actions are looked up.
Action and type strings must match the registered names, including their case.

| Check | Policy method called | Arguments |
|-------|----------------------|-----------|
| `@Can("create", "Doc")` | `DocPolicy.create` | Authenticated user only. |
| `authorize(app, request, "update", doc)` | `DocPolicy.update` | Resolved user and the loaded `Doc` instance. |
| `can(app, request, "update", doc)` | The same `update` method | The same user and object, returning its bool decision. |

A type-level check has no object yet. Use it for actions such as creating a
record or deciding whether the user may enter a list operation. An instance
check runs after the handler has loaded the object and can inspect its owner,
tenant, status or other fields.

Do not put an instance-style `update(user, doc)` method behind
`@Can("update", "Doc")`: the decorator has no loaded `doc` to pass. Load the
object and call `authorize` inside the handler instead.

### Register the policy

For explicit setup:

```gb
gebweb.registerPolicies(app, [DocPolicy()]);
```

The framework inspects that instance and registers its decorated methods. A
policy with dependencies can be constructed with those dependencies before
registration. It is a shared instance, so keep request-specific data in method
arguments rather than mutable fields on the policy.

Automatic discovery is also available: a loaded class with `@Policy` methods
is discovered and built through DI when the app is discovered/starts handling
requests. Import the module containing the policy; putting a file in a source
directory does not load it. `gebweb.discover(app)` can trigger discovery
explicitly. Policy registration does not protect a controller by itself:
ordinary controller handlers must use `@Can` or call `authorize`.

### Complete example: editors create, owners update

The following `policy_api.gb` is a second runnable server in `examples/rsa-auth`.
It uses the same public key and token created earlier in this chapter. All
logged-in users may read these two example documents. An editor may create one;
only the owner may update an existing document.

```gb
import gebweb;
import gebweb.errors as errors;
import io;

class CurrentUser {
    string id;
    list<string> roles;
    func CurrentUser(string id, list<string> roles) {
        this.id = id;
        this.roles = roles;
    }
}

class Doc {
    string id;
    string ownerId;
    string title;
    func Doc(string id, string ownerId, string title) {
        this.id = id;
        this.ownerId = ownerId;
        this.title = title;
    }
}

class DocInput {
    string title;
}

class DocPolicy {
    @Policy("Doc")
    func create(CurrentUser user): bool {
        return user.roles.contains("editor");
    }

    @Policy("Doc")
    func update(CurrentUser user, Doc doc): bool {
        return user.id == doc.ownerId;
    }
}

func findDoc(string id): ?Doc {
    if (id == "d1") {
        return Doc("d1", "user-42", "My document");
    }
    if (id == "d2") {
        return Doc("d2", "another-user", "Someone else's document");
    }
    return null;
}

class DocController {
    any application;

    func load(string id): Doc {
        let doc = findDoc(id);
        if (doc == null) {
            throw errors.NotFoundError("document not found");
        }
        return doc as Doc;
    }

    @Get("/docs/{id}")
    @Auth
    func show(gebweb.Request request, string id): dict<string, any> {
        let doc = this.load(id);
        return {
            "id": doc.id,
            "title": doc.title,
            "canEdit": gebweb.can(this.application, request, "update", doc),
        };
    }

    @Patch("/docs/{id}")
    @Auth
    func update(gebweb.Request request, string id, DocInput input): dict<string, any> {
        let doc = this.load(id);
        gebweb.authorize(this.application, request, "update", doc);
        doc.title = input.title;
        return {"id": doc.id, "title": doc.title};
    }

    @Post("/docs")
    @Can("create", "Doc")
    func create(CurrentUser user, DocInput input): dict<string, any> {
        return {"title": input.title, "ownerId": user.id};
    }
}

let controller = DocController();
let app = gebweb.app([controller]);
controller.application = app;
let publicPem = io.readText("jwt-public.pem");
gebweb.useAuth(app, CurrentUser, [
    gebweb.jwtAuthenticator(
        publicPem,
        func(dict<string, any> claims): ?CurrentUser {
            if (claims["sub"] != "user-42") {
                return null;
            }
            return CurrentUser("user-42", ["editor"]);
        },
        {
            "allowedAlgs": ["RS256"],
            "issuer": "example-issuer",
            "audience": "example-api",
        }
    ),
]);
gebweb.registerPolicies(app, [DocPolicy()]);
gebweb.serve(app, "0.0.0.0:8082");
```

The sample `findDoc` constructs two fixed documents on each request to keep the
authorization flow visible. Updates return the changed object; they are not
persisted between requests. Replace that lookup and the final write with your
repository in an application.

The handler's order matters:

1. `@Auth` establishes a user before entering `update`.
2. `load(id)` fetches the object and returns 404 if it does not exist.
3. `authorize(..., "update", doc)` invokes `DocPolicy.update(user, doc)`.
4. Only after a true result does the handler change `doc.title`. Persist the
   authorized change at this point in an application.

`controller.application` is wired once during startup so its handler can pass
the app to `authorize`/`can`. Neither the current user nor the current document
is stored on that shared controller.

Start the example with `geblang policy_api.gb`, then try:

```sh
curl -i -H "Authorization: Bearer $(cat token.txt)" http://localhost:8082/docs/d2
curl -i -X PATCH -H "Authorization: Bearer $(cat token.txt)" \
  -H 'Content-Type: application/json' -d '{"title":"Edited"}' \
  http://localhost:8082/docs/d1
curl -i -X PATCH -H "Authorization: Bearer $(cat token.txt)" \
  -H 'Content-Type: application/json' -d '{"title":"Edited"}' \
  http://localhost:8082/docs/d2
curl -i -X POST -H "Authorization: Bearer $(cat token.txt)" \
  -H 'Content-Type: application/json' -d '{"title":"New document"}' \
  http://localhost:8082/docs
```

With the earlier `user-42` token, the outcomes are:

| Request | Result and reason |
|---------|-------------------|
| GET `/docs/d2` | 200 with `canEdit: false`: reading is allowed, but this user is not the owner. |
| PATCH `/docs/d1` | 200: `user-42` owns this document. |
| PATCH `/docs/d2` | 403: authentication succeeded, but the ownership policy denied the update. |
| POST `/docs` | 200: `@Can` called `create(user)` and the account's server-side `editor` role passed. |
| The same protected requests without a token | 401 before the handler or type-level policy runs. |
| PATCH `/docs/missing` with a valid token | 404 from the lookup, before an instance policy can run. |

### `can` versus `authorize`

Use `can` to ask a question, for example to include `canEdit` in a JSON response
or decide whether to render an edit button. It returns false for a missing
policy or a false decision; it does not throw 403 merely because access was
denied. Resolver, database and policy errors still propagate.

Calling `can` and ignoring its result does not enforce anything. Likewise,
hiding a button is not access control: a client can send the write request
directly. The write handler still needs `authorize` (or an equivalent enforced
gate).

Use `authorize` when the request must stop on denial. It throws 403 if the
policy returns false or no matching policy exists. Pair it with `@Auth` on
ordinary controller routes; explicit checks reuse the request's authenticated
user. Policy methods are ordinary code: unexpected exceptions are application
failures, not successful authorization decisions.

If you cache a handler that performs instance checks, remember that a cache hit
can skip its body. Invalidate that cache when ownership or other policy data
changes, or leave the handler uncached. Authentication gates still run before
cache lookup; they do not replace the instance decision inside the handler.

### Policies on generated `@ApiResource` routes

There are two independent layers:

- Class-level `@Auth`, `@RequiresRole`, `@RequiresPermission` and `@Can` protect
  the generated routes before CRUD handling, just as on a controller.
- Instance policies inspect a loaded entity on GET-by-id, PUT/PATCH or DELETE.

The generated per-row checks use these action names:

| Generated operation | Instance policy method |
|---------------------|------------------------|
| GET an individual entity | `view(user, entity)` |
| PUT or PATCH an entity | `update(user, entity)` |
| DELETE an entity | `delete(user, entity)` |

They are opt-in **per action**. If there is no matching `view` policy, a
GET-by-id is not restricted by an instance policy merely because `update`
exists. An absent per-row policy passes through; an explicit `@Can` or
`authorize` check with no matching policy denies. Class-level gates still
apply in either case.

The per-row hooks do not filter list results or automatically check a create
policy. Scope list queries in your repository. A class-level `@Can("create",
"Doc")` applies that type-level check to every generated route on the resource,
not only POST; use a dedicated controller action when different operations need
different type-level policies.

Policy lookup uses the entity's reflected class name. Register the policy for
the type actually returned by the repository; naming a base type does not
implicitly register the same policy for all subclasses.

# Authentication, JWTs and authorization

<!-- doctest:strict -->

Authentication establishes who is calling; authorization decides what that
caller may do. Gebweb separates the two:

- **Authenticators** turn a request's credentials (a bearer token, an API key,
  a session cookie) into your user object, or reject them.
- **Gates** on routes (`@Auth`, `@RequiresRole`, `@RequiresPermission`, `@Can`)
  stop a request before the handler runs.
- **Policies** decide actions that depend on application data, such as "only
  the owner may edit this document".

Choose the credential to fit the client: sessions for a browser app, JWT
bearer tokens for an API whose tokens come from an issuer, API keys for
service accounts. For a complete password-login application, see
[Building authentication](40-authentication.md).

## Signed tokens (JWT)

A JWT is a signed set of claims, such as the account id (`sub`), the issuer
(`iss`), the API it is meant for (`aud`) and an expiry (`exp`). With RSA, the
issuer signs with a **private key** and APIs verify with the matching **public
key**, so an API that only verifies tokens never holds the private key.

<!-- doctest:run -->
```gb
import crypt;
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

@Controller("/account")
class AccountController {
    @Get("/me")
    @Auth
    func me(CurrentUser user): dict<string, any> {
        return {"id": user.id, "roles": user.roles};
    }

    @Get("/admin")
    @RequiresRole("admin")
    func admin(): dict<string, any> {
        return {"area": "admin"};
    }
}

func userFromClaims(dict<string, any> claims): ?CurrentUser {
    return claims["sub"] == "user-42" ? CurrentUser("user-42", ["editor"]) : null;
}

let privatePem = crypt.generateRsaKey(3072);
let publicPem = crypt.publicKey(privatePem);

let app = gebweb.app();
gebweb.useAuth(app, CurrentUser, [
    gebweb.jwtAuthenticator(publicPem, userFromClaims, {
        "allowedAlgs": ["RS256"],
        "issuer": "https://auth.example",
        "audience": "orders-api",
    }),
]);

let claims = {"sub": "user-42", "iss": "https://auth.example", "aud": "orders-api"};
let token = gebweb.jwtIssue(privatePem, claims, 900, {"alg": "RS256", "kid": "key-1"});
let forged = gebweb.jwtIssue(crypt.generateRsaKey(3072), claims, 900, {"alg": "RS256"});

let client = gebweb.TestClient(app);
io.println(client.get("/account/me").status);
io.println(client.request("GET", "/account/me", null, {"Authorization": "Bearer ${token}"}).text());
io.println(
    client.request("GET", "/account/admin", null, {"Authorization": "Bearer ${token}"}).status
);
io.println(client.request("GET", "/account/me", null, {"Authorization": "Bearer ${forged}"}).status);
```

Output:

```text
401
{"id":"user-42","roles":["editor"]}
403
401
```

1. Without credentials, the `@Auth` route answers 401.
2. With the token, the authenticator checks the signature against the public
   key and the configured algorithm, issuer, audience and expiry, and only then
   calls `userFromClaims`. The returned `CurrentUser` is injected into `me`.
3. The same user lacks the `admin` role, so `/admin` answers 403.
4. A token signed with any other key fails verification: 401.

`userFromClaims` is where the application decides whether the account is
allowed in. In a real app it loads the account by the verified `sub`, rejects
missing or disabled accounts, and takes the account's **current** roles from
the database. Returning `null` rejects a token even with a valid signature.
Never take roles from unverified claims or let a request choose them.

The example generates keys on every run so that it is self-contained. A real
issuer generates the key pair once, keeps the private key out of source
control, and gives verifiers only the public key; `examples/rsa-auth` in the
framework source shows that as three separate scripts. The JWT functions take
PEM **contents**: read key files with `io.readText`.

### Issuing tokens

`gebweb.jwtIssue(key, claims, ttlSeconds, opts = {})` signs the claims and adds
`iat` and, for a positive TTL, `exp`. Claims you pass override those. The
algorithm defaults to HS256 even with an RSA key, so pass `{"alg": "RS256"}`
explicitly; supported algorithms are HS256/384/512 (a random shared secret),
RS256/384/512, ES256/384/512 and EdDSA (a private PEM). Unsigned `none` tokens
are never accepted. `kid` labels the key for rotation.

Issue a token only after checking credentials and loading an active account,
and use that account's id as `sub`. Never expose an endpoint that signs
whatever subject or roles the caller asks for. A token is signed, not
encrypted: anyone holding it can read its claims, so keep secrets out of them.

### Verifying tokens

`gebweb.jwtVerify(key, token, opts = {})` returns the claims, or `null` when
the signature or any configured check fails. `jwtAuthenticator` applies the
same options:

| Option | Meaning |
|--------|---------|
| `allowedAlgs` | Algorithms to accept; required by `jwtAuthenticator`. Always set it. |
| `issuer` | Require this exact `iss`. |
| `audience` | Require `aud` to equal this string, or to be a list containing it. |
| `requiredClaims` | Claims that must be present and non-null; the authenticator defaults to `["sub", "exp"]`. |
| `leewaySeconds` | Tolerance for clock skew on `exp` and `nbf`; default 0. |
| `maxAgeSeconds` | Require `iat` and reject tokens older than this. |

Expiry is checked when `exp` is present; require it with `requiredClaims`. A
valid signature alone does not show the token was meant for your API, which is
why `issuer` and `audience` matter. The authenticator reads the
`Authorization: Bearer` header only, never a URL or query parameter.
`gebweb.jwt.decode` reads a token without verifying it; never treat decoded
claims as proof of anything.

### Publishing keys: JWKS

`gebweb.useJwks(app, keys)` serves public keys as a JSON Web Key Set at
`/.well-known/jwks.json`, so other services can verify your tokens:

<!-- doctest:fragment context=app -->
```gb
let publicPem = io.readText("keys/jwt-public.pem");
gebweb.useJwks(app, [{"pem": publicPem, "kid": "key-1"}]);
```

`jwtVerify` and `jwtAuthenticator` also accept a JWKS dictionary instead of a
PEM and pick the key matching the token's `kid`. Neither fetches it for you:
load the key set from your configured issuer, refresh it on a schedule, and
never from a URL the token names. To rotate keys, publish the new and old
public keys under different `kid`s, start signing with the new key, and keep
the old public key until every token it signed has expired. Never publish a
private key. The `path` option changes the URL.

## Authenticators

`gebweb.useAuth(app, UserClass, authenticators, opts = {})` registers your user
class and an ordered list of authenticators. Each one reports one of three
outcomes for a request:

| Outcome | Meaning | Chain |
|---------|---------|-------|
| `absent` | No credentials of its kind. | Try the next authenticator. |
| `rejected` | Credentials present but not accepted. | Remember the rejection; try the next. |
| `authenticated` | Credentials identify an accepted user. | Stop; use this user. |

The first authenticator to accept wins. If none accepts, the request is
rejected when any authenticator rejected, and absent otherwise. A rejected
credential never grants anything, so an expired token does not block a valid
API key in the same request.

<!-- doctest:fragment context=script -->
```gb
import web.session as session;

class CurrentUser {
    string id;
}

func fromClaims(dict<string, any> claims): ?CurrentUser {
    return null;
}

func fromKey(string key): ?CurrentUser {
    return null;
}

func fromSession(dict<string, any> data): ?CurrentUser {
    return null;
}

let app = gebweb.app();
let sessions = session.fileSessionStore("var/sessions", 86400);
gebweb.useSession(app, sessions);
gebweb.useAuth(app, CurrentUser, [
    gebweb.sessionAuthenticator(sessions, fromSession),
    gebweb.jwtAuthenticator(io.readText("keys/jwt-public.pem"), fromClaims, {
        "allowedAlgs": ["RS256"],
        "issuer": "https://auth.example",
        "audience": "orders-api",
    }),
    gebweb.apiKeyAuthenticator(fromKey),
]);
```

| Authenticator | Reads | Notes |
|---------------|-------|-------|
| `jwtAuthenticator(key, fromClaims, opts)` | `Authorization: Bearer` | `fromClaims` receives verified claims. |
| `apiKeyAuthenticator(fromKey, opts = {})` | `X-API-Key` | `opts.header` changes the header; `opts.bearer: true` also accepts `Authorization: Bearer`. Store keys hashed and look the account up by key. |
| `sessionAuthenticator(store, fromSession)` | the session cookie | The store's `cookieName` (`geb_sid` by default). Register `useSession` too for flash messages and forms. |

Each resolver returns your user class or `null`, loads the account itself, and
rejects disabled or deleted accounts. Never log the raw credentials it
receives.

Authentication runs when a protected route needs it. On a public route, call
`gebweb.authenticate(app, request)` to resolve an optional user;
`request.user()` then returns it or `null`. The decision is made once per
request and shared by every gate and policy check in it.

### Custom authenticators

Wrap a callable in `CallbackAuthenticator`, or implement the
`gebweb.authentication.Authenticator` interface (`name()` and
`authenticate(request)`):

<!-- doctest:fragment context=script -->
```gb
import gebweb.authentication as auth;

class Device {
    string id;

    func Device(string id) {
        this.id = id;
    }
}

func deviceForToken(string token): ?Device {
    return token == "valid" ? Device("sensor-1") : null;
}

let devices = auth.CallbackAuthenticator("device-token", func(gebweb.Request request): auth.AuthResult {
    let token = request.header("X-Device-Token");
    if (token == null) {
        return auth.AuthResult("absent");
    }
    let device = deviceForToken(token as string);
    return device == null ? auth.AuthResult("rejected") : auth.AuthResult("authenticated", device);
});
gebweb.useAuth(gebweb.app(), Device, [devices]);
```

Names must be unique within a chain. Return `rejected` for bad credentials,
and throw for an infrastructure failure such as an unreachable database, so an
outage can never be mistaken for "try the next authenticator".

### Failure responses

A protected route that fails authentication answers 401 Problem Details, with
`WWW-Authenticate: Bearer` when a JWT authenticator is in the chain. The
`challenge` option replaces that header; `onFailure` replaces the response,
for example to send a browser to the login page:

<!-- doctest:fragment context=script -->
```gb
import gebweb.authentication as auth;

class CurrentUser {
    string id;
}

func toLogin(gebweb.Request request, auth.AuthResult result): any {
    return http.redirect("/login", 303);
}

gebweb.useAuth(gebweb.app(), CurrentUser, [], {"onFailure": toLogin});
```

`onFailure` handles failed authentication only. A signed-in user who lacks a
role or fails a policy still gets 403, never a login redirect. Redirect only
to fixed or validated local paths.

### Older registration helpers

`useAuthenticator(app, UserClass, fromClaims)`, `useSessionAuth(app, store,
UserClass, fromSession)` and `useApiKeyAuth(app, UserClass, fromKey)` each
register a single strategy and replace any earlier configuration. They remain
supported; use `useAuth` to combine strategies. `useApiKeyAuth` also accepts
`Authorization: Bearer <key>`.

## Gating routes

| Gate | Question | Example |
|------|----------|---------|
| `@Auth` | Is the request authenticated? | Any signed-in account may read `/me`. |
| `@RequiresRole("admin", "owner")` | Does the user have at least one of these roles? | Administrators or owners may enter an admin area. |
| `@RequiresPermission("docs.write", "docs.publish")` | Does the user have every one of these permissions? | Publishing needs both. |
| `@Can("create", "Doc")` | Does the type-level policy allow it? | The user may create documents. |
| `gebweb.authorize(app, request, "update", doc)` | Does the policy allow it on this object? | The user owns this document. |

Gates go on a handler or on the controller class, where they apply to every
route in it. Role, permission and `@Can` gates imply `@Auth`. A typed user
parameter on its own does not protect a route. A protected route on an app
with no authenticator answers 500, a configuration error, rather than letting
the request in.

For a protected route, Gebweb runs, in order: request middleware;
authentication (failure: 401); role, permission and `@Can` gates (denial:
403); binding and validation; the handler, which may load an object, answer
404, and call `authorize`.

### Combining gates

<!-- doctest:fragment context=script -->
```gb
class CurrentUser {
    string id;
    list<string> roles;
}

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

This route needs an authenticated user who has `editor` **or** `admin`, has
**both** permissions, and passes the `Doc.create` policy.

- Role names from the class and the method form one any-of list: a class
  `@RequiresRole("editor")` plus a method `@RequiresRole("admin")` admits either
  role. Put an admin-only route in its own controller, or use a policy.
- Every permission listed, on the class or the method, is required.
- Every `@Can` must pass.

### Roles and permissions

Roles are read from the user's `roles` field and permissions from its
`permissions` field, each a `list<string>`. Change that with an extractor:

<!-- doctest:fragment context=script -->
```gb
class CurrentUser {
    string id;
    list<string> roles;
}

func permissionsFor(any value): list<string> {
    let user = value as CurrentUser;
    if (user.roles.contains("admin")) {
        return ["docs.write", "docs.publish"];
    }
    return user.roles.contains("editor") ? ["docs.write"] : [];
}

let app = gebweb.app();
gebweb.usePermissions(app, permissionsFor);
gebweb.useRoleExtractor(app, func(any value): list<string> { return (value as CurrentUser).roles; });
```

A user without a `permissions` field and no extractor has no permissions, so
every permission gate denies.

| Situation | Response |
|-----------|----------|
| No credentials, or credentials rejected | 401, or the `onFailure` response |
| Authenticated, but a role, permission or policy denies | 403 |
| No policy matches a `@Can` or `authorize` check | 403 |
| The handler finds no such object | 404 |
| Protected route, no authenticator configured | 500 |

## Authorization policies

A policy is a method returning `bool`, marked `@Policy("Type")`. The string
names the subject type and the method name names the action:

<!-- doctest:run -->
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
    @Assert.notBlank
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

class DocRepository {
    dict<string, Doc> docs = {
        "d1": Doc("d1", "ada", "Ada's plan"),
        "d2": Doc("d2", "lin", "Lin's notes"),
    };

    func find(string id): ?Doc {
        return this.docs[id] ?? null;
    }
}

@Controller("/docs")
class DocController extends gebweb.Controller {
    gebweb.GebwebApp app;
    DocRepository docs;

    func DocController(gebweb.GebwebApp app, DocRepository docs) {
        parent();
        this.app = app;
        this.docs = docs;
    }

    func load(string id): Doc {
        let doc = this.docs.find(id);
        if (doc == null) {
            this.notFound("no document ${id}");
        }
        return doc as Doc;
    }

    @Get("/{id}")
    @Auth
    func show(gebweb.Request request, string id): dict<string, any> {
        let doc = this.load(id);
        return {"title": doc.title, "canEdit": gebweb.can(this.app, request, "update", doc)};
    }

    @Patch("/{id}")
    @Auth
    func rename(gebweb.Request request, string id, @Body DocInput input): dict<string, any> {
        let doc = this.load(id);
        gebweb.authorize(this.app, request, "update", doc);
        doc.title = input.title;
        return {"title": doc.title};
    }

    @Post("")
    @Can("create", "Doc")
    func create(CurrentUser user, @Body DocInput input): gebweb.Response {
        return this.created({"title": input.title, "ownerId": user.id}, "/docs/d3");
    }
}

func userForKey(string key): ?CurrentUser {
    if (key == "ada-key") {
        return CurrentUser("ada", ["editor"]);
    }
    return key == "rex-key" ? CurrentUser("rex", ["reader"]) : null;
}

let app = gebweb.app();
gebweb.useAuth(app, CurrentUser, [gebweb.apiKeyAuthenticator(userForKey)]);
let client = gebweb.TestClient(app);
let ada = {"X-API-Key": "ada-key"};
let rex = {"X-API-Key": "rex-key"};
io.println("ada reads d2:     ${client.request("GET", "/docs/d2", null, ada).text()}");
io.println(
    "ada renames d1:   ${client.request("PATCH", "/docs/d1", {"title": "Plan v2"}, ada).status}"
);
io.println(
    "ada renames d2:   ${client.request("PATCH", "/docs/d2", {"title": "Mine"}, ada).status}"
);
io.println("ada creates:      ${client.request("POST", "/docs", {"title": "New"}, ada).status}");
io.println("rex creates:      ${client.request("POST", "/docs", {"title": "New"}, rex).status}");
io.println("anonymous reads:  ${client.get("/docs/d1").status}");
io.println("ada reads zz:     ${client.request("GET", "/docs/zz", null, ada).status}");
```

Output:

```text
ada reads d2:     {"canEdit":false,"title":"Lin's notes"}
ada renames d1:   200
ada renames d2:   403
ada creates:      201
rex creates:      403
anonymous reads:  401
ada reads zz:     404
```

| Check | Policy method called | Arguments |
|-------|----------------------|-----------|
| `@Can("create", "Doc")` | `create` | the user |
| `gebweb.authorize(app, request, "update", doc)` | `update` | the user and the loaded document |
| `gebweb.can(app, request, "update", doc)` | `update` | the same, returning the decision |

- **Type-level checks** (`@Can`) run before the handler, with no object yet:
  use them for actions such as creating.
- **Instance checks** (`authorize`) run in the handler after the object is
  loaded, so they can compare its owner, tenant or status. Load first, so a
  missing object is a 404, then authorize, then change it.
- `can` answers a question, for example to show an edit button; it does not
  enforce anything. The write handler must still call `authorize`, because a
  client can send the request without the button.
- `authorize` answers 403 when the policy returns false or no policy matches.

Policy classes are found by discovery when their module is loaded, and built
by the container, so they can take repositories in their constructor. They
are shared across requests: keep request data in method arguments. To register
policies explicitly, use `gebweb.registerPolicies(app, [DocPolicy()])`. The
controller gets the app through its constructor, as
[Dependency injection](06a-dependency-injection.md#lifetimes) describes.

A cached handler can skip its body on a cache hit, and with it any `authorize`
call inside. Leave handlers with instance checks uncached, or invalidate the
cache when ownership changes.

### Policies on `@ApiResource` routes

Class-level gates on an `@ApiResource` entity protect all its generated routes.
In addition, generated routes call instance policies on a loaded entity:

| Generated operation | Policy method |
|---------------------|---------------|
| GET one entity | `view(user, entity)` |
| PUT or PATCH | `update(user, entity)` |
| DELETE | `delete(user, entity)` |

Each is optional: a missing `view` policy leaves GET unrestricted even if
`update` exists. Listing and creating are not checked per row, so scope list
queries in the repository. A class-level `@Can("create", "Doc")` applies to
every generated route, not only POST. The policy is looked up by the entity's
class name; a policy for a base class does not cover subclasses.

## Browser sessions and CSRF

A browser signs in once and is then identified by a session cookie. The login
handler checks the password, starts a fresh session, and sets an `HttpOnly`
cookie with `Secure` and `SameSite` set for your deployment; logout destroys
the server-side session and expires the cookie. Because browsers send cookies
automatically, every state-changing request authenticated by a cookie needs
[CSRF protection](23-csrf.md), even when the cookie holds a JWT.
[Building authentication](40-authentication.md) walks through a complete
login, and the [bookmarks tutorial](38-building-bookmarks.md) shows sessions
with forms.

## OpenAPI security schemes

The generated OpenAPI document lists security schemes for the built-in
authenticators in the chain (bearer JWT, API key with its header name, session
cookie with its cookie name) and marks protected operations. For custom
credentials, describe the scheme yourself:

<!-- doctest:fragment context=app -->
```gb
gebweb.useSecurityScheme(
    app,
    "deviceToken",
    {"type": "apiKey", "in": "header", "name": "X-Device-Token"},
    true
);
```

The last argument makes it the default for protected operations. Without any
scheme, protected operations carry `x-authentication-required: true`. See
[OpenAPI and SwaggerUI](13-openapi.md).

## Reference

| API | Purpose |
|-----|---------|
| `gebweb.useAuth(app, UserClass, authenticators, opts = {})` | Register the user class and an authenticator chain; `opts`: `onFailure`, `challenge`, `securitySchemes`, `defaultScheme`. |
| `gebweb.jwtAuthenticator(key, fromClaims, opts)` | Bearer JWT; options as in the verification table. |
| `gebweb.apiKeyAuthenticator(fromKey, opts = {})` | API key; `opts`: `header`, `bearer`. |
| `gebweb.sessionAuthenticator(store, fromSession)` | Session cookie. |
| `gebweb.authenticate(app, request): AuthResult` | Resolve the user on a public route. |
| `gebweb.jwtIssue(key, claims, ttlSeconds, opts = {})` / `gebweb.jwtVerify(key, token, opts = {})` | Sign and verify tokens. |
| `gebweb.useJwks(app, keys, opts = {})` | Publish public keys. |
| `gebweb.usePermissions(app, fn)` / `gebweb.useRoleExtractor(app, fn)` | Read permissions and roles from your user. |
| `gebweb.useUserScope(app, fn)` | The user id used to partition caches; see [Caching](10-caching.md#authenticated-routes). |
| `gebweb.can(app, request, action, subject)` / `gebweb.authorize(...)` | Ask or enforce a policy. |
| `gebweb.registerPolicies(app, policies)` | Register policy instances explicitly. |
| `gebweb.useSecurityScheme(app, name, definition, setAsDefault)` | Add an OpenAPI security scheme. |
| `@Auth`, `@RequiresRole(...)`, `@RequiresPermission(...)`, `@Can(action, type)`, `@Policy(type)` | Gates and policy methods. |

# Authentication and roles

Gebweb holds at most one authenticator per app. The authenticator is
a callable that takes a `gebweb.Request` and returns either a user
value or `null`. Routes are gated with `@Auth` (any authenticated
user) or `@RequiresRole("a", "b")` (any-of role check).

## JWT authenticator

```gb
class CurrentUser {
    string id;
    string name;
    list<string> roles;

    func CurrentUser(string id, string name, list<string> roles) {
        this.id = id;
        this.name = name;
        this.roles = roles;
    }
}

let app = gebweb.app([UserController(), AdminController()]);
gebweb.useAuthenticator(app, CurrentUser, func(gebweb.Request req): ?any {
    let headers = (req["headers"] ?? {}) as dict<string, any>;
    let header = (headers["Authorization"] ?? "") as string;
    if (!header.startsWith("Bearer ")) { return null; }
    let claims = gebweb.jwtVerify("secret", header.substring(7, header.length()));
    if (claims == null) { return null; }
    let c = claims as dict<string, any>;
    return CurrentUser(c["sub"] as string,
                       (c["name"] ?? "") as string,
                       (c["roles"] ?? []) as list<string>);
});
```

Use `gebweb.jwtIssue(secret, claims, ttlSeconds)` to mint a token in a
login handler; `gebweb.jwtVerify(secret, token)` is the inverse and
returns `null` on signature mismatch, malformed input, or expired
`exp` claim.

## Session authenticator

For cookie-backed session login, `gebweb.useSessionAuth` wraps a
stdlib session store:

```gb
import web.session as session;

let store = session.fileSessionStore("/tmp/app-sessions", 3600);
gebweb.useSessionAuth(app, store, CurrentUser,
    func(dict<string, any> data): ?CurrentUser {
        if (!data.contains("id")) { return null; }
        return CurrentUser(
            data["id"] as string,
            (data["name"] ?? "") as string,
            (data["roles"] ?? []) as list<string>);
    });
```

Login and logout stay user-controlled:

```gb
@Post("/login")
func login(LoginDTO body): dict<string, any> {
    /* verify credentials, then ... */
    let res = (store).save({"status": 200, "body": {"ok": true}},
        {"id": "u-1", "name": body.name, "roles": ["user"]},
        {"path": "/", "maxAge": 3600});
    return res as dict<string, any>;
}

@Post("/logout")
func logout(dict<string, any> request): dict<string, any> {
    return (store).clear({"status": 204, "body": ""}, request) as dict<string, any>;
}
```

The session-auth path swaps the default OpenAPI security scheme from
`bearerAuth` (JWT) to `cookieAuth` (apiKey / cookie / `geb_sid`).

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
    "ada-key":   ServiceAccount("ada",   ["owner"]),
    "carla-key": ServiceAccount("carla", ["admin"]),
};

gebweb.useApiKeyAuth(app, ServiceAccount, func(string key): ?ServiceAccount {
    if (!keys.contains(key)) { return null; }
    return keys[key] as ServiceAccount;
});
```

A request with no header or an unknown key gets a 401. From there
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
class receives the resolved user. The injection is by exact type
name; the parameter can appear anywhere in the signature.

## Role extraction

The default extractor reads `.roles` off the user value as
`list<string>`. Override it when the user stores roles under a
different field or computes them on demand:

```gb
gebweb.useRoleExtractor(app, func(any user): list<string> {
    return (user as CurrentUser).roles;
});
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
    func create(WidgetDto body): dict<string, any> { /* ... */ }
}
```

If your user model stores permissions somewhere other than a
`permissions` field, for example looked up from a database,
register a permission extractor:

```gb
gebweb.usePermissions(app, func(any user): list<string> {
    return permissionsFor((user as CurrentUser).id);
});
```

`@RequiresPermission` can decorate a whole controller class to
gate every route in it.

## OpenAPI security schemes

`useAuthenticator` registers a default `bearerAuth` (HTTP / bearer /
JWT) entry in `components.securitySchemes`. `useSessionAuth` registers
`cookieAuth` instead. Add additional schemes - or replace the default
- with `useSecurityScheme`:

```gb
gebweb.useSecurityScheme(app, "apiKey", {
    "type": "apiKey",
    "in": "header",
    "name": "X-API-Key",
}, true);
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
- `gebweb.useSecurityScheme(app, name, definition, setAsDefault): GebwebApp`
- `gebweb.jwtIssue(secret, claims, ttlSeconds): string`
- `gebweb.jwtVerify(secret, token): ?dict<string, any>`
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
func update(CurrentUser user, string id): dict<string, any> {
    let doc = docRepo.find(id);
    gebweb.authorize(app, request, "update", doc);   // 403 if denied
    // ... apply the update ...
}
```

`gebweb.can(app, request, action, subject)` is the non-throwing
variant, returning a bool (useful for hiding a UI action).

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

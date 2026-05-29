# Authentication and roles

Gebweb holds at most one authenticator per app. The authenticator is
a callable that takes the raw request dict and returns either a user
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
gebweb.useAuthenticator(app, CurrentUser, func(dict<string, any> req): ?any {
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
- `gebweb.useRoleExtractor(app, extractor): GebwebApp` - overrides
  the default `.roles` read.
- `gebweb.useSecurityScheme(app, name, definition, setAsDefault): GebwebApp`
- `gebweb.jwtIssue(secret, claims, ttlSeconds): string`
- `gebweb.jwtVerify(secret, token): ?dict<string, any>`
- Decorators: `@Auth`, `@RequiresRole("a", "b", ...)`.
- User injection: any handler parameter typed as the registered
  user class.

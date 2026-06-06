# OpenAPI and SwaggerUI

Gebweb generates an OpenAPI 3.1 spec from the registered routes and
serves it at `/openapi.json`. SwaggerUI mounts at `/docs`. The spec
is built from three sources:

1. **Route registration.** Method + path + path parameters come from
   the route decorators.
2. **Handler reflection.** Parameter types become request schemas;
   return types become response schemas.
3. **Decorator metadata.** `@Summary`, `@Description`, `@Tag`,
   `@OperationId`, `@Deprecated`, `@ApiResponse`, `@Auth`,
   `@RequiresRole`, and resource-level `@ApiResource` options all
   feed into the spec.

## Customising the spec

`gebweb.setInfo(app, info)` overrides the `info` object:

```gb
let app = gebweb.setInfo(gebweb.app([UserController()]), {
    "title": "Users API",
    "version": "1.0.0",
    "description": "User management API.",
});
```

The default `info` is `{"title": "Gebweb API", "version": "0.1.0"}`.

## Operation-level metadata

```gb
@Get("/users/{id}")
@Summary("Look up a user")
@Description("Returns 404 when the user does not exist.")
@Tag("Users")
@OperationId("getUser")
@ApiResponse(200, "Found", schema: User)
@ApiResponse(404, "Not found")
@Deprecated
func get(string id): User { /* ... */ }
```

| Decorator | Spec field | Notes |
|-----------|------------|-------|
| `@Summary("text")` | `operation.summary` | One-line label shown in SwaggerUI. |
| `@Description("text")` | `operation.description` | Markdown supported. |
| `@Tag("name")` | `operation.tags` | Default tag is the controller name without `Controller`. |
| `@OperationId("id")` | `operation.operationId` | Default is `<controller>_<method>`. |
| `@Deprecated` | `operation.deprecated = true` | |
| `@ApiResponse(status, description, schema?)` | adds a `responses[status]` entry | Multiple permitted. |

## Schema inference

Parameters typed as user classes are added to `components.schemas`
and referenced via `$ref`. The schema includes:

- One JSON-schema property per declared field.
- Per-field nullability (`?T` types).
- Per-field type, with primitive types mapped to OpenAPI standard
  types (`string`, `integer`, `number`, `boolean`).
- Optional `description` taken from a field-level doc comment.

A `list<UserDTO>` parameter or return type becomes an `array` of
`$ref` items.

## Security schemes

The first call to `useAuthenticator` registers a default `bearerAuth`
(HTTP / bearer / JWT) entry; `useSessionAuth` swaps that to
`cookieAuth` (apiKey / cookie / `geb_sid`). Add or override with
`useSecurityScheme`:

```gb
gebweb.useSecurityScheme(app, "apiKey", {
    "type": "apiKey",
    "in": "header",
    "name": "X-API-Key",
}, true);
```

Every operation gated by `@Auth` or `@RequiresRole` carries the
default scheme name in its `security` field. The flag in
`useSecurityScheme` (4th arg, `setAsDefault`) controls which scheme
that is.

## SwaggerUI mount

`/docs` serves a Bootstrap-themed SwaggerUI page. The page reads
`/openapi.json` so any spec change is live without restarting.
Customise the page title via `setInfo({"title": ...})`.

### Offline assets in a built binary

In development the page loads the SwaggerUI CSS/JS from a pinned CDN, so dev
stays dependency-free. When you `gebweb build`, the pinned SwaggerUI assets are
downloaded once (cached under your user cache dir), embedded in the binary, and
served from local `/docs/...` routes. The built binary's docs page works
offline with no CDN dependency. Pass `gebweb build --no-swagger` to skip
embedding the assets (for example when you override `/docs` with your own page).

## Hiding the docs in production

By default `/openapi.json` and `/docs` are open to anyone who
can reach the server. For production, gate them with
`gebweb.useDocsAuth`:

```gb
gebweb.useDocsAuth(app, gebweb.basicAuthGuard("admin", "s3cret"));
```

Pick whichever of these three matches your setup:

- `gebweb.basicAuthGuard(user, password)` for an HTTP Basic
  prompt in the browser.
- `gebweb.bearerTokenGuard(expected)` for an `Authorization:
  Bearer <token>` check with a single shared token.
- `gebweb.requireAppAuth(app, roles = [])` to reuse whatever
  authenticator your app already has, optionally restricted to
  a role.

A failed check returns 401 (or 403 if the role check fails).

For something more involved (IP allow-listing, a custom auth
header, etc.), write your own guard. A guard is just a callable
that takes the request dict and returns either `null` (let the
request through) or a response dict (short-circuit).

## Documenting error responses

By default the framework only documents the success response.
For 4xx / 5xx responses, add an `@ApiResponse(status, description,
schema?)` decorator:

```gb
import gebweb.errors as errors;

@Get("/users/{id}")
@ApiResponse(200, "The user")
@ApiResponse(404, "Not found", schema: errors.Problem)
func get(string id): User { /* ... */ }
```

`errors.Problem` is the framework's built-in schema for RFC 9457
problem-details responses (the body shape thrown `HttpException`s
end up with). Use it as the `schema` argument so clients see the
exact `{status, title, detail}` shape they should expect.

## Spec access from code

`gebweb.openapi.build(info, routes, auth)` returns the raw spec dict
the framework serves. Use it to dump the spec to disk:

```gb
import gebweb.openapi as openapi;
import io;
import json;

let spec = openapi.build(app.info, gebweb.routes(app), app.auth);
io.writeText("openapi.json", json.stringify(spec));
```

## Reference

- `gebweb.setInfo(app, info): GebwebApp` - set the OpenAPI `info`.
- Decorators (see [Routing](02-routing.md)): `@Summary`,
  `@Description`, `@Tag`, `@OperationId`, `@Deprecated`,
  `@ApiResponse`.
- Security: `gebweb.useSecurityScheme(app, name, definition,
  setAsDefault)`; default schemes are registered by
  `useAuthenticator` / `useSessionAuth`.
- Mounts: `/openapi.json` (spec), `/docs` (SwaggerUI).
- Lock the mounts: `gebweb.useDocsAuth(app, guard)` plus
  `basicAuthGuard` / `bearerTokenGuard` / `requireAppAuth`.
- Direct access: `gebweb.openapi.build(info, routes, auth)`.

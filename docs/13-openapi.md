# OpenAPI and SwaggerUI

<!-- doctest:strict -->

Gebweb builds an OpenAPI 3.1 document from registered routes, handler types and
metadata. Use it to inspect an API in SwaggerUI, generate a client, or check
that a response change has not silently changed the public contract. The JSON
is served at `/openapi.json`; the interactive page is at `/docs` when enabled.

## Start with typed handlers

Declare the JSON shapes as classes. Here the input and output are separate:
the client supplies a name, while the server assigns an identifier.

<!-- doctest:run -->
```gb
import gebweb;
import gebweb.errors as errors;
import io;
import json;

class UserInput {
    @Assert.notBlank
    string name;
}

class User {
    string id;
    string name;

    func User(string id, string name) {
        this.id = id;
        this.name = name;
    }
}

@Controller("/users")
@Tag("Users")
class UserController extends gebweb.Controller {
    @Get("/{id}")
    @Summary("Get a user")
    @OperationId("getUser")
    @ApiResponse(404, "Not found", schema: errors.Problem)
    func get(string id): User {
        if (id != "u1") {
            this.notFound("user not found");
        }
        return User(id, "Ada");
    }

    @Post("")
    @ApiResponse(201, "Created", schema: User)
    func create(UserInput input): gebweb.Response {
        return this.created(User("u2", input.name), "/users/u2");
    }
}

let app = gebweb.app([], {"docs": true});
gebweb.setInfo(app, {"title": "Users API", "version": "1.0.0"});
let client = gebweb.TestClient(app);
let spec = client.get("/openapi.json").json();
let getUser = spec["paths"]["/users/{id}"]["get"];
let create = spec["paths"]["/users"]["post"];

io.println("${spec["openapi"]} ${spec["info"]["title"]}");
io.println("${getUser["operationId"]} ${getUser["summary"]}");
io.println(json.stringify(getUser["responses"]["200"]["content"]["application/json"]["schema"]));
io.println(json.stringify(create["requestBody"]["content"]["application/json"]["schema"]));
io.println(getUser["responses"]["404"]["content"].keys()[0]);
io.println(client.get("/users/u1").json()["name"]);
```

Output:

```text
3.1.0 Users API
getUser Get a user
{"$ref":"#/components/schemas/User"}
{"$ref":"#/components/schemas/UserInput"}
application/problem+json
Ada
```

The empty controller list enables `@Controller` discovery. `TestClient` loads
the registered routes and asks for the same spec the server serves. In an
application, make these settings in `build(conn)` before calling `gebweb.cli`;
see [Getting started](01-getting-started.md) for the entrypoint and resource
cleanup pattern.

## Customising the spec

`gebweb.setInfo(app, info)` merges keys into the `info` object and returns the
app. A later value replaces the earlier value for that key. The initial
`info` is `{"title": "Gebweb API", "version": "0.1.0"}`.

| Common `info` key | Initial value | Effect |
|-------------------|---------------|--------|
| `title` | `Gebweb API` | API title; also used as the SwaggerUI page title. |
| `version` | `0.1.0` | Your API's version, independent of the framework version. |
| `description` | Absent | API description passed through into the document. |

Other keys are passed through unchanged. Keep API descriptions and deployment
addresses in configuration rather than editing the generated JSON.

## Operation-level metadata

| Decorator | Where it applies | Generated field |
|-----------|------------------|-----------------|
| `@Summary("text")` | Handler | `summary`; omitted when not set. |
| `@Description("text")` | Handler or parameter | Operation or parameter `description`; a body parameter describes `requestBody`. |
| `@Tag("name")` | Handler, then controller | `tags`; otherwise the controller name without a trailing `Controller`. |
| `@OperationId("id")` | Handler | `operationId`; omitted when not set. |
| `@Deprecated` | Handler or controller | `deprecated: true`. A supplied date also becomes `x-sunset`. |
| `@ApiResponse(status, description, schema: Class)` | Handler | Adds or updates an entry in `responses`; multiple annotations are allowed. |

Choose explicit, stable operation identifiers when generated clients depend on
them. See [Routing](02-routing.md) for versioned routes and deprecation headers.
Unknown decorators remain general metadata; they are not rejected at startup.

## Schema inference

| Declared type | Generated schema |
|---------------|------------------|
| `string`, `int`, `float` / `decimal`, `bool` | `string`, `integer`, `number`, `boolean` respectively. |
| `bytes` | String with `contentEncoding: base64`. |
| `?T` for a primitive, `list<T>`, `set<T>` or class | Nullable primitive/container type, or `oneOf` with a class reference and null. |
| User class | Reference to `components.schemas`, including inherited fields and referenced field classes. |
| `list<T>` or `set<T>` | Array with an element schema. |
| Bare `list` or `set` | Array without an item schema. |
| `dict` / `dict<K, V>` or `any` used as a schema | Object with additional properties allowed. |

Primitive containers keep their element types through nesting and nullability:

<!-- doctest:run -->
```gb
import gebweb.openapi as openapi;
import io;
import json;

io.println(json.stringify(openapi.schemaFor("set<int>")));
io.println(json.stringify(openapi.schemaFor("?set<?int>")));
io.println(json.stringify(openapi.schemaFor("list<set<string>>")));
```

Output:

```text
{"items":{"type":"integer"},"type":"array"}
{"items":{"type":["integer","null"]},"type":["array","null"]}
{"items":{"items":{"type":"string"},"type":"array"},"type":"array"}
```

Handler class references are resolved in their declaring module, including
import aliases. Components normally use the bare class name (`User`). When two
modules contribute classes with the same name, both component names become
`module.Class` to keep them distinct.

Non-nullable class fields are listed as required. `@Assert` rules validate
requests, but they are not translated into schema constraints such as
`minLength` or `pattern`. Field doc comments are not currently emitted as
property descriptions. Do not infer a validation guarantee from an absent
constraint in the spec; see [Validation](05-validation.md).

### Documenting error responses

A `void` handler infers a 204 response. Other handlers infer 200; a typed class
or primitive return documents its body, while `gebweb.Response`, `any` and dict
returns leave the body schema unspecified. Use `@ApiResponse` when returning a
response object with a known body, as the 201 example above does.

Without `schema`, an annotation changes the description and preserves an
inferred body schema at that status. An explicit class replaces that status's
content schema. An annotation for another status adds an entry; it does not
remove the inferred success response or change the actual HTTP status.

`schema:` takes a user class reference, including an imported class. A string,
dict, interface, enum, function or null raises `ValueError` when building the
spec. Explicit classes include inherited fields and nested classes.

`errors.Problem` and its subclasses use `application/problem+json`; other
classes use `application/json`. The Problem schema exposes `type`, `title`,
`status` and `detail`, projecting its source field `typeUri` to `type` on the
wire. Use an `HttpException` or a controller's error helpers to emit the actual
response; do not serialize a bare Problem instance as an error body. See
[Errors](04-responses.md#errors) and `examples/response_schemas.gb`.

## Security schemes

Schemes describe credentials to API clients; declaring a scheme does not
install an authenticator. Configure authentication first. This example uses an
API key, then gives its scheme a custom name:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import json;

class CurrentUser {
    string id = "u1";
}

@Controller("/private")
class PrivateController {
    @Get("/me")
    @Auth
    func me(CurrentUser user): CurrentUser {
        return user;
    }
}

let app = gebweb.app([], {"docs": true});
gebweb.useApiKeyAuth(
    app,
    CurrentUser,
    func(string key): ?CurrentUser { return key == "example-only" ? CurrentUser() : null; }
);
gebweb.useSecurityScheme(
    app,
    "serviceKey",
    {"type": "apiKey", "in": "header", "name": "X-API-Key"},
    true
);
let client = gebweb.TestClient(app);
let spec = client.get("/openapi.json").json();

io.println(json.stringify(spec["paths"]["/private/me"]["get"]["security"]));
io.println(spec["components"]["securitySchemes"]["serviceKey"]["in"]);
io.println(client.get("/private/me").status);
io.println(client.request("GET", "/private/me", null, {"X-API-Key": "example-only"}).json()["id"]);
```

Output:

```text
[{"serviceKey":[]}]
header
401
u1
```

`useSecurityScheme(app, name, definition, setAsDefault)` registers or replaces
that definition. `setAsDefault: true` selects the scheme used on authenticated
operations; `false` leaves the current default alone. It requires a prior auth
configuration. The definition is passed through as supplied.

`useAuthenticator` starts with `bearerAuth` (HTTP bearer, JWT format),
`useSessionAuth` uses `cookieAuth` with the session store's cookie name, and
`useApiKeyAuth` uses `apiKeyAuth` for `X-API-Key`. With composed `useAuth`, no
explicit default means the published schemes are alternatives. A composed
custom authenticator without a scheme marks gated operations with
`x-authentication-required` instead.

Role, permission and policy gates mark an operation as requiring authentication;
the required roles are not expressed as bearer scopes. The injected user is
not documented as a client-supplied parameter. For the full authentication
contracts, see [Authentication](40-authentication.md) and
[Auth and permissions](08-auth.md).

## SwaggerUI mount

SwaggerUI fetches the spec from `/openapi.json` and uses `info.title` as its page
title. Configure it during app construction; changes to application source
still need the normal restart.

### Choosing which routes to mount

Options belong to `gebweb.app(controllers, opts)`, not to `setInfo`:

| Option | Type | Default | Effect |
|--------|------|---------|--------|
| `docs` | `bool` | Environment policy below | Shared fallback for both docs routes. |
| `openapi` | `bool` | `docs`, then environment policy | Whether `/openapi.json` is mounted. |
| `swaggerUi` | `bool` | `docs`, then environment policy | Whether `/docs` is mounted; always disabled when OpenAPI is disabled. |

An explicit option wins over `docs`, which wins over the environment default.
`GEBWEB_DOCS=off`, `false`, `no` or `0` disables the routes; another nonempty
value enables them. Without that override, explicitly setting
`GEBWEB_ENV=prod` or `production` disables them. An unset environment leaves
them enabled. An enabled spec is public unless protected with a docs guard.

<!-- doctest:run -->
```gb
import gebweb;
import io;

let hidden = gebweb.TestClient(gebweb.app([], {"docs": false}));
io.println([hidden.get("/openapi.json").status, hidden.get("/docs").status]);
let specOnly = gebweb.TestClient(gebweb.app([], {"openapi": true, "swaggerUi": false}));
io.println([specOnly.get("/openapi.json").status, specOnly.get("/docs").status]);
```

Output:

```text
[404, 404]
[200, 404]
```

### Offline assets in a built binary

When serving source, SwaggerUI loads its CSS and JavaScript from a pinned CDN.
`gebweb build` downloads the pinned assets once into the user cache, embeds
them, and serves them from local `/docs/...` routes so the built docs page can
work offline. Those local assets use the same docs guard as the page and spec.

`gebweb build --no-swagger` skips embedding. Use it when the built-in UI is
disabled or you provide its assets yourself; an enabled built-in page still
points at local assets in a built binary. See [CLI](15-cli.md).

## Hiding the docs in production

If you enable docs in production, configure a guard before `gebweb.cli` or the
first request. `useDocsAuth` applies to the page, JSON spec and bundled assets:

<!-- doctest:run -->
```gb
import encoding;
import gebweb;
import io;

let app = gebweb.app([], {"docs": true});
gebweb.useDocsAuth(app, gebweb.basicAuthGuard("docs", "example-only"));
let client = gebweb.TestClient(app);
let headers = {"Authorization": "Basic " + encoding.base64Encode("docs:example-only")};

io.println(client.get("/docs").status);
io.println(client.request("GET", "/docs", null, headers).status);
io.println(client.request("GET", "/openapi.json", null, headers).json()["openapi"]);
```

Output:

```text
401
200
3.1.0
```

The credentials above are only for the example. In an application, pass
credentials loaded from configuration or secrets into the guard.

| Guard | Result when denied |
|-------|--------------------|
| `basicAuthGuard(user, password)` | 401 with an HTTP Basic challenge. |
| `bearerTokenGuard(expected)` | 401 with a Bearer challenge for a missing or wrong shared token. |
| `requireAppAuth(app, requiredRoles = [])` | 401 without an authenticated user, 403 if none of the required roles match. Auth must already be configured. |

A custom guard receives a `gebweb.Request` and returns null to continue or a
response dict to deny access:

<!-- doctest:fragment context=app -->
```gb
gebweb.useDocsAuth(app, func(gebweb.Request request): ?dict<string, any> {
    if (request.header("X-Docs-Key") == "example-only") {
        return null;
    }
    return {"status": 403, "headers": {"Content-Type": "text/plain"}, "body": "Forbidden"};
});
```

### SwaggerUI with CSRF protection

With `gebweb.useCsrf`, a successful `/docs` request issues the CSRF cookie and
embeds its token in the page's request interceptor. SwaggerUI sends
`X-CSRF-Token` on "Try it out" requests; it does not need to read the HttpOnly
cookie in JavaScript. A rejected docs guard does not issue the token.

The in-process client does not keep a browser cookie jar, so this test sends
both the cookie and header explicitly:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class Note {
    string text;
}

@Controller("/notes")
class NoteController {
    @Post("")
    func save(Note note): Note {
        return note;
    }
}

let app = gebweb.app([], {"docs": true});
gebweb.useCsrf(app, {"secret": "example-only-csrf-secret"});
let client = gebweb.TestClient(app);
let page = client.get("/docs");
let setCookie = page.headers["Set-Cookie"];
string cookie = (setCookie instanceof list ? (setCookie as list<any>)[0] : setCookie) as string;
cookie = cookie.split(";")[0];
string token = cookie.substring("geb_csrf=".length());

io.println(client.post("/notes", {"text": "hi"}).status);
let result = client.request("POST", "/notes", {"text": "hi"}, {
    "Cookie": cookie,
    "X-CSRF-Token": token,
});
io.println(result.status);
io.println(page.text().contains("X-CSRF-Token"));
io.println(result.json()["text"]);
```

Output:

```text
403
200
true
hi
```

For forms and other clients, see [CSRF](23-csrf.md).

## Spec access from code

`gebweb.routes(app)` ensures controller discovery and returns the route
metadata. Use the builder when exporting a spec for a client-generation or
contract-checking job, including when the HTTP docs routes are disabled:

<!-- doctest:fragment context=app -->
```gb
import gebweb.openapi as openapi;
import json;

let spec = openapi.build(app.info, gebweb.routes(app), app.auth);
io.writeText("openapi.json", json.stringify(spec));
```

Keep this generated file out of hand-edited source. Rebuild it from the same
configured app you deploy, so auth schemes and discovered controllers agree.

## Reference

| API | Purpose |
|-----|---------|
| `gebweb.setInfo(app, info): GebwebApp` | Merge OpenAPI `info` values. |
| `gebweb.app(controllers = [], opts = null): GebwebApp` | Construct the app; `docs`, `openapi` and `swaggerUi` control mounts. |
| `gebweb.useSecurityScheme(app, name, definition, setAsDefault): GebwebApp` | Register a security scheme after configuring authentication. |
| `gebweb.useDocsAuth(app, guard): GebwebApp` | Protect `/docs`, `/openapi.json` and bundled assets. |
| `gebweb.basicAuthGuard(user, password): callable` | Build an HTTP Basic docs guard. |
| `gebweb.bearerTokenGuard(expected): callable` | Build a shared-token docs guard. |
| `gebweb.requireAppAuth(app, requiredRoles = []): callable` | Reuse app authentication, optionally requiring any listed role. |
| `gebweb.openapi.build(info, routes, auth): dict<string, any>` | Build the OpenAPI document from route metadata. |
| `gebweb.openapi.schemaFor(typeName): dict<string, any>` | Map a declared type name to a schema. |

# Parameter binding

The framework inspects each handler's typed parameter list and binds
arguments from the request before calling the method. Bindings are
chosen in priority order:

1. **Path parameter.** A parameter whose name matches a `{name}`
   segment in the route path.
2. **Authenticated user.** When the app has an authenticator
   registered, any parameter whose declared type matches the
   registered user class receives the resolved user (see
   [Authentication](08-auth.md)).
3. **Uploaded files.** A `dict<string, gebweb.UploadedFile>`
   parameter receives the parsed multipart file map (see
   [File uploads](12-file-uploads.md)).
4. **Body.** At most one parameter typed as a user class, or as a
   `list<UserClass>` collection, becomes the JSON request body. The
   framework calls `json.parseAs(body, Class)` to deserialise.
5. **Query parameter.** Any remaining named parameter is read from
   the query string and coerced to its declared type.
6. **Rich request / raw escape hatch.** A single parameter typed
   `gebweb.Request` receives the rich request object (see
   [Responses](04-responses.md)); a single `dict<string, any>`, `dict`,
   or `any` parameter receives the raw request dict unchanged.

## Path parameters

```gb
@Get("/users/{id}/posts/{slug}")
func post(int id, string slug): dict<string, any> {
    return {"id": id, "slug": slug};
}
```

The captured `{id}` is coerced to `int`; a non-numeric path segment
fails with a 400 Bad Request.

## Query parameters

```gb
@Get("/search")
func search(string q, ?int limit, bool fuzzy): list<any> {
    int max = limit ?? 10;
    /* ... */
    return [];
}
```

- `q` is required: missing query parameters on a non-nullable param
  produce a 400.
- `?int limit` is nullable: an absent `limit` query string binds
  `null`.
- Booleans accept `true` / `false` / `1` / `0` / `yes` / `no` (case
  insensitive). Numerics accept the obvious string forms.

## Request body (DTO binding)

A parameter typed as a user class becomes the JSON body:

```gb
class UserCreateDTO {
    string name;
    string email;
    ?int age;
}

@Post("/users")
func create(UserCreateDTO body): dict<string, any> {
    return {"id": "u-1", "name": body.name};
}
```

The framework calls `json.parseAs(rawBody, UserCreateDTO)`, which
populates the data class's fields directly when no explicit
constructor is declared. Parse failures become a 400; validation
failures become a 422 (see [Validation](05-validation.md)).

A `list<UserDTO>` parameter binds an array body:

```gb
@Post("/users/bulk")
func bulkCreate(list<UserCreateDTO> body): dict<string, any> {
    return {"count": body.length()};
}
```

Each element is `json.parseAs`-ed individually.

### Form-encoded bodies

A `Content-Type: application/x-www-form-urlencoded` body (a posted HTML
`<form>`) binds to the same DTO. Each form field is matched to a class
field by name and coerced to that field's declared type (`string`,
`int`, `float`, `bool`); values are URL-decoded first.

```gb
@Post("/signup")
func signup(UserCreateDTO body): dict<string, any> {
    return {"name": body.name};
}
```

A plain `<form method="post" action="/signup">` with `name` and `email`
inputs binds straight to `UserCreateDTO`. Fields absent from the
submission are left unset, and a conversion failure becomes a 400. A
JSON body to the same handler still binds as JSON: the content type
selects the decoder.

## Raw escape hatch

For routes that need full control of the request dict:

```gb
@Post("/raw")
func raw(dict<string, any> request): dict<string, any> {
    return {"method": request["method"], "path": request["path"]};
}
```

Acceptable type names: `dict<string, any>`, `dict`, `any`, or
`Request`. The single-parameter rule applies - handlers with a raw
escape hatch may not declare any other parameters.

## Explicit parameter decorators

The name/type heuristic covers most handlers, but four parameter-
level decorators let you state explicitly where a value comes from
when the heuristic would pick wrong or when you want a different
binding name from the parameter's own identifier:

- `@PathParam("name")` reads from a `{name}` path segment.
- `@QueryParam("name")` reads from the query string.
- `@Body` marks the request body slot.
- `@Header("Header-Name")` reads from a request header
  (case-insensitive lookup).

The argument is optional and defaults to the parameter's own
name; supply one when the public name differs from your code
identifier (e.g. `@PathParam("id") int widgetId`). Path / query /
header values are coerced into the declared type with the same
rules as the heuristic path; missing required values raise
`BadRequestError` (400).

```gb
@Get("/widgets/{id}")
func show(
    @PathParam("id") int widgetId,
    @QueryParam("verbose") ?bool verbose,
    @Header("X-Trace-Id") ?string traceId
): dict<string, any> {
    return {"id": widgetId, "verbose": verbose, "trace": traceId};
}

@Post("/widgets")
func create(@Body WidgetIn payload): dict<string, any> {
    return payload as dict<string, any>;
}
```

Attach `@Description("...")` alongside any of these (or alongside
heuristic-bound parameters) and the text flows into the generated
OpenAPI `description` field for that parameter, so docs explain
each input without hand-authoring the spec.

## Type coercion details

| Declared type    | Accepted form                                 |
|------------------|-----------------------------------------------|
| `string`         | the raw segment / query value                 |
| `int`            | decimal digits, optional leading `-`          |
| `float`          | the usual `1.0`, `2.5e3`, etc.                |
| `bool`           | `true` / `false` / `1` / `0` / `yes` / `no`   |
| `?T` (nullable)  | the above, or absent (binds `null`)           |

Failed coercion throws `BadRequestError("invalid parameter ...")`,
which renders as a 400 problem-details response.

## Reference

- Path parameters: `{name}` segment + matching named handler parameter,
  or any parameter annotated `@PathParam(...)`.
- Query parameters: any non-path, non-body, non-user-injection
  parameter; type-coerced from the query string. `@QueryParam(...)`
  makes the binding explicit and lets the public key differ.
- Body parameter: at most one parameter typed as a user class or
  `list<UserClass>`; deserialised via `json.parseAs`. `@Body` makes
  the binding explicit and overrides the body-shape heuristic.
- Header parameter: any parameter annotated `@Header("Header-Name")`,
  looked up case-insensitively from the request headers.
- Raw escape: single `Request` / `dict<string, any>` / `dict` /
  `any` parameter.
- `@Description("...")` on any parameter populates the OpenAPI
  parameter `description` field; no runtime effect.
- Error responses: type coercion failures and missing required
  parameters produce a 400 Bad Request via the standard problem-details
  shape (see [Responses](04-responses.md)).

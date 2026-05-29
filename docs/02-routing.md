# Routing and decorators

A handler method is registered as a route by annotating it with one of
the HTTP-method decorators. The framework scans the controller class
at app-construction time, finds methods carrying a route decorator,
and registers them against the underlying `web.router`.

## HTTP method decorators

```gb
class WidgetController {
    @Get("/widgets")
    func list(): list<dict<string, any>> { return []; }

    @Get("/widgets/{id}")
    func get(string id): dict<string, any> { return {"id": id}; }

    @Post("/widgets")
    func create(WidgetDTO body): dict<string, any> { return {}; }

    @Put("/widgets/{id}")
    func replace(string id, WidgetDTO body): dict<string, any> { return {}; }

    @Patch("/widgets/{id}")
    func update(string id, WidgetDTO body): dict<string, any> { return {}; }

    @Delete("/widgets/{id}")
    func delete(string id): void { }
}
```

`@Options(path)` and `@Route(method, path)` are also recognised. Use
`@Route` for verbs the convenience decorators don't cover or when the
method is computed:

```gb
@Route("HEAD", "/health")
func health(): void { }
```

## Path parameters

A `{name}` segment in the path becomes a named parameter. The handler
must declare a parameter with the same name; the framework binds the
captured value, coerced to the parameter's declared type:

```gb
@Get("/users/{id}/posts/{slug}")
func post(int id, string slug): dict<string, any> {
    return {"id": id, "slug": slug};
}
```

Type coercion fails are converted into a 400 Bad Request with a
problem-details body explaining the bad parameter.

## Controller-level path prefix

Use the controller class itself as the prefix carrier with `@Route`:

```gb
@Route("/api/v1")
class UserController {
    @Get("/users")
    func list(): list<dict<string, any>> { return []; }
}
```

The class decorator's path is prepended to every route declared on
its methods, so `GET /api/v1/users` matches `list()`.

## Metadata decorators

These don't affect dispatch but show up in the generated OpenAPI
spec:

- `@Summary("text")` - one-line summary on the operation.
- `@Description("text")` - multi-line description.
- `@Tag("name")` - explicit tag (default is the controller class
  name with `Controller` stripped).
- `@OperationId("id")` - custom `operationId` (default is
  `<controller>_<method>`).
- `@Deprecated` - marks the operation as deprecated.
- `@ApiResponse(status, "description", schema: ...)` - additional
  response declarations beyond the inferred 200.

See [OpenAPI and SwaggerUI](13-openapi.md) for the full set.

## Reference

- Route decorators: `@Get(path)`, `@Post(path)`, `@Put(path)`,
  `@Patch(path)`, `@Delete(path)`, `@Options(path)`,
  `@Route(method, path)`.
- Controller prefix: `@Route(prefix)` on the class.
- Metadata: `@Summary(text)`, `@Description(text)`, `@Tag(name)`,
  `@OperationId(id)`, `@Deprecated`, `@ApiResponse(status,
  description, ...)`.
- Path-parameter syntax: `{name}` in the route path; matching
  parameter name in the handler signature.

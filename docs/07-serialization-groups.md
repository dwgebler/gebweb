# Serialization groups

`@Groups` declares which fields of an entity are visible under which
"view". Responses are serialised through the requested groups; request
bodies are deserialised through them too. The same entity can expose
a public summary, a detailed view, and an admin-only field set
without three separate DTO classes.

## Field-level groups

```gb
class User {
    @Groups(["read", "summary"])
    string id;

    @Groups(["read", "summary"])
    string name;

    @Groups(["read", "admin"])
    string email;

    @Groups(["write"])
    string password;
}
```

- The `id` and `name` fields appear in `read` and `summary` views.
- `email` is in `read` and `admin` only - a `summary`-view response
  excludes it.
- `password` is in `write` only - it's accepted on inbound requests
  but never serialised in a response.

A field with no `@Groups` decorator is treated as belonging to a
default unnamed group; it's included whenever no explicit groups are
requested by the caller.

## `@ApiResource` defaults

The cleanest place to wire groups is on the resource:

```gb
@ApiResource("/users", {
    "readGroups": ["read"],
    "writeGroups": ["write"],
})
class User { /* @Groups(...) on fields */ }
```

The auto-generated LIST and GET_ONE responses serialize through
`readGroups`; CREATE / REPLACE / UPDATE deserialize the body through
`writeGroups`.

## Per-route override

For manual handlers, use `@SerializeWith` on the method:

```gb
@Get("/users/{id}/admin")
@SerializeWith(["read", "admin"])
@Auth
@RequiresRole("admin")
func admin(string id): User { /* ... */ }
```

The method's return value is filtered through the listed groups
before JSON serialization.

## Inbound filtering

Write-group filtering happens during body deserialisation: any
incoming field whose group set doesn't intersect `writeGroups` is
silently dropped. This protects against mass-assignment ("client
sends `is_admin: true` and the framework dutifully writes it").

## Reference

- Field decorator: `@Groups(["a", "b"])` - list of group names this
  field belongs to.
- Method decorator: `@SerializeWith(["a", "b"])` - explicit
  read-group list for a single response.
- `@ApiResource` options: `readGroups: list<string>`,
  `writeGroups: list<string>` - defaults applied to every
  auto-generated route.

# Serialization groups

<!-- doctest:strict -->

`@Groups` tags each field of a class with the views it belongs to. One class
can then be sent as a short summary, a full record or an admin view, and can
accept only the fields a client is allowed to write, without a separate class
for each shape.

## Tagging fields

<!-- doctest:run -->
```gb
import gebweb.serialization as serialization;
import io;

class User {
    @Groups("summary", "detail", "admin")
    int id;

    @Groups("summary", "detail", "admin")
    string name;

    @Groups("detail", "admin")
    string email;

    @Groups("admin")
    string role;

    @Groups("write")
    string password;

    string createdAt;

    func User() {
        this.id = 7;
        this.name = "Ada";
        this.email = "ada@example.com";
        this.role = "editor";
        this.password = "secret";
        this.createdAt = "2026-01-01";
    }
}

let user = User();
io.println(serialization.serialize(user, ["summary"]));
io.println(serialization.serialize(user, ["detail"]));
io.println(serialization.serialize(user, ["admin"]));
```

Output:

```text
{"createdAt": "2026-01-01", "id": 7, "name": "Ada"}
{"createdAt": "2026-01-01", "email": "ada@example.com", "id": 7, "name": "Ada"}
{"createdAt": "2026-01-01", "email": "ada@example.com", "id": 7, "name": "Ada", "role": "editor"}
```

- A field is included when one of its groups is among the requested groups.
- A field without `@Groups`, like `createdAt`, belongs to every view.
- `@Groups("a", "b")` and `@Groups(["a", "b"])` mean the same.
- `password` is only in `write`, so no read view ever includes it.

Filtering only happens when groups are requested. A route that returns a
`User` with no group set sends every field, `password` included. Give every
route that returns a class with private or write-only fields an explicit
group set, as below.

## Choosing the view for a route

Put `@Groups` on the handler to filter what it returns:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class User {
    @Groups("summary", "detail")
    int id;

    @Groups("summary", "detail")
    string name;

    @Groups("detail")
    string email;

    @Groups("write")
    string password;

    func User(int id, string name) {
        this.id = id;
        this.name = name;
        this.email = "${name}@example.com";
        this.password = "secret";
    }
}

@Controller("/users")
class UserController extends gebweb.Controller {
    @Get("")
    @Groups("summary")
    func list(): list<User> {
        return [User(1, "ada"), User(2, "lin")];
    }

    @Get("/{id}")
    @Groups("detail")
    func show(int id): User {
        return User(id, "ada");
    }
}

let client = gebweb.TestClient(gebweb.app());
io.println(client.get("/users").text());
io.println(client.get("/users/1").text());
```

Output:

```text
[{"id":1,"name":"ada"},{"id":2,"name":"lin"}]
{"email":"ada@example.com","id":1,"name":"ada"}
```

Handler groups apply to returned class instances, including each instance in
a returned list, and work with `@Produces` for CSV and XML too. A plain dict
has no field tags and is sent as is. So is a response the handler builds
itself (`this.json(user)`): return the instance and let Gebweb encode it when
the groups should apply.

## Write groups and resources

On an `@ApiResource`, `readGroups` choose the view used for responses and
`writeGroups` choose the fields a request body may set. Fields outside the
write groups are ignored when the body is applied, which stops a client from
setting fields it should not control:

<!-- doctest:run -->
```gb
import gebweb;
import gebweb.repository as repository;
import io;
import store;

@ApiResource("/accounts", {"readGroups": ["read"], "writeGroups": ["write"]})
class Account {
    @Groups("read")
    string id;

    @Groups("read", "write")
    string name;

    @Groups("read")
    string role;

    static func repositoryClass(): any {
        return AccountRepository;
    }
}

class AccountRepository implements repository.Repository<Account> {
    store.Store rows = store.Store();

    func find(string id): ?Account {
        return this.rows.get(id) as ?Account;
    }

    func list(repository.Page page): list<Account> {
        return [];
    }

    func save(Account a): Account {
        if (a.id == null || a.id == "") {
            a.id = "a1";
            a.role = "member";
        }
        this.rows.set(a.id, a);
        return a;
    }

    func delete(string id): void {}
}

let client = gebweb.TestClient(gebweb.app([Account]));
io.println(client.post("/accounts", {"name": "Ada", "role": "admin", "id": "x"}).text());
io.println(client.request("PATCH", "/accounts/a1", {"role": "admin"}, {}).text());
```

Output:

```text
{"id":"a1","name":"Ada","role":"member"}
{"id":"a1","name":"Ada","role":"member"}
```

The client's `role` and `id` were ignored on create and on update, because
only `name` is in the `write` group. Without options, resources read through
`["read"]` and write through `["write"]`; untagged fields are read and written
in every group. See [Repositories](06-repositories.md).

## Groups outside a request

`gebweb.serialization` exposes the same filtering for code that builds
payloads itself, such as a job writing to a queue:

| Function | Effect |
|----------|--------|
| `serialization.serialize(instance, groups)` | A dict of the fields visible in `groups`; an empty list means every field. |
| `serialization.applyInput(instance, data, groups)` | Sets fields from `data`, skipping those outside `groups`; returns the instance. |
| `serialization.handlerGroups(handler)` | The groups declared on a handler. |

## Reference

- `@Groups("a", "b")` or `@Groups(["a", "b"])` on a field: the views it
  belongs to; untagged fields belong to every view.
- `@Groups(...)` on a handler: the view applied to returned instances.
- `@ApiResource(path, {"readGroups": [...], "writeGroups": [...]})`: defaults
  `["read"]` and `["write"]`.
- `import gebweb.serialization as serialization` for `serialize`,
  `applyInput` and `handlerGroups`.

# Tasks demo

A JWT-authed task manager that exercises most of the Gebweb feature
set in one example:

- Owner-scoped CRUD on `Task` entities (one user can't see or
  mutate another user's tasks).
- Admin-only cross-user view via `@RequiresRole("admin")`.
- Multipart file uploads as task attachments via
  `dict<string, UploadedFile>` parameter binding.
- OpenAPI 3.1 spec + SwaggerUI at `/openapi.json` and `/docs`.
- In-process `TestClient` suite covering every route.

## Files

- [`tasksapp.gb`](tasksapp.gb) - the framework module. Declares the
  entities (`Task`, `Attachment`), repositories (`TaskRepo`,
  `AttachmentRepo`), DTOs, controllers, and a `build()` factory that
  wires a fresh `GebwebApp`. Two callers consume `build()`:
  `main.gb` for the real server, `main_test.gb` for the
  `TestClient` suite.
- [`main.gb`](main.gb) - runnable entry point. Starts a real HTTP
  server on `127.0.0.1:8080`.
- [`main_test.gb`](main_test.gb) - test suite. 17 tests across five
  test classes covering login, CRUD, cross-user isolation, admin
  gating, attachment upload, and the generated OpenAPI spec.

## Running

```sh
# Start the demo server.
geblang examples/tasks/main.gb

# In another shell, mint a JWT for the seeded user "ada":
curl -XPOST localhost:8080/login \
     -H 'Content-Type: application/json' \
     -d '{"username":"ada"}'

# Use the returned token on the protected endpoints:
TOKEN=...
curl localhost:8080/tasks -H "Authorization: Bearer $TOKEN"
curl localhost:8080/tasks -XPOST \
     -H "Authorization: Bearer $TOKEN" \
     -H 'Content-Type: application/json' \
     -d '{"title":"Write the manual"}'

# Upload an attachment for a task you own:
curl localhost:8080/tasks/t-1/attachments -XPOST \
     -H "Authorization: Bearer $TOKEN" \
     -F file=@./notes.md

# User "carla" also has the admin role - try the admin view:
ADMIN=$(curl -s -XPOST localhost:8080/login \
        -H 'Content-Type: application/json' \
        -d '{"username":"carla"}' | jq -r .token)
curl localhost:8080/admin/tasks -H "Authorization: Bearer $ADMIN"
```

`http://localhost:8080/docs` serves the auto-generated SwaggerUI page.

## Running the test suite

```sh
geblang test examples/tasks/main_test.gb
```

The suite drives the same app through `gebweb.TestClient` so it
runs in-process without binding a port.

## What this example demonstrates

| Feature | Where |
|---------|-------|
| `@Auth` class-level gating | `TaskController`, `AttachmentController` |
| `@RequiresRole("admin")` method gating | `AdminTaskController` |
| JWT authenticator + `gebweb.jwtIssue` / `jwtVerify` | `bearerAuthenticator`, `LoginController` |
| Constructor-injected dependencies | `TaskController(TaskRepo)`, `AttachmentController(...)` |
| User-injection by parameter type | `CurrentUser user` arguments |
| Owner-scoped store helpers | `TaskRepo.listForOwner`, `findForOwner` |
| Multipart file uploads | `AttachmentController.upload(dict<string, UploadedFile> files)` |
| HTTP exception → Problem Details | `throw gebweb.notFound("...")` |
| Auto-generated OpenAPI 3.1 + SwaggerUI | `gebweb.setInfo(...)` + the route decorators |

For the framework's full feature surface, see the
[manual](../../docs/00-index.md).

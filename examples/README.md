# Gebweb examples

Runnable example applications, ordered roughly from simplest to
broadest. Each one runs with `geblang <path>` from the dist root,
or `geblang main.gb` inside the example's own directory (for the
multi-file ones).

## Single-file examples

- [`hello.gb`](hello.gb) - minimal three-endpoint hello world
  with decorator-driven routing, typed parameter binding,
  automatic OpenAPI 3.1, SwaggerUI, and `HttpException` handling.
- [`hello_ctrl.gb`](hello_ctrl.gb) - same surface as `hello.gb`
  but written as a controller class extending `gebweb.Controller`
  to demonstrate the helper-method style (`this.json`,
  `this.notFound`, `this.redirect`).
- [`hello_test.gb`](hello_test.gb) - exercises the hello-world
  controller through the in-process `TestClient`. Runs without
  binding a network port; good for CI smoke checks.
- [`widgets.gb`](widgets.gb) - full CRUD via `@ApiResource` +
  `Repository<T>`. Six auto-generated routes from one decorator.
- [`auth.gb`](auth.gb) - `@Auth` + `@RequiresRole` walkthrough
  with a bearer-token authenticator and user injection.
- [`responses.gb`](responses.gb) - HTML, file download, and
  streaming routes (`gebweb.html`, `gebweb.file`,
  `gebweb.stream`).

## Multi-file examples

- [`tasks/`](tasks/) - JWT-authed task manager. Owner-scoped CRUD,
  cross-user isolation, admin role gating, multipart file uploads
  as task attachments. Ships with a 17-test `TestClient` suite.
- [`chat/`](chat/) - single-room chat application. Shows that one
  Gebweb app can serve HTTP routes and WebSocket connections on
  the same port, sharing a DI-injected broadcast hub.
- [`server_rendered_blog/`](server_rendered_blog/) - in-memory
  blog exercising every server-rendered UX feature: CSRF, flash
  messages, form-state rehydration, fingerprinted assets, and
  CSP nonces.
- [`feature_tour/`](feature_tour/) - small task-manager app
  exercising async work (background jobs, scheduler, event bus,
  mailer), file storage, query DSL, cursor pagination,
  API-key auth, and `@RequiresPermission`. Useful as a
  one-stop reference for the integrations surface.

## Running

```sh
geblang examples/hello.gb           # start the server
geblang examples/hello_test.gb      # in-process smoke
```

Multi-file examples have their own README inside the directory.

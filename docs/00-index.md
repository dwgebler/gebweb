# Gebweb Manual

Gebweb is an opinionated web framework written entirely in Geblang on top
of `stdlib/web/*`. It targets developers who'd otherwise reach for
FastAPI, Symfony, or API Platform: a typed, decorator-driven workflow
with automatic OpenAPI 3.1 + SwaggerUI, the repository pattern,
constructor-injection DI, JWT / session auth, response caching, and
streaming primitives.

This manual is reference-style. Each chapter opens with a short
overview, walks through the feature with code, and ends with a
"Reference" subsection listing every helper signature relevant to the
topic. Read top-to-bottom for a guided tour or jump straight to the
chapter you need.

## Status

Gebweb is at version 1.0.0. The [CHANGELOG](../CHANGELOG.md) lists
every feature shipped in the 1.0 release; this manual documents how
to use them.

## Chapters

1. [Getting started](01-getting-started.md) - install, hello-world,
   request lifecycle.
2. [Routing and decorators](02-routing.md) - `@Get` / `@Post` /
   `@Route`, path parameters, controller-level prefix.
3. [Parameter binding](03-parameter-binding.md) - path, query, body,
   raw escape hatch, type coercion.
4. [Responses](04-responses.md) - automatic JSON wrapping,
   `gebweb.html` / `gebweb.file` / `gebweb.stream`, HTTP exceptions.
5. [Validation](05-validation.md) - `@Assert.*` rules, custom
   validators, 422 Problem Details responses.
6. [Repositories and `@ApiResource`](06-repositories.md) - the
   `Repository<T>` interface, DI container, auto-CRUD.
7. [Serialization groups](07-serialization-groups.md) - `@Groups`,
   read / write filters, `@ApiResource` defaults.
8. [Authentication and roles](08-auth.md) - `useAuthenticator`,
   `useSessionAuth`, JWT helpers, `@Auth`, `@RequiresRole`, user
   injection.
9. [Middleware](09-middleware.md) - `cors`, `securityHeaders`,
   `requestId`, `requestLog`, `compress`, `rateLimit`, `use` /
   `before` / `after`.
10. [Caching](10-caching.md) - `useCacheStore`,
    `@Cache(ttl, vary)`, store options.
11. [WebSockets and SSE](11-websockets-and-sse.md) - `@WebSocket`,
    `@Sse`, streaming responses.
12. [File uploads](12-file-uploads.md) - multipart parsing,
    `UploadedFile`, `dict<string, UploadedFile>` parameter binding.
13. [OpenAPI and SwaggerUI](13-openapi.md) - auto-generated spec,
    decorators that shape it, security schemes, SwaggerUI mount.
14. [Testing](14-testing.md) - `TestClient`, in-process dispatch,
    fixtures, integration with `test.Test`.
15. [The `gebweb` CLI](15-cli.md) - `new`, `dev`, `build`,
    `routes`, `generate`, `migrate`.
16. [Plugins](16-plugins.md) - `gebweb.plugin`, custom extensions,
    sibling packages.
17. [Database migrations](17-migrations.md) - `gebweb migrate`,
    versioned SQL files, sqlite / postgres / mysql.
18. [Background jobs](18-background-jobs.md) - `@Job` handlers,
    `gebweb.enqueue`, `gebweb worker`, retry/backoff, `@Scheduled`
    cron + leader election.
19. [Events](19-events.md) - `@On` subscribers, `gebweb.publish`,
    synchronous in-process pub/sub.
20. [Mailer](20-mailer.md) - `gebweb.Mailable`, SMTP / memory /
    log transports, sync + async send via the worker.
21. [Storage](21-storage.md) - `gebweb.put` / `get` / etc.,
    memory + local-disk backends, `UploadedFile.saveToStorage`.
22. [Message brokers](22-messaging.md) - `@OnMessage` handlers,
    `gebweb.useMessageQueue` / `useMessageTopic`,
    `gebweb.runMessageWorker` for RabbitMQ / STOMP / SQS / Kafka.
23. [CSRF protection](23-csrf.md) - `gebweb.useCsrf`,
    `@CsrfExempt`, the `{{ csrf }}` view variable, token cookies.
24. [Flash messages](24-flash.md) - `gebweb.flash`, session-backed
    one-shot category-grouped messages rendered as `{{ flashes }}`.
25. [Forms and rehydration](25-forms.md) - submitted body + per-
    field error map preserved across redirect after validation
    failure, content-negotiated 422 for JSON clients.
26. [Asset pipeline](26-assets.md) - `gebweb.useStaticAssets`,
    content-hash manifest, the `asset` view filter, dev vs prod
    mode.
27. [Security headers](27-security.md) - `gebweb.useSecurity`,
    default header set, CSP with per-request nonce expansion,
    HSTS opt-in.

## Conventions

- All code samples assume `import gebweb;` at the top of the file.
- Sub-modules are imported with their natural alias when needed
  (e.g. `import gebweb.errors as errors;` for `instanceof
  HttpException` checks).
- "User class" means any class declared in the application's own
  code - typically a DTO, a domain entity, or the user-injection
  type the authenticator returns.

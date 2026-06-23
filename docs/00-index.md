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

Gebweb is at version 1.6.1. The `CHANGELOG.md` in the source tree
lists every feature shipped to date; this manual documents how to
use them.

## For AI agents

A condensed cheatsheet for AI coding agents working in Gebweb
apps lives at [AGENTS.md](AGENTS.md). It's denser than this
manual, focused on idioms and common pitfalls, and intended to be
read once at the start of a session. Point your agent at that
file (and at the Geblang language `AGENTS.md` in the language
docs) before asking it to edit code.

## Chapters

1. [Getting started](01-getting-started.md) - install, hello-world,
   request lifecycle.
2. [Routing and decorators](02-routing.md) - `@Get` / `@Post` /
   `@Route`, path parameters, controller-level prefix.
3. [Parameter binding](03-parameter-binding.md) - path, query, body,
   raw escape hatch, type coercion.
4. [Responses](04-responses.md) - automatic JSON wrapping,
   `gebweb.html` / `gebweb.file` / `gebweb.stream`, HTTP exceptions.
4a. [Views and templates](04a-views.md) - Twig-style templates,
   inheritance, filters, context injection.
5. [Validation](05-validation.md) - `@Assert.*` rules, custom
   validators, 422 Problem Details responses.
6. [Repositories and `@ApiResource`](06-repositories.md) - the
   `Repository<T>` interface and auto-CRUD wiring.
7. [Dependency injection](06a-dependency-injection.md) - the
   container, lifecycle model, autowiring rules, `@Param`,
   interface-typed parameters, testing patterns.
8. [Serialization groups](07-serialization-groups.md) - `@Groups`,
   read / write filters, `@ApiResource` defaults.
9. [Authentication and roles](08-auth.md) - `useAuthenticator`,
   `useSessionAuth`, JWT helpers, `@Auth`, `@RequiresRole`, user
   injection.
10. [Middleware](09-middleware.md) - `cors`, `securityHeaders`,
    `requestId`, `requestLog`, `compress`, `rateLimit`, `use` /
    `before` / `after`.
11. [Caching](10-caching.md) - `useCacheStore`,
    `@Cache(ttl, vary)`, store options.
12. [WebSockets and SSE](11-websockets-and-sse.md) - `@WebSocket`,
    `@Sse`, streaming responses.
13. [File uploads](12-file-uploads.md) - multipart parsing,
    `UploadedFile`, `dict<string, UploadedFile>` parameter binding.
14. [OpenAPI and SwaggerUI](13-openapi.md) - auto-generated spec,
    decorators that shape it, security schemes, SwaggerUI mount.
15. [Testing](14-testing.md) - `TestClient`, in-process dispatch,
    fixtures, integration with `test.Test`.
16. [The `gebweb` CLI](15-cli.md) - `new`, `dev`, `build`,
    `routes`, `generate`, `migrate`.
17. [Plugins](16-plugins.md) - `gebweb.plugin`, custom extensions,
    sibling packages.
18. [Database migrations](17-migrations.md) - `gebweb migrate`,
    versioned SQL files, sqlite / postgres / mysql.
19. [Background jobs](18-background-jobs.md) - `@Job` handlers,
    `gebweb.enqueue`, `gebweb worker`, retry/backoff, `@Scheduled`
    cron + leader election.
20. [Events](19-events.md) - `@On` subscribers, `gebweb.publish`,
    synchronous in-process pub/sub.
21. [Mailer](20-mailer.md) - `gebweb.Mailable`, SMTP / AWS SES /
    memory / log transports, sync + async send via the worker.
22. [Storage](21-storage.md) - `gebweb.put` / `get` / etc.,
    memory + local-disk + S3 backends, `UploadedFile.saveToStorage`.
23. [Message brokers](22-messaging.md) - `@OnMessage` handlers,
    `gebweb.useMessageQueue` / `useMessageTopic`,
    `gebweb.runMessageWorker` for RabbitMQ / STOMP / SQS / SNS /
    Kafka.
24. [CSRF protection](23-csrf.md) - `gebweb.useCsrf`,
    `@CsrfExempt`, the `{{ csrf }}` view variable, token cookies.
25. [Flash messages](24-flash.md) - `gebweb.flash`, session-backed
    one-shot category-grouped messages rendered as `{{ flashes }}`.
26. [Forms and rehydration](25-forms.md) - submitted body + per-
    field error map preserved across redirect after validation
    failure, content-negotiated 422 for JSON clients.
27. [Asset pipeline](26-assets.md) - `gebweb.useStaticAssets`,
    content-hash manifest, the `asset` view filter, dev vs prod
    mode.
28. [Security headers](27-security.md) - `gebweb.useSecurity`,
    default header set, CSP with per-request nonce expansion,
    HSTS opt-in.
29. [LLM integration](28-llm.md) - `gebweb.useLlm`,
    `gebweb.llmClient`, provider-agnostic chat / embed / image
    surface across OpenAI, Anthropic, and AWS Bedrock.
30. [services.yaml](29-services-yaml.md) - `config/services.yaml`
    for parameters, services entries, interface bindings, tags,
    per-env overlays, and the encrypted secrets vault plus
    `gebweb secrets` CLI.
31. [Internationalisation](30-i18n.md) - locale negotiation,
    YAML catalogs, plural-aware lookup, validation localisation,
    locale-aware number and date formatting.
32. [Multi-tenancy](31-multi-tenancy.md) - tenant resolver,
    typed-parameter injection, `tenant_id` stamping and
    query-scoping helpers.
33. [Health checks](32-health-checks.md) - `@HealthCheck`
    decorator, `/healthz` and `/readyz` endpoints, per-probe
    timeout and aggregation.
34. [OIDC and OAuth2 sign-in](33-oidc.md) - Google, GitHub, and
    generic OIDC providers, authorization-code-with-PKCE,
    automatic session establishment.
35. [Outbound webhooks](34-webhooks.md) - subscription registry,
    HMAC-signed delivery through the job queue, configurable
    retry and dead-letter handling.
36. [Concurrency and shared state](35-concurrency.md) - the
    request-per-goroutine model, what is shared vs per-request,
    and using `store.Store` for safe shared mutable state.
37. [Dev profiler bar](36-profiler-bar.md) - `useProfilerBar`,
    the collapsible toolbar injected into HTML responses, time /
    memory / request panels, and the `recordTiming` timeline.
38. [Deployment](37-deployment.md) - `gebweb.cli` standard server
    entrypoint: flag/env/opts precedence, LetsEncrypt and
    self-signed HTTPS, graceful drain on SIGINT/SIGTERM.

## Conventions

- All code samples assume `import gebweb;` at the top of the file.
- Sub-modules are imported with their natural alias when needed
  (e.g. `import gebweb.errors as errors;` for `instanceof
  HttpException` checks).
- "User class" means any class declared in the application's own
  code - typically a DTO, a domain entity, or the user-injection
  type the authenticator returns.

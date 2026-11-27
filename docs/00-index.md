# Gebweb Manual

<!-- doctest:strict -->

Gebweb is a typed, decorator-driven web framework written in Geblang. It
combines routing, constructor injection, validation, OpenAPI, HTML views,
authentication and background work. This manual provides runnable applications
and focused API references.

## Status

This manual documents Gebweb 1.11.1 with Geblang 1.47.0 or later. Consult the
framework's `CHANGELOG.md` for release changes. The framework CLI and the package
imported by an application are separate installations.

## Start here

1. [Install Gebweb](01-getting-started.md#install-gebweb) and create a project.
2. [Build a first API](01-getting-started.md#a-first-api) with a controller and
   constructor-injected service.
3. [Understand app construction](01-getting-started.md#app-construction) before
   choosing discovered or explicit controllers.
4. [Test the app](14-testing.md) and [build it for deployment](37-deployment.md).

[Build a bookmarks app](38-building-bookmarks.md) takes a SQLite application
from an empty project through HTML forms, CSRF, sessions, JSON routes, tests
and an embedded standalone binary. [Building authentication](40-authentication.md)
adds a complete password-login and JWT verification flow. Both have checked-out
source under `examples/`.

Use [Application best practices](39-best-practices.md) when deciding service
lifetimes, shared-state ownership, typed boundaries and resource cleanup.
Read [Security](27-security.md) and the dedicated
[Web application firewall](27a-waf.md) chapter before configuring a public app.

## For AI agents

[AGENTS.md](AGENTS.md) is a compact framework guide with checked examples,
startup conventions, API links and common pitfalls. Pair it with the Geblang
language guide when asking an agent to edit an application. Framework APIs
belong to this manual; language syntax and runtime primitives belong to the
Geblang manual.

## Chapters

Chapter filenames are stable URLs. The lettered chapters extend the neighboring
topic without renumbering existing links.

| Chapter | Covers |
|---------|--------|
| [Getting started](01-getting-started.md) | Installation, a first API and application construction. |
| [Routing and decorators](02-routing.md) | HTTP methods, prefixes, path parameters and route metadata. |
| [Parameter binding](03-parameter-binding.md) | Typed path, query, body and header inputs. |
| [Responses](04-responses.md) | Typed JSON output, response builders, HTML, files and streams. |
| [Views and templates](04a-views.md) | Templates, inheritance, filters, escaping and context injection. |
| [Validation](05-validation.md) | Field assertions, custom validators and failure responses. |
| [Repositories and `@ApiResource`](06-repositories.md) | Repository contracts, generated CRUD routes and pagination. |
| [Dependency injection](06a-dependency-injection.md) | Constructor wiring, lifetimes, interfaces, parameters and test dependencies. |
| [Database connections and concurrency](06b-database-connections.md) | Pool ownership, sizing, admission limits, workers and shutdown. |
| [Serialization groups](07-serialization-groups.md) | Read/write field filtering and resource defaults. |
| [Authentication, JWTs and authorization](08-auth.md) | Authenticators, user injection, signed tokens, roles and policies. |
| [Middleware](09-middleware.md) | Request/response hooks, ordering, compression, rate limits and timing. |
| [Caching and idempotency](10-caching.md) | Response stores, locale-aware keys, tags, invalidation and idempotent writes. |
| [WebSockets and SSE](11-websockets-and-sse.md) | Upgrades, broadcasts, streaming and connection ownership. |
| [File uploads](12-file-uploads.md) | Multipart inputs, typed uploads and storage boundaries. |
| [OpenAPI and SwaggerUI](13-openapi.md) | Generated schemas, metadata, protected docs and interactive requests. |
| [Testing](14-testing.md) | TestClient, cookies, multipart bodies, client identity and both runtimes. |
| [The `gebweb` CLI](15-cli.md) | Project creation, dev, build, routes, generators, migrations, workers and secrets. |
| [Plugins](16-plugins.md) | Typed plugins, setup order and application extension points. |
| [Database migrations](17-migrations.md) | Versioned SQL with SQLite, PostgreSQL and MySQL. |
| [Background jobs](18-background-jobs.md) | Durable jobs, retry policy, workers, schedules and ownership. |
| [Events](19-events.md) | Synchronous subscribers, lifecycle hooks and error handling. |
| [Mailer](20-mailer.md) | Typed mail, test transports, provider setup and queued delivery. |
| [File storage](21-storage.md) | Memory, disk and S3 backends, uploads and streaming reads. |
| [Message brokers](22-messaging.md) | Queue/topic configuration, handlers, workers and delivery semantics. |
| [CSRF protection](23-csrf.md) | Token cookies, form fields, request headers and route exemptions. |
| [Flash messages](24-flash.md) | Session-backed messages, response-aware updates and one-shot rendering. |
| [Forms](25-forms.md) | Validation redirects, field errors, old input and multipart text fields. |
| [Asset pipeline](26-assets.md) | Fingerprints, manifests, templates, reload modes and root containment. |
| [Security](27-security.md) | Browser headers, CSP, HSTS, request limits and related security guides. |
| [Web application firewall](27a-waf.md) | Rules, exclusions, body limits, client identity, logging and bans. |
| [LLM integration](28-llm.md) | Provider configuration, typed results, test backends and request ownership. |
| [services.yaml: parameters, services, bindings, secrets](29-services-yaml.md) | Configuration markers, environment overlays, DI and the secrets vault. |
| [Internationalisation](30-i18n.md) | Locale negotiation, catalogs, translation helpers, formatting and caching. |
| [Multi-tenancy](31-multi-tenancy.md) | Trusted tenant resolution, injection and repository scoping. |
| [Health checks](32-health-checks.md) | Liveness/readiness, the ops bundle, metrics and graceful drain. |
| [OIDC and OAuth2 sign-in](33-oidc.md) | Provider configuration, PKCE callbacks, claim resolution and sessions. |
| [Outbound webhooks](34-webhooks.md) | Subscriptions, signed jobs, retry thresholds and receiver verification. |
| [Concurrency and shared state](35-concurrency.md) | Shared lifetimes, Store operations, snapshots and transaction limits. |
| [Dev profiler bar](36-profiler-bar.md) | Environment gating, timing entries, process measurements and CSP nonces. |
| [Deployment: the standard server entrypoint](37-deployment.md) | CLI options, HTTPS, trusted proxies, logging and shutdown. |
| [Build a bookmarks app](38-building-bookmarks.md) | Complete HTML/JSON application, CSRF-on tests and standalone builds. |
| [Application best practices](39-best-practices.md) | Typed boundaries, dependency lifetimes, ownership and verification. |
| [Building authentication](40-authentication.md) | Password login, JWT verification, account lookup and key management boundaries. |

## Conventions

- Complete examples declare their imports. Named files belong at the paths
  shown; runnable checks use the earlier files from that chapter.
- Public handlers use typed DTOs and output classes. HTML/file helpers return
  shaped dictionaries; native HTTP builders return `http.Response`.
- Use constructor injection and register infrastructure before discovery.
  The standard entry point checks CLI arguments before opening resources.
- Run application tests on both runtimes. Use real HTTP checks for server,
  proxy, cache and standalone-build behavior that in-process tests cannot prove.

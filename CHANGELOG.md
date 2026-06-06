# Gebweb changelog

## 1.3.0

### Asset pipeline and bundling

- `gebweb build` now compiles, minifies, and embeds assets and templates into
  the single-binary release. Declare an `assets:` block in `geblang.yaml`
  (`sourceDir`, `outDir`, `entryPoints`, optional `templatesDir`/`publicDir`)
  and the build bundles JS/TS/JSX/CSS via esbuild, compiles SASS via dart-sass,
  minifies HTML templates (preserving `{{ }}` and `{% %}`), and embeds the
  compiled output, templates, and public files. The binary is self-contained.
- The view engine and static-asset handler are bundle-aware: a built binary
  serves the embedded copies (resolved via `sys.bundleDir()`), while `gebweb dev`
  and plain `geblang` runs serve from disk unchanged. Application code is
  identical in both cases.
- `gebweb build --no-minify` skips minification; `--no-sass` skips SASS
  compilation when dart-sass is absent (otherwise a `.scss` entry with no
  dart-sass fails the build with an actionable message).
- `gebweb dev` compiles the asset entry points once (unminified) before starting
  so the dev server serves them from disk.

### Offline SwaggerUI

- `gebweb build` downloads the pinned SwaggerUI assets once (cached under the
  user cache dir), embeds them, and serves them from local `/docs/...` routes,
  so the built binary's API docs work offline with no CDN dependency. Dev still
  loads SwaggerUI from the CDN. `--no-swagger` skips embedding.

### Docker and Compose generation

- `gebweb docker` generates a `Dockerfile` and `compose.yaml`; `gebweb build
  --docker` builds the binary first, then generates them. The Dockerfile copies
  the host-built static binary into a distroless image and wires `GEBWEB_PORT`.
- `compose.yaml` runs the app with the port and `.env` wired in, plus an
  optional database service for the chosen `--db`: sqlite (named volume, no DB
  service), postgres, pgvector (`pgvector/pgvector:pg16`), or mysql, each with a
  healthcheck and named volume.
- Generation never overwrites an existing `Dockerfile` / `compose.yaml` without
  `--force`.

### Project wizard

- `gebweb new` is now an interactive wizard: it prompts for project name, type
  (app vs API), database (sqlite/postgres/pgvector/mysql), Docker, and port,
  with flags and `--yes` for non-interactive/CI use. It scaffolds a buildable
  entry module, `.env`, a sample controller + model + repository, a TestClient
  suite, a `.gitignore`, and (for app) a template plus a CSS/TS asset wired
  through the asset pipeline; `--docker` also emits the Docker files.
- `gebweb build` now passes the entry as a module name to `geblang build`
  (previously a file path, which failed to resolve), so building a project
  works. Scaffolded entries are `module` files that `export func main`, run
  directly under `gebweb dev`, and build with `gebweb build`.

## 1.2.0

The HTTP layer release: rich Request and Response objects everywhere, plus
TLS / mutual TLS / trusted proxies exposed through `gebweb.serve`. This line
has breaking changes to the request/response contract.

### Rich Request and Response (breaking)

- Handlers receive a `gebweb.Request` object: `req.method()`, `req.path()`,
  `req.scheme()`, `req.isSecure()`, `req.host()`, `req.clientIp()`,
  `req.clientCert()`, `req.header(name)`, `req.cookie(name)`, typed query
  getters (`req.query`, `req.queryInt`, `req.queryBool`, `req.queryAll`),
  `req.isJson()`, `req.text()`, `req.json()`, route params
  (`req.routeParam(name)`, `req.routeParams()`), plus framework context
  (`req.locale`, `req.tenant`, `req.user`, `req.csrfToken`, `req.cspNonce`). It
  stays index-compatible (`req["headers"]`) for migration.
  Declare a handler parameter as `gebweb.Request` to receive it.
- Controller response builders (`json`/`html`/`text`/`created`/`accepted`/
  `noContent`/`redirect`/`problem`/`view`/`partial`/`back`) return a
  `Response`; handlers may also return one directly (`http.response(body,
  status)`, `http.jsonResponse(value, status)`, `http.redirect(url, status)`).
  Response header names are canonicalized.
- Middleware (`before`/`use`/`after`) receive a `gebweb.Request` and (for
  response-phase) a `Response`; the built-in factories (`cors`,
  `securityHeaders`, `requestId`, `requestLog`, `compress`, `rateLimit`) and
  the framework's own middleware were rewritten onto the objects.
- Authenticators (`useAuthenticator`/`useApiKeyAuth`/`useSessionAuth`), the
  docs guards, and `oidcLoginUrl` now receive a `gebweb.Request`.

### TLS and deployment

- `gebweb.serve`/`gebweb.listen` forward TLS options to the server: native
  HTTPS (`tls: {cert, key}` or `selfSigned`), automatic certificates
  (`tls: {autoCert: "host"}`, ACME / Let's Encrypt), HTTP/2 over TLS, and
  mutual TLS (`tls: {clientCa, clientAuth}`) with the verified peer cert on
  `req.clientCert()`.
- `trustedProxies` server option makes `req.clientIp()`/`scheme()`/`host()`
  honour `X-Forwarded-*` only from trusted peers.

### API versioning

- `@ApiVersion("v2")` on a controller prefixes its routes with `/v2` and groups
  them under a `v2` tag in OpenAPI; a method-level `@ApiVersion` overrides the
  controller's. Routes without it are unchanged.
- `@Deprecated` (on a method or controller) emits a `Deprecation: true` response
  header; `@Deprecated("<date>")` also emits a `Sunset` header and an `x-sunset`
  field on the OpenAPI operation.

### Concurrency

- gebweb is now safe under concurrent load (the production VM serves each
  request on its own goroutine). The DI container's per-request scope and
  cycle-detection state are keyed per goroutine, the singleton cache uses
  double-checked locking, and the view-engine template cache is mutex-guarded;
  the rate limiter and `MemoryStorage` moved to a thread-safe `store.Store`
  (atomic token bucket), and `MemoryMailer` is mutex-guarded. Previously a
  gebweb app could crash under concurrent requests (`concurrent map writes`).
  Share mutable state across requests through a `store.Store` or a backing
  store, not a plain captured container.

### Internal

- gebweb's own outbound calls (SSO, webhooks) and the generated OpenAPI client
  use the request builder and the rich `Response`.

## 1.1.1

### Fixes

- `broadcast.Hub` is now goroutine-safe. The subscriber set is guarded by a
  mutex, so concurrent connection handlers can `join`, `leave`, and
  `broadcast` at once without racing; broadcasts snapshot the subscribers and
  send outside the lock, so a slow connection never blocks membership changes.

## 1.1.0

Production essentials. The 1.1 line fills in the gaps a 1.0 SaaS
app routinely needed external libraries for: localisation,
multi-tenancy, health probes, SSO, outbound webhooks, soft
delete, response caching, and timing observability.

### Internationalisation (i18n)

- `gebweb.useI18n(app, opts)` mounts a request-phase middleware
  that resolves the active locale from URL prefix (`/de/...`),
  sticky cookie, and `Accept-Language` header. The matched URL
  prefix is stripped before route matching so routes stay
  locale-agnostic.
- `gebweb.Locale` is injected into handlers as a typed parameter.
- `gebweb.Translator` is injected the same way once a catalog is
  loaded. Catalogs are YAML files at `messages/<tag>.yaml` with
  nested keys flattened to dotted strings (`auth.login.title`).
- `gebweb.t(app, locale, key, args)` is the programmatic surface;
  templates get a `t(key, args)` view helper auto-bound to the
  request locale.
- Pluralisation: catalog entries with `one` / `other` (CLDR
  variants `zero`, `two`, `few`, `many` per locale) are selected
  by the `count` arg. Hand-coded rules for en, de, fr, es, it,
  pt, ru, pl, ar, zh, ja, ko; unknown languages fall back to
  English.
- Validation error localisation: `@Assert.*` failures are
  translated through `validation.<code>` keys with `{field}` and
  rule-specific params; the raw English message stays as the
  fallback.
- Locale-aware number and date formatting via
  `i18n.formatNumber`, `i18n.numberSeparators`, `i18n.formatDate`
  (short and long styles).

### Multi-tenancy

- `gebweb.useTenant(app, resolver, opts)` mounts a request-phase
  middleware that resolves the active tenant via a user-supplied
  callable. `opts.required` (default true) short-circuits with a
  400 Problem Details when resolution returns null.
- `gebweb.Tenant` is injected into handlers as a typed parameter.
- Helpers for shared-schema tenancy: `gebweb.stampTenant(entity,
  tenant)` writes `entity.tenant_id`, `gebweb.tenantOwns(entity,
  tenant)` guards reads, and `gebweb.scopedQuery(query, tenant)`
  appends `tenant_id = ?` to a query.Query.

### Health checks

- `gebweb.useHealth(app, instances, opts)` mounts `/healthz`
  (liveness) and `/readyz` (readiness) endpoints and discovers
  every `@HealthCheck(name, kind, timeout)` method on the
  supplied instances.
- Each probe runs under a per-probe timeout (default 5 s);
  failures and time-outs aggregate into a 503 JSON response with
  per-probe status and duration.

### Security headers and CSP nonce

- The 1.0 `gebweb.useSecurity(app, opts)` surface is now
  documented (`docs/27-security.md`): set
  `opts.csp.scriptSrc = ["'self'", "'nonce'"]` to mint a fresh
  per-request nonce, splice it into the `script-src` directive,
  and expose it to templates as the `cspNonce` view variable.

### OIDC client

- `gebweb.useOidc(app, opts)` mounts callback routes for one or
  more OAuth2 / OIDC providers at `/auth/<provider>/callback`.
- Provider presets: `gebweb.oidcGoogle(clientId, secret)`,
  `gebweb.oidcGithub(clientId, secret)`, `gebweb.oidcGeneric({...})`.
- Authorization-code-with-PKCE flow with the verifier carried
  through an HMAC-signed state cookie; on success the framework
  validates the `iss` / `aud` / `exp` claims, invokes
  `opts.userResolver(claims, provider)` to build the session
  data, and writes the session via the configured store.
- ID-token signature verification against the provider's JWKS is
  out of scope for 1.1; the response is trusted as authentic
  because it was returned over TLS by a direct POST to the
  provider's token endpoint. JWKS verification is planned for
  1.2.

### Outbound webhooks

- `gebweb.useWebhooks(app, opts)` mounts a job-queue-backed
  delivery pipeline; requires a prior `useJobs(...)` call.
- `gebweb.subscribeWebhook(app, event, url, secret)` / `unsubscribeWebhook`
  manage in-memory subscriptions; `gebweb.dispatchWebhook(app,
  event, payload)` enqueues one job per matching subscription.
- Outgoing requests are signed with HMAC-SHA256 over the body
  and carried in the `X-Gebweb-Signature` header by default
  (configurable name and pluggable `opts.signer` callable).
  `X-Gebweb-Timestamp` accompanies every send.
- Failures retry under the job queue's backoff schedule
  (`1s, 5s, 30s, 2m, 10m`). After the final attempt
  `opts.deadLetter(event, payload, lastError)` is called when
  supplied.
- `gebweb.verifyWebhookSignature(body, secret, header)` is the
  constant-time receiver-side verifier.

### Soft delete

- `gebweb.markDeleted(entity)` stamps `entity.deleted_at` with
  the current Unix time; `gebweb.restore(entity)` clears it;
  `gebweb.isDeleted(entity)` is the predicate.
- Query helpers: `gebweb.excludeDeleted(q)` appends
  `deleted_at IS NULL`, `gebweb.onlyTrashed(q)` appends
  `deleted_at IS NOT NULL`, `gebweb.withTrashed(q)` is a
  pass-through that documents intent at admin call sites.

### ETag and conditional GET

- `gebweb.useEtag(app, opts)` mounts a response-phase middleware
  that computes a weak SHA-256 ETag for every eligible 2xx
  response and rewrites the response to 304 when the request's
  `If-None-Match` matches. Skips error responses, empty bodies,
  and bodies outside the configured `minBytes` / `maxBytes`
  window.

### Server-Timing

- `gebweb.useServerTiming(app)` mounts middleware that emits a
  `Server-Timing` header populated from a per-request timing
  list. `gebweb.recordTiming(request, label, ms)` and
  `gebweb.measureTiming(request, label, fn)` append entries.

### Engine-level changes that ride with 1.1

- The web request handler now runs every app-level
  before-middleware once against the original request before
  route matching. Middleware can rewrite `request["path"]` and
  have routes match the rewritten value. Path parameters are no
  longer present on the request dict during before-middleware
  execution; that surface is a routing concern.
- A subclass whose name matches its parent's and whose
  constructor forwards via `parent(...)` no longer crashes on
  the evaluator with `no matching overload`. Facade re-exports
  in `gebweb/src/gebweb.gb` use this pattern routinely.
- `import X;` followed by a top-level `func X(...)` is now a
  compile-time error on both backends. The VM used to silently
  let the function declaration shadow the import. Use
  `import X as Y;` when the local name is taken.

## 1.0.2

Bug fixes.

- Error handling: job, scheduler, message, and event handlers, plus
  request body/parameter binding and the auto-CRUD resource, now catch
  the full `Error` hierarchy. Previously they caught only `RuntimeError`,
  so a handler throwing `IOError` (a network/DB failure) escaped the
  retry/aggregation path, and a malformed JSON body produced a 500
  instead of a 400.
- CSRF: a custom `cookieName` passed to `useCsrf` is now honoured by
  token validation. Previously validation read a hardcoded cookie name,
  so a custom name rejected every unsafe request with 403.
- `jwt.decode` returns null on a malformed token instead of throwing.

## 1.0.1

- Expanded CLI help text; documented the OpenAPI client generator.
- Fixed scheduler error logging.

## 1.0.0

Initial public release. Gebweb is a typed, decorator-driven web
framework for Geblang, modelled on FastAPI / Symfony / API Platform.
The 1.0 surface covers:

### Routing and request handling

- Decorator-driven routing: `@Get`, `@Post`, `@Put`, `@Patch`,
  `@Delete`, `@Route(method, path)`; controller-level `@Prefix`.
- Typed handler signatures with automatic parameter binding from
  the path, query string, body, or request headers.
- Explicit parameter decorators that override the binding
  heuristic: `@PathParam("name")`, `@QueryParam("name")`, `@Body`,
  `@Header("Header-Name")` (case-insensitive lookup).
- Raw-request escape hatch: a single `dict<string, any>` /
  `Request` parameter receives the request unchanged.

### Validation and serialization

- `@Assert.*` validator chain (Email, NotBlank, Length, Range,
  Pattern, Choice, plus custom validators via
  `gebweb.registerAssertion`).
- `@Groups` serialization filtering: declare read / write groups
  per field, render each response under the appropriate view.
- Validation failures become RFC 7807 Problem Details (422) for
  JSON clients; HTML clients get a 303 redirect with submitted
  body and per-field errors stashed in the session.

### Responses

- Automatic JSON wrapping of dict / list / instance returns.
- `gebweb.html`, `gebweb.htmlView`, `gebweb.file`, `gebweb.stream`,
  `gebweb.redirect` helpers.
- `HttpException` family (`BadRequestError`, `NotFoundError`,
  `ConflictError`, ...) for explicit failure responses.

### Authentication and authorization

- Pluggable `gebweb.useAuthenticator(app, UserClass, fn)`; bearer
  / API-key / session strategies via `gebweb.useApiKeyAuth` and
  `gebweb.useSessionAuth`.
- JWT helpers: `gebweb.jwtSign`, `gebweb.jwtVerify` (HS256 /
  HS384 / HS512 / RS256 / RS384 / RS512 / ES256 / ES384 / ES512).
- `@Auth`, `@RequiresRole("admin", ...)`,
  `@RequiresPermission("widgets.write")` decorators.
- OpenAPI security schemes auto-published; `bearerAuth` default,
  `gebweb.useSecurityScheme` for API keys / OAuth2 / HTTP basic.

### Data layer

- `Repository<T>` interface with `@ApiResource` auto-CRUD that
  wires up list / get / create / update / delete handlers from
  the repository.
- Query DSL: `gebweb.eq` / `neq` / `gt` / `ge` / `lt` / `le` /
  `like` / `in_` / `isNull` / `notNull` / `raw`, composable with
  `.and` / `.or` / `.not`, threaded through
  `gebweb.Query("table").where(...).orderBy(...).limit(n)
  .offset(m).select(cols) / count() / delete()`.
- Offset (`Page<T>`) and cursor (`CursorPage<T>`) pagination.
- Schema migrations via `gebweb migrate <create|up|down|status>`
  against sqlite / postgres / mysql.

### Server-rendered UX

- CSRF protection (`gebweb.useCsrf`, `@CsrfExempt`) with
  JWT-signed tokens read from `_csrf` form field or
  `X-CSRF-Token` header; current token published to templates as
  `{{ csrf }}`.
- Session-backed flash messages (`gebweb.flash`,
  `{{ flashes }}`) with one-shot semantics.
- Form-state rehydration on validation failure: submitted body
  and error map preserved across the post-redirect-get with
  `{{ old("field") }}` and `{{ errors("field") }}`.
- Static asset pipeline with content-hash fingerprinting:
  `gebweb.useStaticAssets(app, dir)` plus the `asset` view filter.
- Security headers (`gebweb.useSecurity`): X-Content-Type-Options
  / X-Frame-Options / Referrer-Policy by default; opt-in CSP with
  per-request nonce expansion; opt-in HSTS.

### Views

- Geblang-native Twig-style template engine
  (`gebweb.useViews(app, dir)`, `gebweb.view(app, name, ctx)`)
  with `{{ var|filter }}`, `{% if %}`, `{% for %}`, `{% extends %}`
  + `{% block %}`, `{% include %}`, `{# comment #}`, inline
  ternaries (`a ? b : c`), and `is [not] defined / null / empty`
  tests.
- 17 built-in filters; auto HTML-escape by default; custom
  filters via `gebweb.registerFilter`.
- Per-request view context injection via
  `gebweb.registerViewContext(app, name, fn)`; framework-supplied
  injectors for `csrf`, `flashes`, `old`, `errors`, `cspNonce`,
  and `asset`.

### Streaming and uploads

- `@WebSocket` and `@Sse` decorators for streaming endpoints.
- Multipart file uploads bound as `UploadedFile` or
  `dict<string, UploadedFile>` handler parameters;
  `UploadedFile.saveToStorage` for direct hand-off to the
  storage abstraction.

### Dependency injection

- Constructor injection with autowiring through
  `gebweb.app([Controllers...])`.
- `gebweb.register(app, Type, factory)`,
  `gebweb.registerInstance(app, Type, instance)`,
  `gebweb.registerPerRequest(app, Type, factory)`.
- `@Param("key")` constructor parameter decorator pulls primitive
  config (db URLs, secrets, feature flags) from
  `gebweb.parameter(app, key, value)`; class and parameter deps
  coexist in the same constructor.
- `di.registerInterfaceInstance(c, "Pkg.IfaceName", instance)`
  registers an instance for an interface-typed constructor
  parameter. The framework uses this internally so any wired
  service exposed as an interface (e.g. `llm.Client`) resolves
  automatically when a handler constructor depends on it.

### Declarative configuration: services.yaml

- `config/services.yaml` is auto-loaded by `gebweb.app()` when
  present. Top-level sections: `imports`, `parameters`,
  `services`, `bindings`.
- Parameters support `%env(NAME)%`, `%secret(name)%`, `%ref%`
  cross-references, and `%%` escapes. Single-token strings
  preserve the referenced value's native type.
- Services entries support `class:`, `args:` (with literals,
  `%marker%` interpolation, and `@service-ref`), `tags:`,
  `shared:` (singleton vs transient), and `aliases:`. A bare
  `"@target"` string registers an alias.
- `@Service("custom.id")` decorator registers a class for
  discovery; `gebweb.service(app, id)`, `gebweb.hasService`,
  `gebweb.serviceIds` are the matching read API.
- `bindings:` maps an interface name to a service id;
  `di.bindInterface(c, name, id)` is the programmatic surface.
  Single-implementation interfaces auto-bind on first resolve;
  multi-implementation interfaces without a binding throw at
  warm-up with the candidates listed.
- Tags: `gebweb.taggedServices(app, tag)` returns instances in
  registration order, `gebweb.taggedServiceIds(app, tag)`
  returns ids without resolving, `gebweb.tagsForService(app, id)`
  introspects.
- Per-environment overlays: `services_${GEBWEB_ENV}.yaml`
  alongside the base file merges on top. `gebweb.currentEnv()`
  exposes the active name (defaults to `prod`).
- Imports: `imports:` directive supports `optional: true` for
  files that may be absent; cycles throw at load time.

### Encrypted secrets vault

- `gebweb.useSecrets(app, provider)` wires a `SecretsProvider`
  so `%secret(name)%` markers in YAML resolve. Pluggable
  interface; custom backends (Vault, AWS Secrets Manager, etc.)
  ship as user-side classes implementing `getSecret` /
  `hasSecret`.
- Built-in `gebweb.encryptedFileSecrets()` provider reads
  `config/secrets.enc` (AES-256-GCM, base64, 80-col chunked,
  PEM-style markers) using a 32-byte key from
  `GEBWEB_SECRETS_KEY` (base64 env var, wins when set) or
  `config/secrets.key`.
- `gebweb secrets <init|edit|set|get|list>` CLI manages the
  vault: generates the AES-256 key, encrypts an empty vault,
  opens `$VISUAL` / `$EDITOR` on a JSON pretty-print of the
  current vault for interactive editing, or operates non-
  interactively for CI.

### Background work

- Background jobs: `@Job("name")` handlers,
  `gebweb.enqueue(app, name, payload)`,
  `gebweb.useJobs(app, conn)`, retry with exponential backoff,
  multi-worker safe via atomic conditional updates.
- Scheduled tasks: `@Scheduled("0 3 * * *")` with leader
  election so multiple worker processes don't double-fire.
- In-process event bus: `@On("user.created")`,
  `gebweb.publish(app, "user.created", payload)`.
- Message brokers: `@OnMessage("orders")` handlers with
  pluggable backends (RabbitMQ / STOMP / SQS / SNS / Kafka topics +
  queues); `gebweb.useMessageQueue`, `gebweb.useMessageTopic`,
  `gebweb.runMessageWorker`.

### Integrations

- Mailer abstraction (`gebweb.Mailable`, `gebweb.useMailer`,
  `gebweb.send`) with SMTP / AWS SES / memory / log transports; async
  send via the job queue when one is configured.
- File storage abstraction (`gebweb.useStorage`,
  `gebweb.put` / `get` / `storageExists` / `storageDelete` /
  `storageUrl`) with memory / local disk / S3 (sigv4, also
  works against MinIO / R2 / B2) backends.
- Response caching via `gebweb.useCacheStore(app, store)` and
  `@Cache(ttl, vary)` per route.
- LLM client (`gebweb.useLlm`, `gebweb.llmClient`) wrapping the
  stdlib `llm` module: one Geblang interface for chat completions,
  text embeddings, image analysis, and image generation across
  OpenAI, Anthropic, and AWS Bedrock. The registered client is
  resolvable through DI autowiring (constructor params typed
  `llm.Client` are injected automatically) and via the
  `gebweb.llm(app)` getter for non-DI call sites.

### Security policies

- Per-route `@RateLimit(perSecond, burst)`, `@Cors({...})`.

### OpenAPI 3.1 + Swagger UI

- Auto-generated spec at `/openapi.json` from handler
  signatures; SwaggerUI mounted at `/docs`.
- `@Tag("group")`, `@Summary`, `@Description` flowing into
  OpenAPI per-operation and per-parameter fields.
- `@ApiResponse(status, description)` for explicit additional
  responses.

### Middleware

- `gebweb.use` (response phase) / `gebweb.before` (request
  phase) / `gebweb.after` (after phase) hooks.
- Built-in middleware factories: `cors`, `securityHeaders`,
  `requestId`, `requestLog`, `compress`, `rateLimit`.

### Plugins

- `gebweb.plugin(app, instance)` install hook and `Plugin`
  base class; sibling packages plug in cleanly.

### CLI

- `gebweb new <name>` scaffolds a new project.
- `gebweb dev` runs the project with file-watch hot-reload.
- `gebweb build` produces a release binary.
- `gebweb routes` prints the route table.
- `gebweb generate <controller|dto|repository|resource> <Name>`
  scaffolds boilerplate.
- `gebweb generate client <spec> <Name>` generates a typed HTTP
  client from an OpenAPI 3.x spec (YAML or JSON): one DTO per
  component schema, one method per operation, automatic auth
  handling for bearer / basic / apiKey (header, query, or cookie)
  security schemes.
- `gebweb migrate <create|up|down|status>` runs schema
  migrations.
- `gebweb worker` runs the background-job + messaging worker;
  `--job` / `--handle` flags filter to a subset of work so
  different servers can drain different pools.

### Testing

- `gebweb.TestClient(app)` for in-process request dispatch;
  `client.get` / `post` / `put` / `delete` / `request` /
  `multipart`; `r.assertStatus`, `r.json`, `r.text`.
- Integrates with the `test.Test` base class.

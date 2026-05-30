# Gebweb changelog

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
  pluggable backends (RabbitMQ / STOMP / SQS / Kafka topics +
  queues); `gebweb.useMessageQueue`, `gebweb.useMessageTopic`,
  `gebweb.runMessageWorker`.

### Integrations

- Mailer abstraction (`gebweb.Mailable`, `gebweb.useMailer`,
  `gebweb.send`) with SMTP / memory / log transports; async
  send via the job queue when one is configured.
- File storage abstraction (`gebweb.useStorage`,
  `gebweb.put` / `get` / `storageExists` / `storageDelete` /
  `storageUrl`) with memory / local disk / S3 (sigv4, also
  works against MinIO / R2 / B2) backends.
- Response caching via `gebweb.useCacheStore(app, store)` and
  `@Cache(ttl, vary)` per route.

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

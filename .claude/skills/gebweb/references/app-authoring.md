# Gebweb app-authoring reference

The programming model for a Gebweb application. Verified against Gebweb 1.6.0.
Confirm exact signatures with `geblang doc gebweb/src/` or the `gebweb/docs/`
chapters; names below are read from the framework source.

## Facade and entrypoint (`gebweb.gb`, `app.gb`)

`import gebweb;` exposes a flat surface.

- `gebweb.app(controllers = [], opts = null): GebwebApp` - build the app. With
  NO controller list it auto-discovers every `@Controller` class; an explicit
  list is the complete controller set (other `@Controller` classes are not
  mounted, though `@Service`/`@Policy` handlers still discover).
- Serving: `gebweb.cli(app, opts)` is the standard deployable entrypoint (flag /
  env / opts precedence, TLS, graceful drain); `gebweb.serve(app, addr, opts?)`
  blocks; `gebweb.listen(app, addr, opts?): int` is non-blocking;
  `gebweb.dispatcher(app): callable` and `gebweb.routes(app)` expose internals.
- `gebweb.setInfo(app, info)` sets OpenAPI metadata. App opts toggle features:
  `{"docs": false}` / `{"swaggerUi": false}` / `{"openapi": false}`; env
  `GEBWEB_ENV=production`, `GEBWEB_DOCS=off`.

## Controllers (`controller.gb`)

Extend `gebweb.Controller`. Constructor parameters are dependency-injected.
Response helpers (return `http.Response`): `json(data, status=200)`,
`html(body, status=200, headers?)`, `text(body, status=200)`,
`created(data, location)`, `accepted(data?)`, `noContent()`,
`stream(handler, opts?)`, `problem(status, title, detail, extras?)`,
`redirect(location, status=302)`. Throwing error helpers: `badRequest`,
`unauthorized`, `forbidden`, `notFound`, `conflict`, `unprocessable(detail,
errs)`. View helpers: `view(request, name, ctx, status=200)`,
`partial(request, name, ctx)`, `back(request, fallback="/")`, `flash(...)`,
`redirectWithFlash(...)`.

## Routing and handler decorators (`decorators.gb`)

- Method routes: `@Get`, `@Post`, `@Put`, `@Patch`, `@Delete`, `@Options`,
  `@Route(method, path)`. Path params use `{name}`.
- Class-level: `@Controller("/prefix")`, `@ApiVersion("v2")`, `@Deprecated` /
  `@Deprecated("<sunset>")`.
- OpenAPI-shaping metadata (no dispatch effect): `@Summary`, `@Description`,
  `@OperationId`, `@Tag`, `@ApiResponse(status, desc, ...)`.

## Parameter binding (`binding.gb`)

Handler parameters are bound in priority order: path param -> authenticated user
-> uploaded files (`dict<string, gebweb.UploadedFile>`) -> request body (one
user-class or `list<UserClass>`, deserialized via `json.parseAs`) -> query param
-> rich `gebweb.Request` / raw `dict` / `any`. Explicit decorators override the
heuristic: `@PathParam("name")`, `@QueryParam("name")`, `@Body`,
`@Header("Header-Name")` (case-insensitive). The body parameter must be the only
user-class parameter; a raw-escape parameter (`dict`/`any`/`Request`) must be the
only parameter on the handler.

`gebweb.Request` accessors: `method()`, `path()`, `scheme()`, `isSecure()`,
`host()`, `clientIp()`, `header()`, `cookie()`, `query()`, `queryInt()`,
`queryBool()`, `isJson()`, `text()`, `json()`, `routeParam()`, plus `.locale`,
`.tenant`, `.user`, `.csrfToken`. It stays index-compatible (`req["headers"]`).

## Dependency injection (`di.gb`)

Constructor injection. Facade: `gebweb.register(app, ClassRef, factory)`
(singleton), `gebweb.registerInstance(app, ClassRef, instance)`,
`gebweb.registerPerRequest(app, ClassRef, factory)`,
`gebweb.resolve(app, ClassRef)`. Services: `@Service` / `@Service("custom.id")`
on a class, looked up with `gebweb.service(app, id)`. Declarative wiring via
`config/services.yaml` (`gebweb.loadConfig`). `@Param("key")` injects a
parameter-store value. Auto-discovery scans `@Controller`/`@Service`/`@Policy`
once at first dispatch; cache it with `{"discoveryCache": true}`.

## Validation (`validation.gb`)

`@Assert.<name>` on a DTO field. **Valid names only** (unknown ones silently
no-op): `email`, `url`, `uuid`, `regex(pattern)`, `minLength(n)`, `maxLength(n)`,
`range(min, max)`, `in([...])`, `notBlank`. `@Valid` cascades into nested
DTO / `list<DTO>` / `dict<string, DTO>`. Custom rules:
`gebweb.registerAssertion(app, name, func(any v, list args, dict named):
?string)`. Failures produce a 422 RFC 9457 Problem Details (JSON) or a 303
redirect with stashed input + errors (HTML form).

## Repositories, @ApiResource, query DSL (`repository.gb`, `resource.gb`, `query.gb`)

- `interface Repository<T>`: `find(id): ?T`, `list(Page): list<T>`,
  `save(T): T`, `delete(id): void`. Helpers `Page`, `Cursor`, `CursorPage`;
  build pages with `gebweb.page(offset, size, sort, direction)`. Optional probed
  methods: `count()`, `findBy(criteria)`, `listCursor(Cursor)`.
- `@ApiResource("/path")` generates LIST/GET/POST/PUT/PATCH/DELETE. The
  decorated class MUST declare `static func repositoryClass(): any { return
  <Name>Repository; }` - the framework probes for this static method and throws
  at startup if it is absent.
- Query DSL: `eq, neq, gt, ge, lt, le, like, in_, isNull, notNull, raw, asc,
  desc`; `Query("table").where(...).orderBy(...).limit().offset().select()`.

## Serialization groups (`serialization.gb`)

`@Groups("read", "write", ...)` filters fields per context; `gebweb.serialize`,
`applyInput`, `fieldVisible`.

## Auth and JWT (`auth.gb`, `authz.gb`, `jwt.gb`)

- Setup: `gebweb.useAuthenticator(app, UserClass, func(req): ?any)`,
  `gebweb.useSessionAuth(...)`, `gebweb.useApiKeyAuth(...)`,
  `gebweb.usePermissions(...)`.
- Decorators: `@Auth`, `@RequiresRole("a", "b")`, `@RequiresPermission("x")`,
  `@Policy` / `@Can`. The authenticated user injects into any handler parameter
  typed as the user class.
- JWT (`import gebweb.jwt as jwt;`): `issue(secret, claims, ttlSeconds)`,
  `verify(secret, token)`, `verifyAt(secret, token, nowSec)`, `decode(token)`.
  HS256 only.

## Middleware (`middleware.gb`)

Hooks: `gebweb.use(app, mw)` (response stage), `gebweb.before(app, mw)` (early,
may short-circuit), `gebweb.after(app, mw)`. Factories: `cors`,
`securityHeaders`, `requestId`, `requestLog`, `etag`, `compress`, `rateLimit`,
`abuseGuard`. Per-route decorators: `@RateLimit`, `@Cors`, `@MaxBody`.

## Caching (`cache.gb`)

`@Cache(ttl: 60, vary: [...], tags: [...])`; `gebweb.useCacheStore(app, store)`;
tag invalidation `gebweb.cacheInvalidate(app, tag)`.

## Views (`views.gb`)

`gebweb.useViews(app, dir = "templates")`; render with the controller's
`this.view(request, name, ctx)` or `gebweb.htmlView(app, request, name, ctx)` /
`gebweb.view(app, name, ctx, request?)`. Twig-style: `{{ expr }}`, `{# comment
#}`, `{% if/elif/else/endif %}`, `{% for ... endfor %}` (`loop.index/first/last/
length`), `{% set %}`, `{% include %}`, `{% extends %}` / `{% block %}`,
`{% raw %}`, inline ternary, `is [not] defined/null/empty`. Filters: `escape`/
`e`, `raw`/`safe`, `upper`, `lower`, `capitalize`, `length`, `default`, `json`,
`date`, `replace`, `trim`, `join`, `split`, `first`, `last`, `abs`, `round`.
Auto-escape is on by default. Register filters/context with
`gebweb.viewsFilter(app, name, fn)` / `gebweb.registerViewContext(app, name,
fn)`.

## Async work, events, schedule, messaging

- Jobs (`jobs.gb`): `@Job("name", priority:, unique:, retry:, timeoutMs:)`;
  `gebweb.useJobs(app, conn, opts?)`, `gebweb.enqueue(app, name, payload,
  opts?)`, `gebweb.runWorker(app, {queues: [...]})`. DB-backed.
- Events (`events.gb`): `@On("event.name")`; `gebweb.useEvents(app)`,
  `gebweb.publish(app, name, payload)`. Synchronous in-process.
- Scheduler (`scheduler.gb`): `@Scheduled("<cron>")`; `gebweb.useScheduler(app)`,
  `gebweb.tickScheduler(app)`. 5-field cron, DB-row leader election.
- Messaging (`messaging.gb`): `@OnMessage("handle")`;
  `gebweb.useMessageQueue(app, name, handle)`,
  `gebweb.useMessageTopic(app, name, handle)`,
  `gebweb.runMessageWorker(app, opts?)`. RabbitMQ / STOMP / Kafka / SQS / SNS.

## Streaming, uploads, OpenAPI, i18n, tenancy

- WebSockets / SSE (`streaming.gb`, `broadcast.gb`): `@WebSocket`, `@Sse`;
  `gebweb.Hub` / `gebweb.newHub()` for fan-out.
- Uploads (`uploads.gb`): bind `dict<string, gebweb.UploadedFile>`;
  `UploadedFile.bytes()`, `size()`, `saveTo(path)`, `saveToStorage(app, name)`.
- OpenAPI / SwaggerUI (`openapi.gb`, `swaggerui.gb`): spec at `/openapi.json`,
  UI at `/docs`; `gebweb.useDocsAuth(app, guard)` with `gebweb.basicAuthGuard` /
  `gebweb.bearerTokenGuard` / `gebweb.requireAppAuth`.
- i18n (`i18n.gb`): `gebweb.useI18n(app, opts?)`, `gebweb.t(app, locale, key,
  args = {})`; YAML catalogs, plural-aware.
- Multi-tenancy (`tenancy.gb`): `gebweb.useTenant(app, resolver, opts?)`,
  `gebweb.currentTenant(request)`, `scopedQuery(query, tenant)`.

## Other facade modules

Mailer (`useMailer` / `smtpMailer` / `sesMailer` / `Mailable` / `send`), storage
(`useStorage` / `localStorage` / `s3Storage` / `put` / `get`), CSRF (`useCsrf`,
`@CsrfExempt`, `{{ csrf }}`), flash (`gebweb.flash`, `{{ flashes }}`), forms,
static assets (`useStaticAssets`, `asset` filter), health (`useHealth`,
`@HealthCheck`, `/healthz` / `/readyz`), OIDC (`useOidc` / `oidcGoogle` /
`oidcGithub`), webhooks (`useWebhooks` / `dispatchWebhook` /
`verifyWebhookSignature`), LLM (`useLlm` / `llmClient`), sessions (`useSession`),
secrets (`useSecrets`), soft-delete (`markDeleted` / `restore` / `excludeDeleted`),
profiler bar (`useProfilerBar`), plugins (`gebweb.plugin`, `Plugin`). Confirm any
of these with `geblang doc gebweb/src/gebweb.gb`.

For `instanceof` / `catch` on HTTP errors: `import gebweb.errors as errors;` then
match `errors.HttpException` and its subclasses.

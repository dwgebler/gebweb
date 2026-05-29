# Gebweb Roadmap

Forward-looking work after the 1.0.0 initial public release. Shipped
history lives in [CHANGELOG.md](CHANGELOG.md).

Each release is a themed bundle. Items in the same release ship
together so the boundary makes sense to users; items across releases
land independently. T-shirt sizes are S (1-2 days), M (3-7 days),
L (1-2 weeks), XL (3+ weeks).

---

## 1.1.0 - i18n / localization

Theme: ship to non-English markets.

- **Locale negotiation (S).** Resolve current locale from URL prefix
  (`/en/`, `/de/`), session cookie, or `Accept-Language`. Order
  configurable via `gebweb.useI18n(app, {sources: [...]})`.
- **Translation catalogs (M).** JSON / TOML files per locale under
  `locales/`. Loader caches at boot; reload-on-save in dev.
  `gebweb.t("auth.invalidCredentials")` from handlers; `{{ t(...) }}`
  in views. Parameter interpolation: `{{ t("greeting", {name: user}) }}`.
- **Pluralisation (M).** CLDR plural-rule subset (one / other / few /
  many) with named cases per locale. `{{ tn("cart.items", count) }}`.
- **Validator messages (S).** `@Assert.*` produces locale-keyed
  message codes (`assert.email`); the validator output translates
  them through the active locale at serialisation time. Existing
  English defaults remain when no translation is found.
- **Locale-aware formatting (S).** `format.date(instant)`,
  `format.number(decimal)`, `format.currency(decimal, "USD")`
  helpers respecting the active locale + zone.

Estimated total: 2 weeks. Validator + view integration are the
load-bearing parts.

---

## 1.2.0 - API versioning + multi-tenancy

Theme: SaaS-shaped concerns. Small surface; ships together because
both touch route resolution.

- **`@ApiVersion("v2")` decorator (S).** Auto-prefixes the route
  (`/v2/...`), groups in OpenAPI under per-version sections, emits
  `Deprecation` and `Sunset` headers when annotated as deprecated.
  Backwards-compatible: routes without the decorator keep current
  behaviour.
- **Tenant resolution (M).** `gebweb.useTenant(app, resolver)` where
  the resolver inspects the request (subdomain / header / path /
  authed user claim) and returns a `Tenant` value bound on the
  request-scope DI container. Handlers inject `Tenant` as a typed
  parameter.
- **Tenant-scoped repository (M).** New `TenantRepository<T>` base
  that auto-applies `WHERE tenant_id = ?` to every query and binds
  the resolved tenant on insert. Existing `Repository<T>` is
  unchanged.
- **Optional schema-per-tenant migration (L).** Migration runner
  iterates over a tenant list and applies up/down per schema. Behind
  an explicit `--per-tenant` flag; the default stays single-schema.

Estimated total: 1.5-2 weeks.

---

## 1.3.0 - Worker enhancements

Theme: production-grade job processing.

- **Priority queues (M).** `@Job("send-mail", priority: "high")` and
  worker config `{queues: ["high", "default", "low"]}` drained in
  order. Schema: `priority` column on `gebweb_jobs`, indexed.
- **Unique jobs (S).** `@Job("name", unique: "$payload.userId")`
  dedupes by computed key for the duration of the active retry
  window. Backed by a unique index on `(name, dedupe_key, completed_at IS NULL)`.
- **Per-handler retry policy (S).** Override exponential default per
  job: `@Job("name", retry: {maxAttempts: 5, backoff: "linear"})`
  or full custom: `retry: backoffFn`.
- **Dead-letter inspection (S).** `gebweb worker dlq list/retry/purge`
  CLI for jobs that exhausted retries. Today they sit in the table
  with no operator UX.
- **Per-job timeout (S).** `@Job("name", timeoutMs: 30000)` with
  context cancellation. Handler interruption + clean release on
  timeout.

Estimated total: 1.5 weeks. Schema migration to add the new columns
is the one breaking-change moment; ship behind a migration that
defaults old jobs to priority="default", unique=null, etc.

---

## 1.4.0 - TLS / HTTP/2

Theme: single-binary deployment. Today gebweb apps assume a reverse
proxy (nginx / Caddy / Cloud Run) terminates TLS.

- **Native TLS (M).** `gebweb.serve(app, ":443", {tls: {certFile,
  keyFile}})` or `{tls: {autoCert: "example.com"}}` (ACME / Let's
  Encrypt via `golang.org/x/crypto/acme/autocert`). Required cert
  format documented; reload on SIGHUP.
- **HTTP/2 (S).** Comes for free with TLS in Go's net/http via
  `h2_bundle.go`; verify it's wired and documented. Plain-text H2C
  for proxy-fronted deployments.
- **HSTS opt-in (S).** Already shipped in 1.0.0 via the security
  headers middleware; verify documentation + default-off behaviour
  alongside the TLS work.

Estimated total: 1 week.

---

## 2.0.0 - Live components (design-doc-gated)

Theme: SPA-feel without a SPA. The biggest UX play and the biggest
risk; gated on a written design before any implementation starts.

**Stage 0 (design doc, 1-2 weeks):** Write
`docs/design/live-components.md` covering:

1. Protocol shape. JSON event envelope (`{component, action, payload,
   patch}`). HTML diff format: full-replace vs morphdom-style
   patches vs operation list. Reconnect semantics on WebSocket drop.
2. Component model. Server-side `LiveComponent` base with `state`,
   `handle(event)`, `render()`. Lifecycle: mount, update,
   destroy. Component identity (per-tab vs per-user vs shared).
3. State lifetime + storage. Memory by default; opt-in DB-backed
   sessions for long-lived state.
4. Security boundary. Server validates every incoming event against
   the component's declared event handlers. CSRF for the upgrade.
   Component-state encryption when echoed to the client.
5. View integration. `{% live "ChatComponent" %}` view tag; client
   JS shim that wires WebSocket + DOM patching.
6. Backpressure + presence. Integrates with the messaging module
   for cross-process broadcast.

**Stage 1 (MVP, 2-3 weeks):** Full-HTML-per-event mode. Server
re-renders the entire component on each state change and replaces
its DOM root client-side. No diff engine. Genuinely useful for
forms / dashboards; lays the protocol + component-model
groundwork.

**Stage 2 (diff engine, 2-3 weeks):** Replace full-HTML with a
morphdom-style diff or operation-list patch. Cuts bandwidth
substantially; lets long pages feel snappy.

**Stage 3 (presence + broadcast, 1-2 weeks):** Multi-client state
synchronisation via the messaging module. Lets a chat component or
multi-cursor editor work across browser tabs and servers.

Total realistic timeline: 8-10 weeks of focused work. Worth the
2.0.0 boundary because the streaming primitives behind the API are
significantly more constrained than today's `@WebSocket` / `@Sse`.

---

## Parked / not on the roadmap (yet)

- **GraphQL.** Common request but niche for gebweb's REST-first
  audience. Revisit if a real workload surfaces.
- **Cache tag-based invalidation.** Useful but not yet on a
  release; can layer on the existing `@Cache` decorator as a 1.x.y
  patch when the need is concrete.
- **Health-check + structured request-context conventions.** Small
  enough to land as a 1.x.y patch alongside whatever else is
  happening.
- **Database factories + seeders.** Generator family expansion
  (Laravel factories / Django fixtures). Worth doing alongside
  1.2.0 multi-tenancy because test fixtures benefit from tenant
  scoping.
- **Mail preview routes / dev viewer.** Laravel-Mailtrap-style
  local inbox. Small lift; can ride 1.1.0 or 1.2.0 if cycles allow.
- **Notifications channel abstraction.** Laravel-style multi-
  channel notify (email / SMS / Slack / DB). Defer until there's a
  real ask; the existing mailer + messaging covers most cases.

---

## Working-style notes

- **Tests + docs land in the same release** as the feature. Bundle
  has its own doc chapter under `docs/` and its own integration
  test file under `tests/`.
- **CHANGELOG entries** stay concise per the geblang house style:
  user-visible bullets, no implementation chatter.
- **Decorators are append-only.** Once shipped a decorator can gain
  options but cannot be renamed without a major-version bump.

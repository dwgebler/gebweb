# Application best practices

Use the [bookmarks tutorial](38-building-bookmarks.md) for a complete application
and this chapter when deciding how to structure and operate your own app. The
guidance below follows Gebweb's actual binding, service-lifecycle and runtime
contracts.

## Keep boundaries typed and explicit

Declare concrete parameter and return types on controllers and services. Use
input classes for submitted data and separate output values when their fields
differ. The bookmarks app accepts `NewBookmark` and returns `Bookmark`: the
database assigns the ID, while the input class declares URL and title rules.

`@Body` binding and `@Assert.*` validation give a typed handler a checked input
value. Values read manually from JSON or a raw request dictionary still need
shape, type and domain validation before entering typed application logic.
An `any` cast alone does not establish that contract.

Keep parsing at the HTTP boundary, business decisions in application services,
and SQL in repositories. Pass the values a service needs rather than making
every service inspect an entire request. This makes the same logic usable in
HTTP handlers, jobs and tests.

Return a response for the protocol you intend. Creation can use
`this.created(value, location)`; deletion can use `this.noContent()`. Throw the
framework's HTTP exceptions for expected failures such as an absent record.
Validation failures produce 422 for JSON clients and, with sessions configured,
a 303 redirect with form state for HTML clients. A write protected by CSRF can
be rejected before binding or validation runs.

See [Parameter binding](03-parameter-binding.md), [Responses](04-responses.md)
and [Validation](05-validation.md).

## Own application construction in one place

Use a factory that receives infrastructure and configuration, then returns a
fully wired app. The bookmarks factory takes a connection, a CSRF secret and a
session directory. Its entry point owns configuration loading and serving;
tests provide an in-memory database and temporary session storage.

Register infrastructure-backed services before binding controllers that need
them. Constructor injection makes those dependencies visible. Configure the
app during startup, then serve it; keep runtime state changes in the stores
designed for them.

Import the modules containing controllers, services and handlers. Discovery
inspects loaded classes from the import graph. Creating a source file without
loading its module does not make its classes discoverable. Use
`gebweb.discover(app)` when you want to inspect the discovery report before the
first request.

See [App construction](01-getting-started.md#app-construction) and
[Dependency injection](06a-dependency-injection.md).

## Choose a lifetime before choosing a binding

Controllers and ordinary autowired services are shared within an app. Use
shared services for stateless logic and infrastructure designed for concurrent
use. Keep request-specific values in handler parameters, locals or services
resolved within a request scope.

| Value | Suitable lifetime and owner |
| --- | --- |
| Database pool or reusable cache client | Application-owned; shared by services until shutdown. |
| Read-only configuration | Construct at startup; share without mutation. |
| Current request, identity or route inputs | Pass through the active handler call. |
| Mutable per-request service | `registerPerRequest`, resolved inside the active dispatch scope. |
| Transaction, row cursor or request-owned file | Acquire for the operation; finish or close before its owner returns. |
| State that must survive a process restart | Persist in the database or another backing store. |

`registerInstance` supplies a shared object. `registerPerRequest` reuses one
instance for resolves during one dispatched request and creates another for
the next request. Outside a request scope, its factory runs on every resolve.

Controller constructors run during route registration. Injecting a per-request
service into a controller constructor captures the startup instance; the
controller keeps it across requests. Pass request values into handler calls,
or resolve the service within the active request rather than retaining that
startup object. The lifecycle registration controls reuse, not resource
disposal.

## Make shared writes one operation

A handler local can still refer to a shared list, dictionary or object.
Collections use call by sharing, and `copy()` is shallow. A new local variable
or an outer-container copy does not isolate nested shared values.

Keep shared services stateless where possible. For mutable in-process state,
use the atomic operations on `store.Store` or an appropriate lock. For example,
each `incr` below is a single atomic operation:

```gb
import io;
import store;

let hits = store.Store();
io.println(hits.incr("bookmarks"));
io.println(hits.incr("bookmarks"));
```

This prints `1` and `2`. A read followed by a write remains two operations,
even when both operations are individually safe. Use `incr`, `compareAndSet`
or `update` for the matching operation; protect a wider invariant with a lock
or a database transaction. A Store is local to its process, so multiple app
instances need a shared backing store for a common invariant.

See [Concurrency and shared state](35-concurrency.md).

## Give database resources an owner

Share a `db.Connection` pool through repositories that keep no mutable
per-request fields. A handler should finish its own transaction or cursor
without closing the shared pool. The application owner closes the pool after
requests and workers have stopped using it.

Use bound SQL parameters. The bookmarks repository's insertion is:

```gb
let result = this.conn.exec(
    "INSERT INTO bookmarks (url, title) VALUES (?, ?)",
    input.url,
    input.title
);
```

For a first row, `queryOne` returns a row or null and closes its cursor. For
streaming queries, close the row cursor when finished, including early exits.
Materializing with `all()` is convenient for bounded results; add pagination
when the result can grow with application data.

A pool makes individual calls safe to share. Several calls do not form one
transaction automatically. When related writes must succeed together, keep a
transaction local to the operation, pass that transaction to collaborators,
and commit or roll back before returning. Queries made through the pool while
a transaction is open are separate from that transaction. Keep unrelated
network work outside the transaction's lifetime.

Keep migration IDs stable after application and add new migrations for schema
changes. Choose whether deployment or startup owns migration execution. The
bookmarks app uses `conn.migrate` at startup; the CLI migration workflow is a
separate option described in [Database migrations](17-migrations.md).

## Separate identity, authorization and request integrity

Use an authenticator to establish identity and route policies to require the
appropriate user, roles or permissions. Enforce ownership of a particular
record in application logic or a repository query using the trusted current
user. An ID supplied in a route or body identifies a requested record; it does
not establish permission to read or modify it.

Cookie-authenticated browser writes need CSRF protection. Enable `useCsrf`
explicitly, use a nonblank random secret, and send both the cookie and either
the hidden `_csrf` field or `X-CSRF-Token`. Returning JSON does not change how
the browser supplies credentials. Keep GET handlers free of state-changing
business operations.

Use `@CsrfExempt` when the route has a deliberate independent verification
contract, such as a provider-signed webhook. Implement and test that verifier;
an exemption supplies no authentication by itself.

Configure cookie flags where each cookie is issued. The CSRF configuration's
`cookieOptions` controls its token cookie; it does not configure the separate
session cookie. Verify Secure, HttpOnly, SameSite, path and expiry on actual
responses for the deployment you serve. See [Authentication](08-auth.md) and
[CSRF protection](23-csrf.md).

The bookmarks app deliberately has no accounts or ownership model. Add those
before using it as a private collection shared by several users.

## Preserve the intended browser flow

Pass the active request into `this.view` so CSRF, flashes and form state enter
the template context. Keep automatic HTML escaping for stored values and old
input. Treat a `raw` filter as a decision that the value is trusted HTML.

Form rehydration persists submitted values and validation errors in a session,
then consumes them after the next render. Decide which data may be retained;
sensitive forms can return an explicit error response without queuing their
contents as old input. Test both the first redirect render and the subsequent
render so one-shot state does not silently become permanent UI state.

Session helpers may rotate the session cookie. Carry the latest Set-Cookie
values into the next request, including after rendering consumed state.
For manual form flows, pass each helper's returned response into the next
helper so its session update is preserved. See [Forms](25-forms.md) and
[Flash messages](24-flash.md).

## Cache a response only within its actual scope

Choose cache keys from everything that changes the response: route and query,
negotiated language or representation, and the relevant user or tenant scope.
Gebweb partitions cached routes that require a user by the resolved user;
`useUserScope` supplies a different partition when needed. The same user scope
must not stand for different tenant-visible data.

Keep request-specific form tokens, one-shot messages and CSP nonce-bearing
HTML out of a shared response cache. A cached body carrying an old nonce can
disagree with a fresh response security header. Verify invalidation through
the route that changes the underlying data, not only by inspecting a TTL.
See [Caching](10-caching.md) and [Security headers](27-security.md).

## Validate configuration after loading it

Read configuration at startup and pass validated values to the app factory.
`secrets.requireEnv` checks that a variable exists; an empty string is still a
present value. Validate nonblank secrets and domain-specific ranges yourself.
The bookmarks factory uses this check before creating its session/CSRF setup:

```gb
if (csrfSecret.trim().isEmpty()) {
    throw ValueError("CSRF_SECRET must not be empty");
}
```

Choose environment precedence deliberately. `dotenv.loadAndApply` overwrites
matching process environment values when you call it. The server CLI then
resolves flags over environment over option defaults. Keep `.env`, database
files, session files and other mutable deployment data out of source control
and outside public asset directories.

Use a session store accessible to every instance that must read the same
session. The bookmarks file store is convenient for one app instance; separate
local directories do not share sessions between instances. Keep each process's
configuration and writable paths explicit.

## Close request resources and observe background work

Use `defer` or `finally` near acquisition for operation-owned handles.
When handing a handle to a response stream, give the producer explicit
ownership through its last read or write and close it when the stream ends.
`gebweb.onRequestEnd` is available for request-owned resources recorded in the
request dictionary. Finalizers run in reverse registration order even when
dispatch fails; a finalizer failure is logged and remaining finalizers still
run. Pass the owned handle through request state. The normal DI request scope
has ended by finalization, so resolving a per-request service there can create
a different instance.

Give background work an explicit owner that observes its result and failure.
Use a persistently backed job queue and an owned worker for work that must
survive request completion or process failure. Execution is at least once:
make external effects idempotent and
persist enough state to recognize completed work. A job timeout requests
cooperative cancellation and retains its slot and lease until the handler
returns. Poll cancellation in long work; a timeout is not a forced stop.
See [Background jobs](18-background-jobs.md).

## Test observable behavior in the right environment

Construct the app through the same factory used by the real entry point.
Use an in-memory database or isolated temporary resources and close/remove
them in teardown. Supply test services at boundaries when useful, and assert
on responses and stored results rather than collaborator call counts.

Run tests from the app directory when templates and fixtures use relative
paths. `TestClient` dispatches in process; carry cookies explicitly and provide
the real Content-Type, Accept and Referer headers for the flow under test.
Keep CSRF enabled while testing protected writes.

| Change | Useful verification |
| --- | --- |
| Typed body or validation rule | Valid and invalid input; HTML redirect and JSON 422. |
| Private resource route | Authorized, unauthenticated and wrong-owner cases. |
| Browser form or session behavior | Missing/mismatched CSRF, redirects, latest cookies, one-shot state and escaped output. |
| Shared mutable state | Concurrent requests exercising the invariant that must hold. |
| Resource ownership | Successful response, expected error and early exit paths. |
| Deployment or resource layout | Warm-cache serving and a built binary from a directory without source/templates. |

For the bookmarks layout, run strict checking and both runtimes:

```sh
geblang check --strict src/
geblang test src/
geblang test --runtime=evaluator src/
```

Adjust the paths when your tests live in a separate directory.
Add real HTTP checks for boundaries an in-process client cannot cover, such as
wire headers, listeners, streamed files and shutdown. The bookmarks example's
`verify.sh` demonstrates temporary data, readiness probing, cache reuse and an
isolated standalone binary. See [Testing](14-testing.md).

## Deploy the artifact you verified

Commit the dependency lockfile and build with the matching toolchain and
framework version. Test the built binary and embedded resources. Configure
external database/session paths explicitly when running it from another
directory; a bundled template does not bundle live application data.

Use `gebweb.cli` for the standard server flags and shutdown contract. With
`useOps`, readiness reports draining during shutdown so traffic can move away
before shared infrastructure is closed. Give workers and other long-running
tasks the same drain ownership.

Configure trusted proxies for the proxies that actually terminate connections
so client-IP policy has the intended meaning. Choose HTTPS, cookie flags and
security headers for that serving path. Set request-body and concurrency
limits from the workload you support, and measure latency and memory using
the built artifact before changing runtime tuning. See
[Deployment](37-deployment.md) for the available options.

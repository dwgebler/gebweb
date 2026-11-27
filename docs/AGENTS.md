# AGENTS.md: Gebweb for AI agents

<!-- doctest:strict -->

This is a framework cheatsheet for editing Gebweb applications. Use Geblang
1.47.0 or later with Gebweb 1.11.1. Consult the language manual for syntax and
runtime primitives, and the linked chapters for complete framework contracts.
Ground APIs in the installed version rather than borrowing conventions from
another language or framework.

## What Gebweb is

Gebweb provides typed routes, DI, validation, OpenAPI, views, authentication,
repositories and background work. Controllers and singleton services are shared
across requests. Discovery inspects decorated classes already loaded through
the import graph; it does not scan source directories for controllers.

## The minimum app

Use typed input and output, an injected repository and a factory that accepts
its infrastructure. Put this in `src/contacts_app.gb`:

<!-- doctest:file contacts_app.gb -->
```gb
import db;
import gebweb;
import log;

class NewContact {
    @Assert.email
    string email;
}

class Contact {
    int id;
    string email;

    func Contact(int id, string email) {
        this.id = id;
        this.email = email;
    }
}

class ContactRepository {
    db.Connection conn;

    func ContactRepository(db.Connection conn) {
        this.conn = conn;
    }

    func all(): list<Contact> {
        list<Contact> contacts = [];
        for (row in this.conn.query("SELECT id, email FROM contacts ORDER BY id").all()) {
            contacts.push(Contact(row["id"] as int, row["email"] as string));
        }
        return contacts;
    }

    func find(int id): ?Contact {
        let row = this.conn.queryOne("SELECT id, email FROM contacts WHERE id = ?", id);
        return row == null ? null : Contact(row["id"] as int, row["email"] as string);
    }

    func add(NewContact input): Contact {
        let result = this.conn.exec("INSERT INTO contacts (email) VALUES (?)", input.email);
        return Contact(result["lastInsertId"] as int, input.email);
    }
}

@Controller("/contacts")
class ContactController extends gebweb.Controller {
    ContactRepository contacts;

    func ContactController(ContactRepository contacts) {
        parent();
        this.contacts = contacts;
    }

    @Get("")
    func all(): list<Contact> {
        return this.contacts.all();
    }

    @Get("/{id}")
    func show(int id): Contact {
        let contact = this.contacts.find(id);
        if (contact == null) {
            this.notFound("contact not found");
        }
        return contact as Contact;
    }

    @Post("")
    func create(@Body NewContact input): gebweb.Response {
        let contact = this.contacts.add(input);
        return this.created(contact, "/contacts/" + (contact.id as string));
    }
}

export func build(db.Connection conn): gebweb.GebwebApp {
    conn.migrate([
        {
            "id": "001_contacts",
            "sql": "CREATE TABLE contacts (id INTEGER PRIMARY KEY AUTOINCREMENT, email TEXT NOT NULL)",
        },
    ]);
    let app = gebweb.app();
    gebweb.useLogger(app, log.stderr());
    gebweb.registerInstance(app, ContactRepository, ContactRepository(conn));
    return app;
}
```

The app is a local teaching API with no authentication. Add access controls
before exposing private data. The repository borrows the pool; its caller owns
cleanup. This is the standard `src/main.gb`:

<!-- doctest:file main.gb -->
```gb
module main;
import contacts_app;
import db;
import gebweb;
import gebweb.cli as cli;
import sys;

export func main(list<string> args): int {
    cli.checkArgs(args, {"name": "contacts"});
    let conn = db.connect("sqlite", sys.getenv("DATABASE_URL") ?? "contacts.db");
    let app = contacts_app.build(conn);
    gebweb.onShutdown(app, "database", func(): void {
        conn.close();
    });
    gebweb.cli(app, {"name": "contacts", "host": "127.0.0.1"});
    return 0;
}
```

Check arguments before loading secrets or opening resources. The CLI handles
flags, environment defaults and graceful drain. `--help` and `--version` go
directly to the app; do not insert `-- --help`. For full commands and packaging,
see [CLI](15-cli.md) and [deployment](37-deployment.md).

## Routing

`@Controller("/prefix")` opts a class into discovery and supplies its prefix.
Method paths append literally: `@Post("")` is the prefix itself, `@Post("/")`
adds a trailing slash. Use an empty controller prefix for root paths.

Methods include `@Get`, `@Post`, `@Put`, `@Patch`, `@Delete`, `@Options` and
`@Route(["GET", "POST"], "/search")`. GET routes also answer HEAD. Register
explicit controller lists only when you deliberately want explicit wiring.
See [routing](02-routing.md).

## Parameter binding

Use `@PathParam`, `@QueryParam`, `@Header` and `@Body` to make a boundary
explicit. Typed body classes are bound and validated before the handler runs.
A `gebweb.Request` parameter provides request helpers; `request.toDict()` is
the escape hatch for APIs requiring the raw dictionary. An `UploadedFile` or
typed upload dictionary binds multipart files. See [binding](03-parameter-binding.md)
and [uploads](12-file-uploads.md).

## Validation

Use `@Assert.email`, `url`, `uuid`, `notBlank`, `minLength`, `maxLength`,
`range`, `regex` and `in` with their documented arguments. Names are exact and
case-sensitive; unknown assertions fail during app preparation. Custom rules
use `registerAssertion` and return null on success or an error string on failure.

JSON failures become 422 Problem Details. HTML validation redirects need
session setup; errors and old input survive one render. Multipart old input
contains text fields, not uploaded files. See [validation](05-validation.md)
and [forms](25-forms.md).

## Response shapes

| Handler result | Effect |
|----------------|--------|
| Typed output object or collection | JSON response. |
| Native `http.Response` / `gebweb.Response` | Explicit status, headers and body through the builder API. |
| `gebweb.html`, `file`, `stream` helpers | Shaped response dictionaries; declare `dict<string, any>` when returning them directly. |
| `null` | Empty 204 response. |

Controllers extending `gebweb.Controller` call `parent()` in their constructor.
Their `json`, `text`, `created`, `noContent`, `view` and `redirect` helpers return
native responses. Error helpers such as `this.notFound(detail)` throw and
return void: call them as statements. For free functions, use `gebweb.errors`
exceptions or documented exception factories. Never guess a constructor on a
type alias. See [responses](04-responses.md).

## Dependency injection

Register infrastructure before the first discovery/dispatch, when controllers
are built. Explicit controller lists build immediately, so supplying a factory
after that point cannot replace an already injected dependency.

| Registration | Lifetime |
|--------------|----------|
| `register(app, T, factory)` | Shared, built on first resolution. |
| `registerInstance(app, T, instance)` | Shared, supplied instance. |
| `registerPerRequest(app, T, factory)` | One per active request scope. |

A request-scoped service captured by a startup controller can remain shared.
Resolve it within the active request. Keep mutable shared data in a Store or
protect all accesses with an appropriate lock; separate safe operations do not
form a transaction. Use a database for durable or cross-instance invariants.
See [DI](06a-dependency-injection.md), [pools](06b-database-connections.md) and
[concurrency](35-concurrency.md).

`services.yaml` supports parameters, named services, interface bindings, tags
and overlays. A whole `%parameter%` marker preserves its type; embedding a
marker in text produces a string. `dotenv.loadAndApply` overwrites existing
process values. See [configuration](29-services-yaml.md).

## Auth

Use `useAuth(app, UserType, authenticators)` to compose JWT, session, API-key
or custom authenticators. Distinguish absent from rejected credentials. Any
successful authenticator wins; without success, a rejection wins over absence.
`authenticate(app, request)` and route gates share one request decision.

JWT authenticators require explicit `allowedAlgs` and default to required
`sub` and `exp`. Configure issuer/audience, then resolve a current active
account and trusted roles. `jwtIssue` signs supplied claims; it does not query
accounts or provide revocation. Do not pass client claims directly to it.

`@Auth`, `@RequiresRole`, `@RequiresPermission` and `@Can` gate routes. User
injection uses the registered type identity, including imported aliases.
Session auth needs cookie policy and CSRF for browser writes; `useSessionAuth`
also installs normal session/flash integration. See [auth APIs](08-auth.md),
[the login application](40-authentication.md) and [OIDC](33-oidc.md).

## Middleware

`before` runs request hooks and can short-circuit. `use`/`after` register
response hooks, which run in reverse order. Keep streamed/upgraded responses
intact when changing headers. Per-route/controller `@RateLimit`, `@Cors` and
`@MaxBody` configure their documented policies. Limiters group IPv6 clients by
/64. See [middleware](09-middleware.md).

Use [security](27-security.md) as the security overview. `useSecurity` configures
browser headers and CSP nonces; the separate [WAF](27a-waf.md) covers inspection,
byte limits, exclusions, trusted client identity and bans. Neither replaces
authentication or application validation.

## Caching

Use `useCacheStore` with `@Cache(ttl: ..., vary: ..., tags: ...)`, and invalidate
through `cacheInvalidate(app, tag)` when data changes. Framework cache keys
include the negotiated locale. Do not cache private responses without a
suitable identity-aware policy. Separate idempotency stores protect retryable
writes under their documented contract. See [caching and idempotency](10-caching.md).

## Background jobs

Call `useJobs` before registering job handlers or webhooks. Tag discovered
handler classes with `@Tag("job.handler")`; `@Job("name")` marks methods.
Throw to retry, configure queue limits/backoff, and make external effects
idempotent. Workers need the same bootstrap/configuration as producers. Use
`runWorker(app, {"drainOnce": true})` for deterministic drains in tests.
See [jobs](18-background-jobs.md) and [webhooks](34-webhooks.md).

## Events

Events are synchronous and in process. Configure tagged subscribers or
explicit registrations before dispatch. Ordinary subscriber errors aggregate;
an `HttpException` stops dispatch and propagates. Observe the distinction
between application events and framework lifecycle hooks.
See [events](19-events.md).

## Repositories

`@ApiResource` generates CRUD routes from the documented repository contract.
Use `Page` / `Cursor` and typed outputs as required by that contract; do not
invent method names. Optional methods such as count/filter/cursor support
change observable pagination behavior. Tenant scoping and authorization belong
at the repository/data boundary too. See [repositories](06-repositories.md),
[groups](07-serialization-groups.md) and [tenancy](31-multi-tenancy.md).

## Templates

Use `useViews` and `this.view(request, name, context)` so CSRF, flash, old input
and other view providers receive the active request. Default escaping protects
text, not URL scheme policy: validate stored URLs. Reserve `safe`/raw output
for trusted content. Twig-style branches use `elseif`; loops have one variable.
Use documented filters/context helpers, including callable context values such
as the i18n translator. See [views](04a-views.md), [flash](24-flash.md),
[forms](25-forms.md), [assets](26-assets.md) and [i18n](30-i18n.md).

## Testing

Test routes with real dependencies that can run locally. This check uses the
same factory with an in-memory database and verifies validation before writing:

<!-- doctest:run -->
```gb
import contacts_app;
import db;
import gebweb;
import io;

let conn = db.connect("sqlite", ":memory:");
try {
    let app = contacts_app.build(conn);
    let client = gebweb.TestClient(app);
    let created = client.post("/contacts", {"email": "ada@example.test"});
    io.println(created.status);
    io.println(client.get(created.headers["Location"] as string).json()["email"]);
    io.println(client.post("/contacts", {"email": "invalid"}).status);
    io.println((client.get("/contacts").json() as list<any>).length());
} finally {
    conn.close();
}
```

```text
201
ada@example.test
422
1
```

TestClient does not maintain a browser cookie jar; explicitly carry cookies
for sessions and CSRF. Use multipart helpers for upload bodies. Register test
dependencies before constructing the client. Imported classes referenced by
name from a test module must be exported. See [testing](14-testing.md).

## CLI

```sh
gebweb new myapp
cd myapp
geblang install
geblang check --strict src/
geblang test tests/
geblang test --runtime=evaluator tests/
gebweb routes
gebweb build --no-swagger
```

Use `--no-swagger` when the app does not need bundled SwaggerUI assets.
`gebweb dev` restarts for its documented watched inputs; it is not a general
HTML/template watcher. Worker, migration, client-generator and secrets commands
are described in the [CLI chapter](15-cli.md). The CLI executable and the
framework package dependency are installed separately.

## Idioms

- Use typed DTOs at inputs and typed classes/collections for JSON output.
- Keep SQL in repositories that borrow a connection owned by the entry point.
- Configure services, listeners and policies during startup.
- Treat controllers as shared; keep identity and mutable request state local.
- Test both runtimes, then use real HTTP for proxy, listener and binary behavior.

## Anti-idioms

- Do not replace typed binding with unchecked JSON dictionaries inside services.
- Do not assume collection assignment or `copy()` recursively isolates state.
- Do not log request bodies, authorization headers, tokens or provider secrets.
- Do not use cache/session get-then-set as an atomic transaction.
- Do not weaken validation or disable CSRF just to make a test pass.

## When in doubt

Read the linked contract and inspect an observable TestClient response. Use a
real server test when transport behavior matters. The complete
[bookmarks](38-building-bookmarks.md) and [authentication](40-authentication.md)
applications exercise source, cached VM, evaluator and standalone builds.

## Explicit template reload policy

`useViews(app, directory, {"reload": false})` enables permanent caching and
startup preload from source. `reload: true` checks mtimes and keeps preload
lazy, including in bundles. Omit it for the source/bundle defaults. Framework
source changes still require a process restart.

## Environment

`currentEnv()` normalizes `GEBWEB_ENV`: development becomes dev, production
becomes prod, and an unset value is prod. Only dev enables detailed 500 errors
and the profiler by default; an explicit profiler option can override its gate.
CSP nonce policies apply to the bar's inline tags. See [profiler](36-profiler-bar.md).

## Shutdown and logging

Register named `onShutdown` hooks for owned resources. Hooks run once, latest
first, after listeners and in-process jobs drain; failure does not skip later
hooks. `shutdown` reports whether shutdown was clean. The CLI exits 1 on failure
and 130 on a second signal. Keep a timeout budget for cleanup.

Framework logs use the app logger; `GEBWEB_LOG_FORMAT=json` selects JSON output.
Use `useLogger` to supply a sink. Development requests are logged by default
unless request logging middleware is already installed.
See [deployment](37-deployment.md) and [health/ops](32-health-checks.md).

## Lifecycle hooks

`on(app, name, listener, priority = 0)` returns an id for `off`. Configure before
serving; higher priorities run first. Request start/response listeners can
reject with an `HttpException`. Auth listeners are observers, and finish/error
observers cannot skip finalizers. Preserve response status when adding headers
to WebSocket upgrades or streams. See [lifecycle events](19-events.md#framework-lifecycle-events).

## Reference

Use the [manual index](00-index.md) for the full API map, including
[plugins](16-plugins.md), [OpenAPI](13-openapi.md), [mail](20-mailer.md),
[storage](21-storage.md), [messaging](22-messaging.md) and [LLM integration](28-llm.md).

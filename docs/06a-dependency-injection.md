# Dependency injection

Gebweb wires application objects through a constructor-injection
container. The framework constructs controller, repository, and
discovery-scanned classes for you by walking their declared
constructor parameters and resolving each one to a registered
service. You can hand-register services with explicit factories
or instances, and you can read services out of the container
directly when DI is not in the loop.

This chapter walks the full surface: what the framework
constructs, the lifecycle model, the register / resolve methods,
how parameter binding interacts, and the patterns you reach for
in tests.

## Automatic discovery

When an app first starts handling requests (`serve`, `listen`,
`TestClient`, or `dispatcher`), Gebweb sweeps every loaded class once
and wires the ones carrying a marker, so most registration is
automatic:

```gb
import gebweb;

@Controller
class HealthController {
    @Get("/health")
    func check(): dict<string, any> { return {"status": "ok"}; }
}

@Service
class Clock { func now(): int { return 0; } }

class DocPolicy {
    @Policy("Doc")
    func edit(CurrentUser u, Doc d): bool { return u.id == d.ownerId; }
}

let app = gebweb.app();        // no controller list needed
gebweb.serve(app, ":8080");    // HealthController, Clock, DocPolicy all wired
```

The sweep handles `@Controller` (mounted), `@Policy` methods
(registered as authorization policies), `@Service` (DI container), and
`@Tag("event.subscriber" | "job.handler" | "message.handler" |
"scheduled")` handlers. It runs at most once per app and logs a
one-line summary at startup.

`@Controller` classes are auto-mounted only when `gebweb.app()` is
called with no controller list. If you pass an explicit list
(`gebweb.app([A, B])`), that is the complete controller set and no
other `@Controller` classes are mounted, but services, policies, and
tagged handlers are still discovered. A controller that appears in both
the explicit list and the scan is registered once.

Calling `gebweb.discover(app)` yourself is optional and idempotent; it
returns a `DiscoveryReport` listing the wired controllers, services,
policies, and tagged classes.

### Caching the sweep

For large apps the scan can be cached to trim startup. It is opt-in
because it writes a file:

```gb
gebweb.serve(app, ":8080", {"discoveryCache": true});
/* or a custom path: {"discoveryCache": ".cache/discovery.json"} */
```

The first run writes a manifest to `.gebweb-cache/discovery.json` (add
it to `.gitignore`); later runs reload it and skip the scan when the set
of loaded classes is unchanged, matched by a fingerprint. Adding or
removing a class invalidates the cache automatically; a decorator-only
change on an existing class does not, so delete the file (or redeploy
clean) after one. `gebweb.discoverCached(app, path)` runs the same
cached sweep directly.

## What the framework constructs

The container kicks in whenever Gebweb needs an instance:

- Controllers passed as class references to `gebweb.app([...])`.
  An entry like `gebweb.app([UserController])` resolves
  `UserController` through the container.
- Repositories referenced by `@ApiResource` (the framework
  resolves the repo class declared by the resource's static
  `repositoryClass()` method).
- Classes discovered through `reflect.classes()` for messaging
  / job / scheduled-task / event subscribers when a framework
  feature is enabled. The scanner asks the container for
  instances of each handler class.

In all three paths the container walks the constructor's typed
parameter list and resolves each parameter before instantiating
the class.

```gb
class UserRepo {
    db.Connection conn;
    func UserRepo(db.Connection conn) {
        this.conn = conn;
    }
}

class UserController {
    UserRepo repo;
    func UserController(UserRepo repo) {
        this.repo = repo;
    }

    @Get("/users")
    func index(): list<User> {
        return this.repo.findAll();
    }
}

let app = gebweb.app([UserController]);
gebweb.register(app, db.Connection, func(): db.Connection {
    return db.Connection({
        "driver": "postgres",
        "dsn": "...",
        "maxOpenConns": 16,
    });
});

/* `gebweb.app` queued UserController for construction; the
 * container instantiates it lazily on the first request, walks
 * its ctor, sees `UserRepo`, recursively resolves it, which
 * in turn resolves `db.Connection` from the factory. */
```

A singleton `db.Connection` is the right default: it wraps a connection
pool, and concurrent requests query it in parallel. Size `maxOpenConns`
to your expected request concurrency (geblang 1.19.0 applies pool
options at connect time).

## Lifecycle model

Three lifecycles:

| Lifecycle | API | Behaviour |
|-----------|-----|-----------|
| App singleton (default) | `register(app, T, factory)` | Factory runs once on first resolve; result is cached for the rest of the process. |
| Pre-built instance | `registerInstance(app, T, value)` | Skip the factory and use the value directly. Caching identical to app singleton. |
| Per request | `registerPerRequest(app, T, factory)` | Inside a request scope (set up automatically by the request dispatcher), the factory runs once per request and the result is cached for that request only. Outside a scope (e.g. test bootstrap), every resolve calls the factory fresh (transient behaviour). |

Pick by service shape:

- DB connection / connection pool / cache client / mailer
  transport: app singleton. Cheap to share, no per-request state.
- A `RequestContext` that holds the current user + tenant +
  trace id, or a `TransactionManager` that wraps the active
  request's DB transaction: per-request.
- Anything that you want to stub in a test: register the stub
  with `registerInstance`.

There is no transient-by-default mode. If you genuinely want a
fresh instance per resolve, register it as per-request and never
enter a request scope, or use `registerInstance` with a different
value each time you swap.

## Registering services

### `gebweb.register(app, T, factory)`

Lazy factory. The factory takes no arguments and returns the
service value. It is invoked the first time the container is
asked for `T`; the result is cached for subsequent resolves.

```gb
gebweb.register(app, db.Connection, func(): db.Connection {
    return db.connect("postgres", gebweb.parameter(app, "db.url") as string);
});
```

Use a factory when construction has side effects (opens a
connection, reads a file), depends on runtime configuration
that the container cannot infer, or produces a value the
container cannot assemble via reflection.

### `gebweb.registerInstance(app, T, instance)`

Skip the factory. The container caches the supplied instance
directly. Common for tests:

```gb
let stub = StubMailer();
gebweb.registerInstance(app, gebweb.MailerContract, stub);
```

Also useful when you have an already-constructed value that
multiple services should share (a long-lived broker connection,
a Bedrock client built at startup).

### `gebweb.registerPerRequest(app, T, factory)`

Per-request scope. The dispatcher opens a fresh request scope
for every incoming HTTP request, so within one request the
factory runs once and the result is shared across every service
that asks for `T` during that request. Once the response is
flushed the scope closes and the cache is discarded.

```gb
class RequestContext {
    string userId;
    string tenant;
    func RequestContext(string userId, string tenant) {
        this.userId = userId;
        this.tenant = tenant;
    }
}

gebweb.registerPerRequest(app, RequestContext, func(): RequestContext {
    /* Read whatever the auth middleware put on the request. The
     * dispatcher exposes the active request through a stdlib
     * thread-local; see the Auth chapter for the helper. */
    let req = gebweb.activeRequest();
    return RequestContext(req["userId"] as string, req["tenant"] as string);
});
```

Outside a request scope the factory runs on every resolve
(transient behaviour). That makes test code straightforward:
exercise the same service from several test methods and each
gets its own instance.

### `di.registerInterfaceInstance(c, "Pkg.Iface", instance)`

Register an instance keyed by an interface type name. Necessary
because Geblang interfaces cannot be referenced as first-class
values at runtime, so the class-keyed `registerInstance(c, T, ...)`
form does not work for interface-typed parameters.

This is mostly an internal hook: gebweb uses it under the bonnet
to register framework-supplied services that expose an interface
(`gebweb.useLlm(app, client)` registers under `"llm.Client"`).
Application code reaches for it directly when a custom service
also lives behind an interface:

```gb
interface Notifier {
    func send(string subject, string body): void;
}

class EmailNotifier implements Notifier {
    /* ... */
}

/* Note: di is gebweb.di. Pull it through an import alias. */
import gebweb.di as di;

di.registerInterfaceInstance(app.container, "Notifier", EmailNotifier());
```

Constructor parameters typed `Notifier` now resolve to the
registered instance:

```gb
class AlertService {
    Notifier notifier;
    func AlertService(Notifier notifier) { this.notifier = notifier; }
}

let svc = gebweb.resolve(app, AlertService) as AlertService;
```

## Constructor autowiring

When the container resolves a class `T`, it does the following:

1. If `T` has a registered factory, call it and cache the result.
2. If `T` is registered as a per-request binding and a request
   scope is active, return the request-cached instance (or build
   one and cache it for the request).
3. Otherwise inspect `T`'s constructor parameters via
   reflection. For each parameter, in order:
   - If the parameter has a `@Param("key")` decorator, resolve
     it from the app's parameter store.
   - Else if the parameter's declared type is an interface name
     registered via `registerInterfaceInstance`, return that
     instance.
   - Else resolve the parameter's declared class type
     recursively through the same rules. Cycles raise a
     `RuntimeError` naming the resolution stack.
4. Invoke `T(args...)` with the resolved arguments and cache.

Parameter resolution does not care about parameter order beyond
declaration order; you can mix class-typed and `@Param`-typed
parameters in the same constructor.

### `@Param("key")` for primitive config

`@Param("key")` pulls from the app's parameter store instead of
the type system. Use it for primitives that the container cannot
auto-build: database URLs, secrets, feature flags, region names.

```gb
class S3Bucket {
    string bucket;
    string region;
    func S3Bucket(@Param("s3.bucket") string bucket,
                  @Param("aws.region") string region) {
        this.bucket = bucket;
        this.region = region;
    }
}

gebweb.parameter(app, "s3.bucket", "uploads-prod");
gebweb.parameter(app, "aws.region", "us-east-1");

let svc = gebweb.resolve(app, S3Bucket) as S3Bucket;
```

A missing key raises a `RuntimeError` naming the key, so typos
surface immediately rather than producing a `null` deep in your
service.

### Interface-typed parameters

If a constructor depends on an interface, register an instance
under the interface name (as above) and the container does the
rest:

```gb
class Pipeline {
    Notifier notifier;
    llm.Client llm;
    func Pipeline(Notifier notifier, llm.Client llm) {
        this.notifier = notifier;
        this.llm = llm;
    }
}

di.registerInterfaceInstance(app.container, "Notifier", EmailNotifier());
gebweb.useLlm(app, gebweb.llmClient({"provider": "anthropic", "apiKey": "..."}));

let p = gebweb.resolve(app, Pipeline) as Pipeline;
```

`gebweb.useLlm` registers the LLM client under `"llm.Client"`
internally; the user-side custom `Notifier` is wired the same
way.

## Resolving manually

`gebweb.resolve(app, T)` returns the cached or freshly built
instance of `T`. Use it from places the autowiring chain cannot
reach (a one-off script, the inside of a controller method that
needs an extra service it does not want as a constructor
dependency):

```gb
class ImportController {
    @Post("/import")
    func run(ImportRequest body): dict<string, any> {
        let parser = gebweb.resolve(this.app, CsvParser) as CsvParser;
        return parser.parse(body.payload);
    }
}
```

`gebweb.resolve` returns `any`; cast to the expected type at the
call site.

There is no `resolve-by-interface-name` helper for runtime
lookup. The framework's own pattern is to expose a typed getter
alongside the registration (e.g. `gebweb.llm(app)` for the LLM
client, `gebweb.mailer(app)` for the mailer transport). Custom
services follow the same pattern: pair `useFoo` with `foo` for
read-back.

## Testing patterns

The DI container is the seam tests use to swap real
dependencies for controllable stubs. The pattern is the same
across handler tests, repository tests, and integration tests
against `TestClient`:

```gb
class WidgetControllerTest extends test.Test {
    @test
    func deleteReturns204(): void {
        let app = gebweb.app([WidgetController]);
        let stub = StubWidgetRepo();
        gebweb.registerInstance(app, WidgetRepo, stub);

        let client = gebweb.TestClient(app);
        let r = client.delete("/widgets/w-1");
        r.assertStatus(204);
        this.assertEquals(["w-1"], stub.deleted);
    }
}
```

Three guidelines:

- Register stubs BEFORE the first resolve. Once the container
  has cached an instance, subsequent `registerInstance` calls
  for the same key overwrite the binding but instances already
  injected into other services keep the original. Calling
  `registerInstance` immediately after `gebweb.app([...])`
  guarantees the stub is in place before the first request.
- Prefer `registerInstance` over `register` in tests when you
  do not need lazy construction. The instance form makes the
  setup linear and inspectable.
- For per-request services, your test does not enter a request
  scope unless you go through `TestClient`. A direct
  `gebweb.resolve(app, RequestContext)` from test code returns
  a fresh instance each time, which is usually what tests want.

## Common patterns

### Pass the active app into a controller

Controllers do not get the `GebwebApp` injected unless they
declare it. Pattern: declare a `?gebweb.GebwebApp app` field and
let the framework call the controller's `bindApp(app)` method
(the `gebweb.Controller` base class implements this hook
already). Controllers extending `gebweb.Controller` can then
call `gebweb.resolve(this.app, T)` from any handler.

### Chained services

A service that depends on another service just declares it in
the constructor. The container resolves transitively:

```gb
class SearchIndex {
    db.Connection conn;
    func SearchIndex(db.Connection conn) { this.conn = conn; }
}

class SearchService {
    SearchIndex index;
    Notifier notifier;
    func SearchService(SearchIndex index, Notifier notifier) {
        this.index = index;
        this.notifier = notifier;
    }
}

/* db.Connection is registered; Notifier is registered by
 * interface name; SearchIndex is built automatically by walking
 * its ctor; SearchService is built by resolving its two deps. */
let svc = gebweb.resolve(app, SearchService) as SearchService;
```

You do not need to register `SearchIndex` or `SearchService`
explicitly; the container constructs them lazily when something
asks for them.

### Wrapping an external client

External clients (LLM, S3, broker) usually come in as instances
from a `useXxx` helper. The helper both stores the client on the
app and registers it for DI. Your services depend on the client
type normally:

```gb
gebweb.useLlm(app, gebweb.llmClient({"provider": "anthropic", "apiKey": "..."}));

class Summariser {
    llm.Client llm;
    func Summariser(llm.Client llm) { this.llm = llm; }
}
```

## Declarative configuration

Beyond the programmatic surface in this chapter, gebweb reads a
`config/services.yaml` file at app construction time (when the
file exists). The YAML loader can register services with arg
overrides, tag them, bind interfaces to chosen implementations,
load parameters with env/secret/cross-reference markers, and
merge per-environment overlays. See
[services.yaml](29-services-yaml.md) for the full surface; the
two paths interoperate, so anything you can do in YAML you can
also do via `gebweb.register*` / `di.bindInterface` / `di.tagService`.

## Reference

| API | Purpose |
|-----|---------|
| `gebweb.register(app, T, factory)` | App-singleton factory. |
| `gebweb.registerInstance(app, T, instance)` | App-singleton instance. |
| `gebweb.registerPerRequest(app, T, factory)` | Per-request scope (transient outside a scope). |
| `di.registerInterfaceInstance(c, "Pkg.Iface", instance)` | Register under an interface name. `c` is `app.container`. |
| `di.bindInterface(c, "Pkg.Iface", "service.id")` | Bind an interface to a registered service id (lazy resolve through `gebweb.service`). |
| `gebweb.resolve(app, T)` | Build or return the cached instance of `T`. |
| `gebweb.service(app, id)` | Resolve a service by its registered id (`@Service` or YAML). |
| `gebweb.parameter(app, key, value)` | Set a primitive value in the parameter store. |
| `gebweb.parameter(app, key)` | Read a value from the parameter store. |
| `@Param("key")` | Pull the value at `key` into the annotated constructor parameter. |
| `@Service("custom.id")` | Mark a class for the discovery sweep; the id defaults to the class name. |
| `gebweb.useLlm` / `gebweb.useMailer` / `gebweb.useStorage` / `gebweb.useCacheStore` / `gebweb.useViews` / `gebweb.useMessageQueue` / ... | Built-in `use*` helpers register their service on the app and (where applicable) plug it into the DI container under the right key. Look for a paired `gebweb.<noun>(app)` getter for read-back without going through DI. |

### Errors

| Symptom | Likely cause |
|---------|--------------|
| `cannot resolve X: parameter 'p' of type 'T' is not a known class` | `T` has no factory, no class registration, and no interface-name registration. Either add `gebweb.register(app, T, ...)` or `di.registerInterfaceInstance(c, "T", ...)`. |
| `gebweb parameter 'key' is not registered` | A `@Param("key")` constructor parameter was hit but `gebweb.parameter(app, "key", ...)` was never called. |
| `DI cycle detected resolving X (stack: X -> Y -> X)` | Circular dependency. Break the cycle by injecting an interface and registering at startup, or by passing one of the two services as a factory closure rather than a constructor parameter. |

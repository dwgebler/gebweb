# Plugins

<!-- doctest:strict -->

A plugin packages application setup behind a named object. It can install
middleware, register services, add routes or configure authentication through
the public Gebweb API. Use one when several applications need the same setup,
or when a feature needs configuration and a small API of its own. A single
middleware function does not need a plugin wrapper.

## A response-tagging plugin

This plugin adds a response header and counts responses by request path.
Configuration stays on the instance; counters use atomic `store.Store`
operations because middleware runs concurrently across requests.

Put it in `src/response_tag.gb`:

<!-- doctest:file response_tag.gb -->
```gb
import gebweb;
import http;
import store;

export class ResponseTagPlugin extends gebweb.Plugin {
    string headerValue;
    store.Store counters;

    func ResponseTagPlugin(string headerValue) {
        parent("response-tag");
        this.headerValue = headerValue;
        this.counters = store.Store();
    }

    func install(any application): void {
        let app = application as gebweb.GebwebApp;
        this.counters.incr("installs");
        let self = this;
        gebweb.use(app, func(gebweb.Request request, http.Response response): http.Response {
            self.counters.incr("path:" + request.path());
            return response.withHeader("X-Application", self.headerValue);
        });
    }

    func responses(string path): int {
        return (this.counters.get("path:" + path) ?? 0) as int;
    }

    func installations(): int {
        return (this.counters.get("installs") ?? 0) as int;
    }
}
```

Install it while constructing the app, before creating a `TestClient` or
starting the HTTP server:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import response_tag;

class Health {
    bool ok = true;
}

@Controller("/health")
class HealthController {
    @Get("")
    func show(): Health {
        return Health();
    }
}

let app = gebweb.app();
let tag = response_tag.ResponseTagPlugin("catalog");
gebweb.plugin(app, tag);

let client = gebweb.TestClient(app);
let response = client.get("/health");
io.println(response.status);
io.println(response.headers["X-Application"]);
io.println(response.text());
client.get("/health");
io.println(tag.responses("/health"));
io.println(tag.installations());
```

```text
200
catalog
{"ok":true}
2
1
```

`gebweb.Plugin` supplies `name()` and a no-op `install(any app): void`.
Its constructor accepts the name, so subclasses call `parent("name")` and
override `install`. The plugin above uses `withHeader`, which returns a changed
response while preserving its status and body.

Response middleware follows the usual dispatch rules: it runs after a handler
or a request-middleware short-circuit, in reverse registration order. It is not
an unconditional request finalizer and does not see every failed dispatch. The
counter above measures responses that reach this middleware, not all incoming
requests. See [Middleware](09-middleware.md).

## Implementing the contract directly

A class can implement `gebweb.PluginContract` instead of extending the base
class. It must provide both `name(): string` and `install(any app): void`.
Having methods with those names alone does not declare the interface.

<!-- doctest:run -->
```gb
import gebweb;
import io;

class FeaturePlugin implements gebweb.PluginContract {
    func name(): string {
        return "feature-flags";
    }

    func install(any application): void {
        let app = application as gebweb.GebwebApp;
        gebweb.parameter(app, "features.search", true);
    }
}

let app = gebweb.app();
gebweb.plugin(app, FeaturePlugin());
io.println(gebweb.getParameter(app, "features.search"));
let found = gebweb.findPlugin(app, "feature-flags");
if (found != null) {
    io.println(found.name());
}
io.println(gebweb.findPlugin(app, "missing") == null);
```

```text
true
feature-flags
true
```

`install` receives the application as `any`; cast it to `gebweb.GebwebApp`
before using the typed facade helpers. Keep this signature when implementing
the contract or overriding the base method.

## Registration and lookup

`gebweb.plugin(app, instance)` appends the instance to the app's plugin list,
then calls `install(app)` synchronously. It returns the same app for setup
composition. It does not discover plugin classes, instantiate them through DI
or wait until the first request.

| Behavior | Rule |
|----------|------|
| Installation | Once per call to `gebweb.plugin`, including repeated calls with the same instance. |
| Names | Supplied by `name()`; choose a stable name for peer lookup. |
| Duplicate names | Allowed; every registration still installs. |
| Lookup | `gebweb.findPlugin(app, name)` returns the first registered match, or `null`. |
| Lookup order | Registration order; names are compared exactly. |
| Installation failure | The exception propagates; the instance has already been appended. |

For example, duplicate names do not replace an earlier plugin or suppress its
installation:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import response_tag;

let app = gebweb.app();
let first = response_tag.ResponseTagPlugin("first");
let second = response_tag.ResponseTagPlugin("second");
gebweb.plugin(app, first);
gebweb.plugin(app, second);
gebweb.plugin(app, first);

io.println(first.installations());
io.println(second.installations());
let found = gebweb.findPlugin(app, "response-tag");
if (found != null) {
    io.println((found as response_tag.ResponseTagPlugin).headerValue);
}
```

```text
2
1
first
```

Use inferred types for a lookup result and cast to the concrete plugin class
when you need its extra API. Prefer passing a dependency explicitly when you
can; name lookup is useful for optional integration, not a substitute for a
plugin's required constructor arguments.

## Setup order and resource ownership

Install plugins in the application's `build(conn)` factory, alongside service
registration and other framework setup, before calling `gebweb.cli`. Short
examples use `TestClient` to exercise that configuration without a listener;
see [Getting started](01-getting-started.md) for the full entry-point pattern.

- With `gebweb.app()` discovery, install service-registering plugins before
  discovery runs. Creating a `TestClient`, explicitly calling `gebweb.discover`
  or asking for routes can trigger discovery and construct controllers.
- An explicit `gebweb.app([ControllerClass])` list constructs its controllers
  immediately. A plugin installed afterward cannot supply their constructor
  dependencies retroactively. Prefer discovery and register dependencies first.
- When one plugin requires another, register the dependency first and validate
  it during setup. There is no dependency resolver or automatic install order.
- A plugin that opens a pool or another long-lived resource should register a
  named `gebweb.onShutdown` hook to close it. The plugin contract has no
  separate stop or uninstall method. See [Deployment](37-deployment.md).
- Treat configuration as immutable once requests begin. For shared writes,
  use `store.Store`, a database or another concurrency-safe service. Keep
  request-specific data out of plugin fields.

Installation is not transactional. If a plugin throws after registering a
service or middleware, that setup is not rolled back, and the plugin remains
visible to `findPlugin`. Let startup fail and fix its configuration rather than
catching the exception and serving a partially configured app. Re-registering
can add the same middleware twice.

The plugin mechanism adds no sandbox or permission boundary. Install trusted
code, and use the same capability and configuration controls as the application
that loads it. Runtime-dependent features can also expose ordinary setup
functions; not every optional framework facility is a `Plugin` subclass.

## Reference

| API | Contract |
|-----|----------|
| `gebweb.Plugin(name)` | Convenience base class with a stored name and no-op `install`. |
| `gebweb.PluginContract` | Interface requiring `name(): string` and `install(any app): void`. |
| `gebweb.plugin(app, instance)` | Append and immediately install a plugin; return the app. |
| `gebweb.findPlugin(app, name)` | Return the first matching registered plugin or `null`. |

The plugin mechanism has no options dictionary. Plugin constructors define
and validate their own configuration; the first example's `headerValue` is a
required string and has no default.

Related chapters: [Dependency injection](06a-dependency-injection.md),
[Middleware](09-middleware.md), [Testing](14-testing.md),
[Concurrency and shared state](35-concurrency.md), and
[Deployment](37-deployment.md).

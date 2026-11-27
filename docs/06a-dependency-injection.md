# Dependency injection

<!-- doctest:strict -->

Gebweb builds controllers and services for you. A class declares what it
needs as typed constructor parameters, and the app's container supplies them:
classes it can build itself, instances you registered, configuration values
and interface implementations. Code depends on its collaborators' types and
never constructs or looks them up itself, which keeps wiring in one place and
makes every class easy to test with a different collaborator.

## How the container builds objects

<!-- doctest:run -->
```gb
import gebweb;
import io;

class Clock {
    func now(): string {
        return "09:30";
    }
}

class GreetingService {
    Clock clock;
    string siteName;

    func GreetingService(Clock clock, @Param("site.name") string siteName) {
        this.clock = clock;
        this.siteName = siteName;
    }

    func greet(string who): string {
        return "${this.siteName} welcomes ${who} at ${this.clock.now()}";
    }
}

@Controller("/greet")
class GreetingController extends gebweb.Controller {
    GreetingService greetings;

    func GreetingController(GreetingService greetings) {
        parent();
        this.greetings = greetings;
    }

    @Get("/{who}")
    func greet(string who): dict<string, any> {
        return {"message": this.greetings.greet(who)};
    }
}

let app = gebweb.app();
gebweb.parameter(app, "site.name", "Library");
io.println(gebweb.TestClient(app).get("/greet/ada").text());
```

Output:

```text
{"message":"Library welcomes ada at 09:30"}
```

To build `GreetingController`, the container resolves each constructor
parameter in turn:

1. A parameter marked `@Param("key")` gets that key's configuration value.
2. A parameter typed as an interface gets the instance registered for it.
3. A parameter typed as a class gets that class from the container: a
   registered instance or factory if there is one, otherwise one built the same
   way from its own constructor.

Nothing above was registered except the `site.name` parameter: `Clock` and
`GreetingService` were built from their constructors. A dependency cycle
(`A` needs `B`, `B` needs `A`) fails with an error naming the chain.

## Lifetimes

By default each class is built once per app and the instance is shared by
everything that asks for it, across all requests. That suits stateless
services, repositories and clients that hold a connection pool. Requests run
in parallel, so a shared object must not keep per-request state in its fields;
see [Concurrency and shared state](35-concurrency.md).

| Lifetime | How to get it | What each resolve returns |
|----------|---------------|---------------------------|
| Shared (default) | Build from the constructor, or `register`, or `registerInstance` | The same instance for the life of the app |
| Per request | `registerPerRequest(app, T, factory)` | One instance per HTTP request; a new one for the next request |
| New every time | A `services.yaml` entry with `shared: false` | A new instance on every resolve |

Controllers are built once, when the app starts handling requests, so they
should hold only shared dependencies. To use a per-request service, ask the
container for it during the request. Declaring a `gebweb.GebwebApp`
constructor parameter gives a class the app it belongs to:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class AuditTrail {
    list<string> events = [];

    func record(string event): int {
        this.events.push(event);
        return this.events.length();
    }
}

@Controller("/orders")
class OrderController extends gebweb.Controller {
    gebweb.GebwebApp app;

    func OrderController(gebweb.GebwebApp app) {
        parent();
        this.app = app;
    }

    @Post("")
    func create(): dict<string, any> {
        let audit = gebweb.resolve(this.app, AuditTrail) as AuditTrail;
        audit.record("validated");
        return {"events": audit.record("saved")};
    }
}

let app = gebweb.app();
gebweb.registerPerRequest(app, AuditTrail, func(): AuditTrail { return AuditTrail(); });
let client = gebweb.TestClient(app);
io.println(client.post("/orders", {}).text());
io.println(client.post("/orders", {}).text());
```

Output:

```text
{"events":2}
{"events":2}
```

Each request got its own `AuditTrail`, so the second request did not see the
first one's events. Outside a request, such as in a test calling `resolve`
directly, a per-request factory runs on every resolve. Never inject a
per-request service into a constructor: the constructor runs once, so it would
keep the first instance for every request.

## Registering what the container cannot build

### Instances and factories

Register an instance when the object needs values the container cannot
supply, such as a database connection, or when it already exists:

<!-- doctest:fragment context=app -->
```gb
class NoteRepository {
    db.Connection conn;

    func NoteRepository(db.Connection conn) {
        this.conn = conn;
    }
}

gebweb.registerInstance(app, NoteRepository, NoteRepository(conn));
```

`register(app, T, factory)` registers a function instead; it runs on the first
resolve and its result is shared from then on. Use it when building the object
is expensive and not every run needs it. Register before the app handles its
first request: controllers are built then, and an instance already injected
into a controller is not replaced by a later registration. A native type such
as `db.Connection` cannot be registered directly; register the class that
holds it, as above.

### Configuration values

`@Param("key")` on a constructor parameter reads a value from the app's
parameter store, set with `gebweb.parameter(app, key, value)` or loaded from
`config/services.yaml`. Use it for strings, numbers and flags that a class
needs but the container cannot infer. A missing key is an error naming the
key when the class is built.

### Interfaces

A parameter typed as an interface needs an implementation registered for that
interface's name:

<!-- doctest:run -->
```gb
import gebweb;
import gebweb.di as di;
import io;

interface Notifier {
    func send(string message): string;
}

class EmailNotifier implements Notifier {
    func send(string message): string {
        return "emailed: ${message}";
    }
}

class StockAlerts {
    Notifier notifier;

    func StockAlerts(Notifier notifier) {
        this.notifier = notifier;
    }

    func lowStock(string sku): string {
        return this.notifier.send("${sku} is low");
    }
}

let app = gebweb.app([]);
di.registerInterfaceInstance(app.container, "Notifier", EmailNotifier());
io.println((gebweb.resolve(app, StockAlerts) as StockAlerts).lowStock("B-12"));
```

Output:

```text
emailed: B-12 is low
```

For an interface from another module, use its qualified name, such as
`"notify.Notifier"`. [services.yaml](29-services-yaml.md) can bind interfaces
to services declaratively. Built-in integrations register themselves this way:
`gebweb.useLlm(app, client)` makes an `llm.Client` parameter work.

### Named services

`@Service("id")` registers a class under an id when discovery runs, and
`gebweb.service(app, id)` returns it. Ids are mainly for
[services.yaml](29-services-yaml.md) references and tags; constructor
parameters resolve by type and need no id.

## Resolving by hand

`gebweb.resolve(app, T)` returns the container's instance of `T`, building it
if needed; cast the result to `T`. Use it at the edges: in `main`, in a test,
or for a per-request service as shown above. Elsewhere, take dependencies as
constructor parameters, which keeps them visible and replaceable.

## Testing

Register test doubles before the app handles its first request, then drive
the real routes:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class PaymentGateway {
    func charge(int cents): string {
        return "charged ${cents} for real";
    }
}

class FakeGateway extends PaymentGateway {
    list<int> charges = [];

    func charge(int cents): string {
        this.charges.push(cents);
        return "fake-1";
    }
}

@Controller("/checkout")
class CheckoutController {
    PaymentGateway gateway;

    func CheckoutController(PaymentGateway gateway) {
        this.gateway = gateway;
    }

    @Post("")
    func pay(): dict<string, any> {
        return {"receipt": this.gateway.charge(4200)};
    }
}

let fake = FakeGateway();
let app = gebweb.app();
gebweb.registerInstance(app, PaymentGateway, fake);
io.println(gebweb.TestClient(app).post("/checkout", {}).text());
io.println(fake.charges);
```

Output:

```text
{"receipt":"fake-1"}
[4200]
```

Assert on responses and on what the fake recorded, not on which methods were
called in which order. See [Testing](14-testing.md).

## Discovery and its cache

Discovery finds `@Controller`, `@Service` and handler classes among the loaded
modules when the app first handles a request; see
[App construction](01-getting-started.md#app-construction). For a large app,
the result can be cached to shorten startup:

<!-- doctest:fragment context=app -->
```gb
gebweb.serve(app, "127.0.0.1:8080", {"discoveryCache": true});
```

The first run writes `.gebweb-cache/discovery.json` (keep it out of version
control); later runs reuse it while the set of loaded classes is unchanged. A
change only to a class's decorators does not invalidate it, so delete the file
after one. `gebweb.discoverCached(app, path)` runs the cached sweep directly.

## Errors

| Message | Cause |
|---------|-------|
| `cannot resolve X: parameter 'p' of type 'T' is not a known class (register a factory for it)` | `T` is a native or primitive type the container cannot build; register the class that needs it, or use `@Param`. |
| `gebweb: @Param("key") requested but no parameter is registered (call gebweb.parameter(app, "key", ...))` | The configuration key was never set. |
| `DI cycle detected resolving A (stack: A -> B)` | `A` and `B` need each other; break the cycle by passing one to the other through a method instead. |

## Reference

| API | Purpose |
|-----|---------|
| `gebweb.register(app, T, factory)` | Shared instance built by `factory` on first resolve. |
| `gebweb.registerInstance(app, T, instance)` | Shared, existing instance. |
| `gebweb.registerPerRequest(app, T, factory)` | One instance per HTTP request. |
| `gebweb.resolve(app, T): any` | The container's instance of `T`. |
| `gebweb.parameter(app, key, value)` / `gebweb.getParameter(app, key)` | Set and read configuration values. |
| `@Param("key")` | Inject a configuration value into a constructor parameter. |
| `@Service("id")` / `gebweb.service(app, id)` | Register a class under an id; look it up. |
| `di.registerInterfaceInstance(app.container, name, instance)` | Implementation for an interface-typed parameter. |
| `di.bindInterface(app.container, name, serviceId)` | Bind an interface to a registered service id. |
| `gebweb.GebwebApp` constructor parameter | The app the class belongs to. |

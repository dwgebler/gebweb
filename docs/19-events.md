# Events

<!-- doctest:strict -->

Gebweb events are synchronous, in-process notifications. A publisher supplies
an event name and payload; registered listeners run before `gebweb.publish`
returns. Use them for application extension points and notifications that belong
in the current operation. Use a durable job when work needs retries or must
survive process failure.

## Publishing from a controller

This endpoint returns a typed greeting and publishes `greeting.created`. A
subscriber records a count using `store.Store`, which is safe when requests run
concurrently. Discovery constructs the controller and tagged subscriber through
the same DI container.

<!-- doctest:run -->
```gb
import gebweb;
import io;
import store;

class AuditTotals {
    store.Store totals;

    func AuditTotals() {
        this.totals = store.Store();
    }

    func record(string name): void {
        this.totals.incr(name);
    }

    func count(string name): int {
        return (this.totals.get(name) ?? 0) as int;
    }
}

@Tag("event.subscriber")
class AuditSubscriber {
    AuditTotals audit;

    func AuditSubscriber(AuditTotals audit) {
        this.audit = audit;
    }

    @On("greeting.created")
    func record(gebweb.Event event): void {
        this.audit.record(event.name);
    }
}

class GreetingInput {
    @Assert.notBlank
    string name;
}

class Greeting {
    string message;

    func Greeting(string name) {
        this.message = "Hello, " + name;
    }
}

@Controller("/greetings")
class GreetingController {
    gebweb.GebwebApp app;

    func GreetingController(gebweb.GebwebApp app) {
        this.app = app;
    }

    @Post("")
    func create(GreetingInput body): Greeting {
        let greeting = Greeting(body.name);
        gebweb.publish(this.app, "greeting.created", {"name": body.name});
        return greeting;
    }
}

let app = gebweb.app();
let audit = AuditTotals();
gebweb.useEvents(app);
gebweb.registerInstance(app, AuditTotals, audit);
let response = gebweb.TestClient(app).post("/greetings", {"name": "Ada"});
io.println(response.status);
io.println(response.text());
io.println(audit.count("greeting.created"));
```

```text
200
{"message":"Hello, Ada"}
1
```

`@Tag("event.subscriber")` marks the service for discovery; `@On` marks its
listener methods. Configure the event bus and required services before
triggering discovery. For an application, do that in `build(conn)` before
`gebweb.cli`; see [Getting started](01-getting-started.md) for the entry-point
pattern. The short example uses `TestClient` to avoid opening a listener.

## Subscriber registration and event data

Call `gebweb.useEvents(app)` before `gebweb.publish` or
`gebweb.registerEventSubscribers`. Repeated `useEvents` calls preserve existing
registrations. A publish with no matching listeners is a no-op once the bus
exists; publishing without a configured bus raises an error.

Alternatively, pass already-constructed subscriber instances to
`gebweb.registerEventSubscribers(app, instances)`. This is useful for explicit
composition and tests. Do not also discover the same instance's class through
a subscriber tag, or its methods can be registered twice.

A method may stack `@On` decorators to subscribe to several exact event names:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class UserAudit {
    @On("user.created")
    @On("user.deleted")
    func record(gebweb.Event event): void {
        io.println("${event.name}: ${event.payload["id"]}");
    }
}

let app = gebweb.app();
gebweb.useEvents(app);
gebweb.registerEventSubscribers(app, [UserAudit()]);
gebweb.publish(app, "user.created", {"id": "u-1"});
gebweb.publish(app, "user.deleted", {"id": "u-1"});
```

```text
user.created: u-1
user.deleted: u-1
```

| Event member | Meaning |
|--------------|---------|
| `name: string` | Exact published name. |
| `payload: dict<string, any>` | Published data; the same event payload is visible to later listeners. |
| `app: any` | Owning application; cast to `gebweb.GebwebApp` when using typed helpers. |
| `stopPropagation(): void` | Stop later listeners in this dispatch. |

Domain payloads need not be JSON, but keep them JSON-serializable when handing
work to jobs or webhooks. Lifecycle payloads contain framework objects. Avoid
storing request-owned objects on a shared subscriber; use local variables,
request-scoped services or a concurrency-safe store for shared writes.

## Callable listeners and ordering

`gebweb.on(app, name, listener, priority = 0)` creates the bus if necessary and
returns an integer listener ID. `gebweb.off(app, id)` removes that registration
and returns whether it existed.

Higher priorities run first; equal priorities retain registration order.
Decorated subscribers use priority zero. This example removes one listener and
shows stable order among the rest:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app();
gebweb.on(app, "sample", func(gebweb.Event event): void {
    io.println("first");
}, 10);
int removed = gebweb.on(app, "sample", func(gebweb.Event event): void {
    io.println("removed");
});
gebweb.on(app, "sample", func(gebweb.Event event): void {
    io.println("second");
}, 10);
gebweb.on(app, "sample", func(gebweb.Event event): void {
    io.println("last");
}, -1);
io.println(gebweb.off(app, removed));
io.println(gebweb.off(app, removed));
gebweb.useEvents(app);
gebweb.publish(app, "sample", {});
```

```text
true
false
first
second
last
```

Each publish snapshots its listener list. Setup-time registration changes do
not change the list already being dispatched. No wildcard matching, background
transport or delivery retry is implied by an event name.

### Stopping propagation

Stopping propagation affects this dispatch only. It does not undo completed
listeners or erase errors already collected:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app();
gebweb.on(app, "handled", func(gebweb.Event event): void {
    io.println("handled");
    event.stopPropagation();
}, 1);
gebweb.on(app, "handled", func(gebweb.Event event): void {
    io.println("not reached");
});
gebweb.publish(app, "handled", {});
gebweb.publish(app, "handled", {});
```

```text
handled
handled
```

## When a listener fails

For ordinary domain events, dispatch catches each listener's `Error`, continues
with later listeners and throws one aggregated `RuntimeError` afterward.
The message includes listener labels and their error messages. This includes
errors such as `IOError`, not only `RuntimeError` subclasses:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app();
gebweb.on(app, "user.created", func(gebweb.Event event): void {
    throw IOError("audit unavailable");
});
gebweb.on(app, "user.created", func(gebweb.Event event): void {
    io.println("peer ran");
});
gebweb.on(app, "user.created", func(gebweb.Event event): void {
    throw ValueError("invalid notification");
});
try {
    gebweb.publish(app, "user.created", {"id": "u-1"});
} catch (RuntimeError error) {
    io.println(error.message);
}
```

```text
peer ran
events: 2 subscriber(s) failed publishing 'user.created': listener: audit unavailable; listener: invalid notification
```

An `HttpException` is different: it stops dispatch immediately and propagates
unchanged. Later listeners do not run, even if an earlier ordinary error was
collected. `stopPropagation` also prevents later listeners from running. Do not
promise that every listener runs regardless of rejection or propagation control.

Publishing is not transactional. A successful listener's database write or
external call remains performed when a later listener fails. Retrying the whole
publish can repeat those effects. Catch failures at the application's intended
boundary and make repeated effects safe rather than blindly republishing.

## Framework lifecycle events

The framework publishes application, request and authentication hooks on the
same bus. Register them during application or plugin setup.

| Event | Payload | Timing |
|-------|---------|--------|
| `app.start` | `address` | Before binding the first listener; normally once per app. A listener failure prevents binding and allows a later startup attempt to emit it again. |
| `app.stopping` | Empty | After marking the app as draining, before stopping listeners; once per app. |
| `app.stopped` | Empty | After the shutdown drain attempt and shutdown hooks; once per app. |
| `request.start` | `request` | After admission, before routing and middleware. |
| `request.response` | `request`, `response` | After normal dispatch and response middleware, before returning the response. |
| `request.error` | `request`, `error` | When an unexpected exception escapes dispatch or a start/response listener. |
| `request.finish` | `request` | At dispatcher completion, before request finalizers. |
| `auth.authenticated` | `request`, `result` | Authentication resolved a user. |
| `auth.rejected` | `request`, `result` | Supplied credentials were rejected. |
| `auth.absent` | `request`, `result` | Authentication found no credentials. |

`request` is a `gebweb.Request`, `response` is an `http.Response`, and `result`
is a `gebweb.authentication.AuthResult`. Authentication events fire only when
something asks for authentication, once per request/configuration; a public
route does not implicitly authenticate. Composed authenticators populate
`result.authenticator` with their name; legacy registration APIs need not.

### Request order and response changes

This example shows the relative order of event hooks, middleware and cleanup
on a missing route. A response hook adds a header without changing its status:

<!-- doctest:run -->
```gb
import gebweb;
import http;
import io;

let app = gebweb.app();
gebweb.on(app, "request.start", func(gebweb.Event event): void {
    let request = event.payload["request"] as gebweb.Request;
    io.println("start " + request.path());
});
gebweb.before(app, func(gebweb.Request request): ?http.Response {
    io.println("before");
    return null;
});
gebweb.after(app, func(gebweb.Request request, http.Response response): http.Response {
    io.println("after");
    return response;
});
gebweb.on(app, "request.response", func(gebweb.Event event): void {
    let response = event.payload["response"] as http.Response;
    io.println("response " + (response.status() as string));
    event.payload["response"] = response.withHeader("X-Observed", "yes");
});
gebweb.on(app, "request.finish", func(gebweb.Event event): void {
    io.println("finish");
});
gebweb.onRequestEnd(app, func(dict<string, any> request): void {
    io.println("cleanup");
});
let response = gebweb.TestClient(app).get("/absent");
io.println(response.status);
io.println(response.headers["X-Observed"]);
```

```text
start /absent
before
after
response 404
finish
cleanup
404
yes
```

A response listener replaces the outgoing response by assigning
`event.payload["response"]`. Use `withHeader`, `withBody` or another Response
operation deliberately. Keeping the original status preserves WebSocket/stream
markers; changing it replaces the upgrade or stream. See
[WebSockets and SSE](11-websockets-and-sse.md).

Dispatcher hooks also apply to missing routes, body-limit responses and static
assets. Admission rejection bypasses start/response hooks but still produces a
finish notification. A rendered 401/404/500 response is a response, not itself
an escaping exception: it does not automatically emit `request.error`.

### Rejection and observer failures

A `request.start` or `request.response` listener may reject by throwing an
`HttpException`. The dispatcher renders that status, still runs finish and
cleanup, and does not emit an error notification for the rejection:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app();
gebweb.on(app, "request.start", func(gebweb.Event event): void {
    throw gebweb.forbidden("maintenance");
});
gebweb.on(app, "request.error", func(gebweb.Event event): void {
    io.println("not an unexpected error");
});
gebweb.on(app, "request.finish", func(gebweb.Event event): void {
    io.println("finish");
});
gebweb.onRequestEnd(app, func(dict<string, any> request): void {
    io.println("cleanup");
});
io.println(gebweb.TestClient(app).get("/private").status);
```

```text
finish
cleanup
403
```

| Listener group | Failure policy |
|----------------|----------------|
| Domain events | Aggregate ordinary errors; `HttpException` aborts immediately. |
| `app.start` | Propagate failure and prevent listener binding. |
| `request.start`, `request.response` | Ordinary errors aggregate and enter normal dispatcher error handling; `HttpException` rejects immediately. |
| `auth.*` | Observer failures, including `HttpException`, are logged and do not change authentication. |
| `request.error`, `request.finish` | Failures are logged and cannot suppress the original error or skip request finalizers. |
| `app.stopping`, `app.stopped` | Failures are logged; shutdown continues. |

Logged observer groups still use the normal bus dispatch rules internally: an
`HttpException` can stop later observers in that group, but its outer observer
boundary logs it instead of changing the operation's outcome. Ordinary failures
allow peers to run before the aggregate is logged.

### Registration lifetime and cleanup

After the first successful `listen`/`serve`, the listener registry is frozen:
`on`, `off` and `registerEventSubscribers` reject changes with `RuntimeError`.
A later `useEvents` call does not reopen it. `TestClient` does not bind a server;
register listeners before creating it anyway so tests follow production setup.

Subscribers and callable captures are shared across requests. Keep mutable
request state in the payload or the request's DI scope. Lifecycle hooks and
request finalizers share that scope; it closes after completion.

Use `gebweb.onRequestEnd` for unconditional request cleanup; finalizers run in
reverse registration order. Use `gebweb.onShutdown` for application resources,
rather than relying on a stopping/stopped event to close them. Shutdown hooks
have explicit budgets; `app.stopped` does not promise that drain succeeded.
See [Deployment](37-deployment.md#shutdown-hooks).

## Events, middleware and durable jobs

| Requirement | Surface |
|-------------|---------|
| Synchronous notification to application extensions | `publish`, `on`, `@On` subscribers. |
| Early HTTP response or permission gate | Request middleware via `before`, or a rejecting start hook. |
| HTTP response transformation | `after` / `use`, or `request.response`. |
| Request-owned resource cleanup | `onRequestEnd`. |
| Long-lived resource shutdown | `onShutdown`. |
| Durable execution, retry and process-independent work | Background jobs. |

An event listener can enqueue durable work once jobs are configured:

<!-- doctest:fragment context=app -->
```gb
gebweb.useJobs(app, conn);
gebweb.on(app, "order.placed", func(gebweb.Event event): void {
    let application = event.app as gebweb.GebwebApp;
    gebweb.enqueue(application, "send-receipt", {"orderId": event.payload["orderId"]});
});
```

Register the `send-receipt` handler before running its worker. Enqueueing durably
records the job when that call succeeds, but does not make a separate order
write and event publish atomic. Use an outbox or a deliberately shared
transaction when those operations must commit together. Do not launch
unobserved background work from a listener and assume it has durable delivery.
See [Background jobs](18-background-jobs.md).

## Reference

| API | Contract |
|-----|----------|
| `gebweb.useEvents(app)` | Configure the bus if absent; preserve existing listeners. |
| `gebweb.registerEventSubscribers(app, instances)` | Register decorated methods on supplied instances; return the app. |
| `gebweb.publish(app, name, payload)` | Synchronously dispatch an exact event name; return `void`. |
| `gebweb.on(app, name, listener, priority = 0)` | Register a callable; return its integer ID. |
| `gebweb.off(app, id)` | Remove one listener registration; return whether it existed. |
| `gebweb.Event` | Context with name, payload, app and propagation control. |
| `gebweb.events.subscriberCount(config, name)` | Count registrations on an `EventConfig`, such as `app.eventConfig` after casting. |

The bus has no options dictionary; priority is the optional fourth argument to
`on`. Decorated subscribers have priority zero, and event names are exact
strings rather than wildcard patterns.

Related chapters: [Dependency injection](06a-dependency-injection.md),
[Middleware](09-middleware.md), [Plugins](16-plugins.md),
[Background jobs](18-background-jobs.md), [Authentication](40-authentication.md),
and [Deployment](37-deployment.md).

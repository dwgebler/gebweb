# Events

Gebweb's event bus is in-process pub/sub: publishers fire named
events with a payload dict; subscribers register methods with
`@On("event-name")` and receive an `Event` context.

Dispatch is synchronous - `gebweb.publish` returns once every
subscriber has finished. Subscriber exceptions are collected and
re-thrown as a single aggregated error after the dispatch, so one
buggy listener can't silently break the chain.

Distributed transports (Redis, NATS, ...) will land as sibling
packages that reuse the same subscriber registry through a
`Transport` interface. v1 is single-process only.

## Wiring

```gb
import gebweb;

let app = gebweb.app([HomeController()]);
gebweb.useEvents(app);
gebweb.registerEventSubscribers(app, [WelcomeMailer(), AuditLog()]);
```

## Publishing

Fire from any code holding the `app` reference:

```gb
class SignupController {
    @Post("/signup")
    func signup(SignupForm form): dict<string, any> {
        let user = userRepo.create(form);
        gebweb.publish(app, "user.created", {"id": user.id, "email": user.email});
        return {"id": user.id};
    }
}
```

The payload is a dict. Keep domain-event data JSON-serialisable when forwarding
it to a job queue or webhook. Lifecycle payloads include framework objects and
are not intended for serialization. Subscribers receive an `Event` context:

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | The event name passed to `publish`. |
| `payload` | `dict<string, any>` | The published payload. |
| `app` | `any` | The GebwebApp; subscribers use this to resolve DI or enqueue follow-on jobs. |

## Subscribing

```gb
class WelcomeMailer {
    @On("user.created")
    func send(gebweb.Event ev): void {
        let email = ev.payload["email"] as string;
        mailer.welcome(email);
    }
}

class AuditLog {
    @On("user.created")
    @On("user.deleted")
    func record(gebweb.Event ev): void {
        auditService.write(ev.name, ev.payload);
    }
}
```

A single method may stack multiple `@On` decorators to subscribe
to several events. Subscriber order within an event is the
registration order.

## When a subscriber fails

Publishing is synchronous: `gebweb.publish` runs every subscriber
in turn and only returns after the last one finishes. If a
subscriber throws, the framework remembers the failure and keeps
running the rest. After they've all had a turn, a single
`RuntimeError` is thrown back to the publisher with every
subscriber's error joined into one message.

That means the publisher only needs one `catch`:

```gb
try {
    gebweb.publish(app, "user.created", {"id": "u-1"});
} catch (RuntimeError e) {
    log.error("event delivery had failures", {"reason": e.message});
}
```

Successful subscribers always run, so a buggy listener can't
silently break the chain. The publisher sees the combined
failure and can decide whether to retry, log, or fail the
request.

## At-least-once delivery

Events are best-effort, in-process, and synchronous. For
guaranteed delivery (retry on failure, survives process death),
enqueue a job from the subscriber:

```gb
class OrderListener {
    @On("order.placed")
    func sendReceipt(gebweb.Event ev): void {
        /* Hand off to the job queue, which retries with backoff. */
        gebweb.enqueue(ev.app, "send-receipt", ev.payload);
    }
}
```

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.useEvents(app)` | Attach the in-process event bus. |
| `gebweb.registerEventSubscribers(app, instances)` | Discover `@On` subscribers on each instance. |
| `gebweb.publish(app, name, payload)` | Synchronously fire `name` to every subscriber. |
| `gebweb.Event` | Subscriber context: `name`, `payload`, `app`. |
| `gebweb.events.subscriberCount(config, name)` | Subscriber count for `name` (mostly tests). |

## Callable listeners

Register a callable without defining a subscriber class:

```gb
let id = gebweb.on(app, "user.created", func(gebweb.Event event): void {
    auditCreated(event.payload["id"] as string);
}, 10);
```

`on` creates the bus if necessary and returns a listener id. `off(app, id)`
removes that registration and returns whether it existed. Repeated `useEvents`
calls preserve registrations. Configure listeners during app/plugin setup;
registration and removal are not concurrent runtime configuration APIs.

Higher priorities run first; equal priorities preserve registration order.
Existing `@On` subscribers have priority zero. `event.stopPropagation()` stops
later listeners for that dispatch. Each dispatch takes a snapshot of its listener
list. Events remain synchronous; do not start unobserved work or assume they
provide transactionality or durable delivery.

## Framework lifecycle events

Applications and plugins use the same bus for application and request hooks:

| Event | Payload | Timing |
|-------|---------|--------|
| `app.start` | `address` | Before binding each listener; failure prevents binding. |
| `app.stopping` | empty | After marking the app as draining, before stopping listeners. |
| `app.stopped` | empty | After the shutdown drain attempt. |
| `request.start` | `request` | After admission, before dispatch and middleware. |
| `request.response` | `request`, `response` | After normal dispatch and response middleware, before returning the response. |
| `request.error` | `request`, `error` | When an exception escapes dispatch or a start/response listener. |
| `request.finish` | `request` | At dispatcher completion, before request finalizers. |
| `auth.authenticated` | `request`, `result` | After resolving a user. |
| `auth.rejected` | `request`, `result` | After rejecting supplied credentials. |
| `auth.absent` | `request`, `result` | After finding no credentials. |

`request` is a `gebweb.Request`; `response` is an `http.Response`; `result` is a
`gebweb.authentication.AuthResult`. Authentication events occur only when
something requests authentication, once per request/configuration, including
with existing registration APIs. Public routes are not authenticated implicitly.
Only composed authenticators populate `result.authenticator` with their name.

A response listener can replace `event.payload["response"]` with a new Response.
It runs on missing routes, body-limit responses and static assets too. Admission
rejections bypass start/response hooks but still produce finish notifications.
Middleware may have been explicitly disabled for assets; dispatcher events still
apply. A rendered 401/404/500 response reaches the response event; it does not
also raise an error event unless an exception actually escapes.

Start, response and authentication listener failures propagate through normal
error handling after other listeners have run, unless propagation was stopped.
Error, finish and shutdown listener failures are logged and cannot suppress the
original error or skip request finalizers. Do not use finish for work that must
change the response. `onRequestEnd` remains the cleanup API and runs finalizers
in reverse registration order.

## Events versus middleware and jobs

Use `before(app, callback)` to return an early response or continue with `null`.
Use `after(app, callback)` (also named `use`) to transform a response. Before
middleware runs in registration order; response middleware runs in reverse
order. Request start events precede those chains; response events follow them.
Register the same APIs from a plugin's `install` method to extend an app.

Use events for synchronous notifications and shared extension points. Event
listeners are shared services: keep request-specific values in the payload,
not mutable listener fields. Lifecycle hooks and request finalizers share the
handler's per-request DI scope; it closes after completion. Use a durable job when work must survive process
failure. Enqueuing from an event listener is not an atomic transaction with a
separate business database write; use an outbox or a shared transaction when
that guarantee is required.

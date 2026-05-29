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

The payload is any JSON-serialisable dict. Subscribers receive an
`Event` context:

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

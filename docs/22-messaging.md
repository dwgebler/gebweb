# Message Brokers

Gebweb integrates with external message brokers (RabbitMQ AMQP,
ActiveMQ STOMP, AWS SQS, Kafka) via the language's `messaging`
module. Application code registers one or more named broker
handles on the app and tags handler methods with
`@OnMessage("handle-name")`. A worker process drives the
consume / subscribe loop and dispatches each incoming message to
the registered handlers.

The broker handle types come from `stdlib/messaging` (see the
language reference under "Standard library > Messaging" for
backend-specific configuration). Gebweb only routes between
handle names and handler methods; it does not own the broker
connection.

## Wiring

```gb
import gebweb;
import messaging;

let orders = messaging.connect({
    "driver": "rabbitmq",
    "url":    "amqp://guest:guest@localhost:5672/",
    "queue":  "orders"
});

let app = gebweb.app([HomeController()]);
gebweb.useMessageQueue(app, "orders", orders);
gebweb.registerMessageHandlers(app, [OrderProcessor()]);
```

`useMessageQueue(app, name, handle)` registers a queue handle (a
`MessageQueue` returned by `messaging.connect`).
`useMessageTopic(app, name, handle)` does the same for a topic
handle from `messaging.topic`. The `name` is the string passed to
`@OnMessage`; multiple handles can be registered under different
names and each handler routes to the matching one.

### Topics (pub/sub)

Use a topic when one published message should be delivered to
several independent subscribers (e.g. a `user.created` event
consumed by both the email service and the analytics service).
Use a queue when exactly one consumer should receive each message
(work distribution).

```gb
import messaging;

let userEvents = messaging.topic({
    "driver": "sns",
    "region": "us-east-1",
    "topic":  "user-events",
});

gebweb.useMessageTopic(app, "user-events", userEvents);
```

Handlers register against the topic name the same way:

```gb
class UserEventListener {
    @OnMessage("user-events")
    func onEvent(any msg): void { /* ... */ }
}
```

## Writing handlers

A handler is a method on any class. Decorate it with
`@OnMessage("handle-name")` and it joins the dispatch list for
that handle. The method receives the message dict produced by
the broker:

```gb
class OrderProcessor {
    @OnMessage("orders")
    func handle(any msg): void {
        let m = msg as dict<string, any>;
        let order = json.parse(m["body"] as string);
        /* process; throw to surface the failure to the worker
         * log. Queue backends will redeliver per their
         * visibility-timeout / requeue rules. */
    }
}
```

Multiple methods can subscribe to the same handle - stack
decorators or split across classes. They run in registration
order; if any handler throws, the loop collects the errors and
re-throws one aggregated `RuntimeError` after the full dispatch.

## Running the worker

`gebweb.runMessageWorker(app)` blocks the calling process,
walking each registered handle and running `consume()` (queues)
or `subscribe()` (topics) in sequence. Most deployments have a
a subset of handles per process; pass `opts.handle` to select one:

```gb
gebweb.runMessageWorker(app, {"handle": "orders"});
```

For a process that owns more than one (but not all) registered
handles, pass `opts.handles` as a list:

```gb
gebweb.runMessageWorker(app, {"handles": ["orders", "audit"]});
```

The `gebweb worker --handle <name>` CLI flag (repeatable)
populates `GEBWEB_WORKER_HANDLES`, which the facade reads
automatically; main.gb doesn't need to plumb anything through.
Different worker processes can specialise on different broker
handles by composing `--handle` flags.

Selected handles run concurrently under one worker supervisor. Call
`runMessageWorker` once with the desired `handles` list; each individual
handle still dispatches and acknowledges serially.

## CLI entry point

A typical `main.gb` branches on an env var so one binary serves
HTTP, a job worker, or a messaging worker:

```gb
if (sys.getenv("GEBWEB_RUN") == "messaging") {
    gebweb.runMessageWorker(app);
} else if (sys.getenv("GEBWEB_RUN") == "worker") {
    gebweb.runWorker(app);
} else {
    gebweb.serve(app, "0.0.0.0:3000");
}
```

## Differences vs `@Job` and `@On`

| Decorator | Source of messages | Dispatch |
|-----------|--------------------|----------|
| `@Job("name")` | Versioned SQL job queue populated by `gebweb.enqueue`. | Bounded workers claim jobs and call their registered handler. |
| `@On("event-name")` | In-process `gebweb.publish`. | Synchronous; every subscriber runs in the publisher's thread. |
| `@OnMessage("handle-name")` | External broker (RabbitMQ / STOMP / SQS / Kafka). | Worker drives the broker's consume / subscribe loop and routes each message to all registered handlers. |

Use `@Job` for asynchronous work owned by this app. Use
`@OnMessage` when work originates from (or is shared with)
other services on a broker.

## Managed worker lifecycle

`gebweb.runMessageWorker(app, opts)` supervises one owned consumer task for each
selected named handle. Different handles progress concurrently; delivery and
acknowledgement within each handle remain serial. Acknowledgement follows all
successful handlers. An exception leaves work unacknowledged for backend
redelivery. A worker observes child failures and stops siblings before returning.
Handle and handler registration closes at worker startup.

Built-in handles support optional `messaging.ManagedMessageQueue.consumeUntil`
and `ManagedMessageTopic.subscribeUntil` contracts. Their stop predicate ends
admission, waits for dispatch/acknowledgement, interrupts idle reads where needed,
and closes transports. Existing `consume`, `subscribe` and custom handles remain
compatible. `workerCapabilities(config)` reports managed support; custom handles
without it cannot promise bounded cooperative drain.

For a module bootstrap returning `gebweb.workerCli(app, args)`, run:

```sh
gebweb worker --entry app.worker --messaging-only
gebweb worker --entry app.worker --messaging-only --handle orders --handle audit
```

The managed CLI requires capable handles. Library callers may set `signals: true`
when they own process signals; the default leaves host signal ownership alone.
`shutdownTimeoutMs` defaults to 30000. Handlers must return for drain to finish.
Optional `factories` map handle names to reconnect factories, with `maxRestarts`
(default 5) and `restartDelayMs` (1000); failures without a factory are surfaced.
The restart count resets once a consumer has run for `restartResetMs` (60000).
SQS receive requests have bounded HTTP deadlines. Visibility renewal and parallel
processing within one handle are deferred; size visibility timeout for handlers.

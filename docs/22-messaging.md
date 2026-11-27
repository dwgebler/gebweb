# Message brokers

<!-- doctest:strict -->

Messaging connects named broker handles to `@OnMessage` methods. Use it when
work arrives through RabbitMQ, STOMP, SQS, Kafka or another service's broker.
The framework dispatches each delivery to its registered handlers and
supervises the selected consumer tasks; the backend owns the broker protocol.

## A runnable consumer without a broker

The finite queue below is a test fixture, not a production broker. It holds a
fixed batch, retains unacknowledged messages in a `store.Store`, and implements
both the queue and cooperative-consumption contracts. It returns when its batch
is empty, so examples need no sleeps or background service.

Put it in `src/batch_queue.gb`:

<!-- doctest:file batch_queue.gb -->
```gb
import messaging;
import store;

export class BatchQueue implements messaging.MessageQueue, messaging.ManagedMessageQueue {
    list<dict<string, any>> ready;
    store.Store pending;
    store.Store state;

    func BatchQueue(list<dict<string, any>> messages) {
        this.ready = messages.copy();
        this.pending = store.Store();
        this.state = store.Store();
        for (message in messages) {
            this.pending.set(message["handle"] as string, true);
        }
    }

    func publish(any payload): void {
        throw RuntimeError("BatchQueue accepts a fixed test batch");
    }

    func receive(int timeoutMs): ?dict<string, any> {
        if (this.ready.isEmpty()) {
            return null;
        }
        let message = this.ready[0];
        this.ready = this.ready.slice(1);
        return message;
    }

    func ack(any receipt): void {
        this.pending.delete(receipt as string);
    }

    func consume(callable handler): void {
        this.consumeUntil(handler, func(): bool { return false; });
    }

    func consumeUntil(callable handler, callable shouldStop): void {
        while (!(shouldStop() as bool)) {
            let message = this.receive(0);
            if (message == null) {
                return;
            }
            handler(message);
            this.ack(message["handle"]);
        }
    }

    func close(): void {
        this.state.set("closed", true);
    }
}
```

This handler normalizes a string or binary body, validates its JSON input and
passes a concrete string into application work. Put it in
`src/order_handlers.gb`:

<!-- doctest:file order_handlers.gb -->
```gb
import bytes;
import io;
import json;

@Tag("message.handler")
export class OrderProcessor {
    @OnMessage("orders")
    func handle(dict<string, any> message): void {
        any raw = message["body"];
        string body = raw instanceof bytes ? bytes.toString(raw as bytes) : raw as string;
        let data = json.parse(body) as dict<string, any>;
        any id = data["orderId"];
        if (!(id instanceof string) || (id as string).isEmpty()) {
            throw ValueError("orderId must be a non-empty string");
        }
        this.process(id as string);
    }

    func process(string orderId): void {
        io.println("processed " + orderId);
    }
}
```

Register the handle before discovery. The tag lets discovery construct the
handler through DI; constructor dependencies can be registered in the usual way:

<!-- doctest:run -->
```gb
/** @allowUnusedImports */
import batch_queue;
import gebweb;
import gebweb.messaging as messages;
import io;
import order_handlers;

let queue = batch_queue.BatchQueue([
    {"id": "1", "handle": "receipt-1", "body": '{"orderId":"o-1"}'},
    {"id": "2", "handle": "receipt-2", "body": '{"orderId":"o-2"}'},
]);
let app = gebweb.app();
gebweb.useMessageQueue(app, "orders", queue);
gebweb.discover(app);
let config = app.messagingConfig as messages.MessagingConfig;
io.println(messages.workerCapabilities(config)["orders"]);
gebweb.runMessageWorker(app, {"requireManaged": true});
io.println(queue.pending.len());
io.println(queue.state.get("closed"));
```

```text
true
processed o-1
processed o-2
0
true
```

The worker acknowledges only after the handler returns successfully, then
closes the selected handle when its consumer finishes. The fixture's pending
set is empty at that point. A real consumer normally waits for new messages
until it is stopped, rather than returning on an empty poll.

## Handles, queues and topics

| Registration | Handle | Dispatch |
|--------------|--------|----------|
| `gebweb.useMessageQueue(app, name, handle)` | Queue from `messaging.connect(opts)` or a compatible custom queue | Drive its consume loop. |
| `gebweb.useMessageTopic(app, name, handle)` | Topic from `messaging.topic(opts)` or a compatible custom topic | Drive its subscribe loop. |
| `gebweb.registerMessageHandlers(app, instances)` | Already-constructed handler instances | Register decorated methods without DI construction. |

The registration name is an application label matched exactly by `@OnMessage`;
it need not equal a broker queue, topic or consumer-group name. Registering a
handle configures messaging automatically. Before worker startup, registering
another handle under the same label replaces that record.

A queue distributes work among its consumers, but delivery can repeat after
failure. A topic supports separate subscriber streams through its backend's
fan-out mechanism. Those streams still have backend-specific retention,
subscription and acknowledgement rules; neither API promises exactly-once
business effects.

| Factory driver | Queue support | Topic support / requirements |
|----------------|---------------|------------------------------|
| `rabbitmq` | Named queue | Fanout exchange; topic option names the exchange. |
| `stomp`, `activemq` | Destination | Topic destination. |
| `kafka` | Shared consumer group | Separate subscriber groups. |
| `sqs` | SQS queue | Not supported by `messaging.topic`; use SNS fan-out with SQS subscriptions. |
| `sns` | Not a queue driver | SNS publishing; subscription consumption requires a paired `queueUrl`. |

Broker connection options belong to the language's `messaging` module. The
framework does not invent a second connection format. For example, RabbitMQ
queue construction requires `url` and `queue`:

<!-- doctest:fragment context=app -->
```gb
import messaging;

let queue = messaging.connect({
    "driver": "rabbitmq",
    "url": "amqp://localhost:5672/",
    "queue": "orders",
    "durable": true,
});
gebweb.useMessageQueue(app, "orders", queue);
```

That fragment requires a running RabbitMQ service when executed. Provision
credentials, broker permissions and the intended queue/exchange topology for
your environment. Do not put real connection secrets in source examples.

An SNS `topic` name alone is not sufficient: the factory requires
`topicArn`, `region`, `accessKey` and `secretKey`. A topic handle without a
`queueUrl` can publish but cannot subscribe. Provision the SNS-to-SQS
subscription separately; creating a handle does not create that subscription.

## Handler dispatch and acknowledgement

A handler receives the backend's message dictionary. Common fields are:

| Field | Meaning |
|-------|---------|
| `body` | Payload, which may be a string or bytes depending on the backend. |
| `id` | Broker-specific message identifier; not a universal business idempotency key. |
| `handle` | Receipt/acknowledgement token interpreted by the backend. |

Normalize and validate data before passing it into typed application logic.
`@OnMessage` does not apply HTTP DTO binding or `@Assert` validation to an
arbitrary broker payload. Keep receipt tokens separate from business identifiers.

A method may stack `@OnMessage` decorators for several application handle names.
Inherited decorated methods participate in registration. For one delivered
message, all registered handlers run serially in registration order. If one
throws an `Error`, peers still run; afterward the framework throws one aggregate
`RuntimeError`. Unlike the in-process event bus, an `HttpException` is not a
special early-rejection mechanism here.

This example shows a failed delivery remaining unacknowledged even though a
peer handler ran:

<!-- doctest:run -->
```gb
import batch_queue;
import gebweb;
import io;

class FailingProcessor {
    @OnMessage("orders")
    func handle(any message): void {
        throw IOError("processing unavailable");
    }
}

class AuditProcessor {
    @OnMessage("orders")
    func record(any message): void {
        io.println("peer ran");
    }
}

let queue = batch_queue.BatchQueue([{"id": "1", "handle": "receipt-1", "body": "order"}]);
let app = gebweb.app();
gebweb.useMessageQueue(app, "orders", queue);
gebweb.registerMessageHandlers(app, [FailingProcessor(), AuditProcessor()]);
try {
    gebweb.runMessageWorker(app, {"requireManaged": true});
} catch (RuntimeError error) {
    io.println(error.message.contains("1 handler(s) failed"));
}
io.println(queue.pending.len());
io.println(queue.state.get("closed"));
```

```text
peer ran
true
1
true
```

A successful dispatch is followed by backend acknowledgement. A failed dispatch
is not acknowledged by the managed queue loop. Redelivery depends on the broker,
visibility timeout, connection closure and subscription configuration; the
framework does not add a universal nack policy or dead-letter queue. Make each
handler's effects idempotent because a peer failure can repeat work that already
succeeded.

## Running a module worker

Use a module exporting `main(list<string> args): int`, configure its handles and
handlers, and return `gebweb.workerCli(app, args)`. This RabbitMQ entry point is
strict-checked without contacting the broker; save it as `src/broker_worker.gb`:

<!-- doctest:file broker_worker.gb -->
```gb
/** @allowUnusedImports */
module broker_worker;
import gebweb;
import log;
import messaging;
import order_handlers;
import sys;

export func main(list<string> args): int {
    let app = gebweb.app();
    gebweb.useLogger(app, log.stderr());
    let queue = messaging.connect({
        "driver": "rabbitmq",
        "url": sys.getenv("AMQP_URL") ?? "amqp://localhost:5672/",
        "queue": "orders",
    });
    gebweb.onShutdown(app, "orders", func(): void {
        queue.close();
    });
    try {
        gebweb.useMessageQueue(app, "orders", queue);
        gebweb.discover(app);
        return gebweb.workerCli(app, args);
    } finally {
        gebweb.shutdown(app);
    }
}
```

Run it with the messaging mode explicitly selected:

```sh
gebweb worker --entry broker_worker --messaging-only
gebweb worker --entry broker_worker --messaging-only --handle orders
```

The module CLI accepts `--messaging-only` and repeated `--handle NAME`
selections. It requires managed handles, installs signal handling and returns
`0` on normal completion, `2` on invalid selection arguments or `1` when the
consumer supervisor fails. SQL-job flags such as `--once`, `--max-jobs` and
`--concurrency` are not messaging-mode options.

A module worker does not need a SQL job queue just to consume broker messages.
Keep HTTP argument parsing separate: an HTTP entry point uses `cli.checkArgs`,
builds the app and calls `gebweb.cli`. It does not switch to a messaging worker
merely because `GEBWEB_RUN` is present. The legacy file bootstrap's
`--messaging-only` flag sets `GEBWEB_RUN=worker` and
`GEBWEB_WORKER_KIND=messaging`; its entry point must explicitly select the
message loop. See [The CLI](15-cli.md#legacy-file-bootstrap).

## Supervision and worker options

`gebweb.runMessageWorker(app, opts = {})` starts one owned consumer task per
selected handle that has registered handlers. Different handles progress
concurrently; each individual handle dispatches and acknowledges serially.
Call it once with the intended selection rather than starting unobserved loops.

| Option | Default | Effect |
|--------|---------|--------|
| `handle` | None | Select one application handle name. |
| `handles` | `[]` | Select a list; empty means all. If both selectors are supplied, both filters apply. |
| `signals` | `false` | Own SIGINT/SIGTERM while running; clear those handlers afterward. |
| `requireManaged` | `false` | Reject selected consumer handles without cooperative stop support. The module CLI enables it. |
| `shutdownTimeoutMs` | `30000` | Positive drain deadline; exceeding it records a failure while the supervisor still joins owned consumers. |
| `drainCheck` | App's drain state | Callable returning `true` to stop admission. |
| `factories` | `{}` | Handle-name map of zero-argument factories returning replacement connections after failure. |
| `maxRestarts` | `5` | Reconnection limit per consumer; no factory means the failure propagates. |
| `restartDelayMs` | `1000` | Positive pause before reconnecting. |
| `restartResetMs` | `60000` | Positive healthy-runtime threshold after which the reconnect count resets on failure. |

With neither selector supplied, `GEBWEB_WORKER_HANDLES` provides a comma-separated
list when set. Explicit selectors override it. Unregistered selections and
handles with no registered handlers do not start tasks; an empty selection of
consumer tasks can return immediately. Validate your wiring using the inspection
helpers below rather than treating a clean empty run as proof of consumption.

Handle and handler registration closes at worker startup, including for handles
not selected by that run. Register all handlers first, either through discovery
with `@Tag("message.handler")` or explicit instances, without duplicating both.
Selected handles are worker-owned and closed on exit. Do not share a selected
connection with unrelated consumers or publishers that need to outlive it.

A reconnect factory must return a fresh compatible handle, not the already
closed instance. The supervisor closes the old connection before reconnecting,
logs the attempt and observes replacement failures. When a consumer fails
without recovery, it requests sibling shutdown, joins owned tasks, then reports
the combined failure.

### Cooperative drain

Managed queues implement `messaging.ManagedMessageQueue.consumeUntil(handler,
shouldStop)`; managed topics implement
`messaging.ManagedMessageTopic.subscribeUntil(handler, shouldStop)`. These are
additional interfaces alongside the normal queue/topic contract. Built-in
handles provide them, although a particular configuration may still lack a
usable subscriber, as with an SNS topic without a paired queue.

A stop request ends admission and waits for the current handler and its
acknowledgement. Idle reads are bounded or interrupted by the backend. The drain
deadline is not a thread kill: if a handler or custom consumer does not return,
the supervisor records a failure, warns at the deadline and continues waiting
for its owned tasks. After they return it throws, so the module worker exits
`1` even if those consumers eventually finished cleanly. Use a process
supervisor when a hard termination boundary is required.

Legacy `consume`/`subscribe` custom handles remain usable in library mode, but
cannot promise bounded cooperative shutdown. SQS visibility renewal and
parallel delivery within one handle are not provided here; choose visibility
and external timeouts for the work your handlers perform.

## Inspection and choosing an abstraction

Import `gebweb.messaging as messages` for configuration inspection:

| Helper | Result |
|--------|--------|
| `messages.handlerCount(config, name)` | Count registered methods for an application handle. |
| `messages.handle(config, name)` | Configured handle, or `null` when absent. |
| `messages.workerCapabilities(config)` | Map of handle names to managed-interface support. |

Use `app.messagingConfig as messages.MessagingConfig` for `config` after
registering a handle. Capability reporting checks interfaces, not live broker
health, permissions or the correctness of a subscription.

| Requirement | Surface |
|-------------|---------|
| Durable asynchronous work owned by this app's database | `@Job`, `gebweb.enqueue` and job workers. |
| Synchronous notification in the current process | `@On`, `gebweb.publish` and event listeners. |
| Work or event streams shared with other services through a broker | `@OnMessage` and named queue/topic handles. |

Neither an external publish nor a handler's side effect is automatically atomic
with a separate application database write. Use an outbox or an appropriate
transactional design where that guarantee matters. Broker retention, retry and
dead-letter settings remain operational responsibilities of the chosen backend.

## Reference

| API | Purpose |
|-----|---------|
| `gebweb.useMessageQueue(app, name, handle)` | Register a named queue. |
| `gebweb.useMessageTopic(app, name, handle)` | Register a named topic. |
| `gebweb.registerMessageHandlers(app, instances)` | Register decorated methods on supplied instances. |
| `gebweb.runMessageWorker(app, opts = {})` | Supervise selected consumers and join them on exit. |
| `gebweb.workerCli(app, args)` | Module entry surface; pass `--messaging-only` for brokers. |
| `@OnMessage("name")` | Bind a method to an exact application handle label. |
| `messages.workerCapabilities`, `handlerCount`, `handle` | Inspect configured handles and registrations. |

Related chapters: [Dependency injection](06a-dependency-injection.md),
[The CLI](15-cli.md), [Background jobs](18-background-jobs.md),
[Events](19-events.md), [Concurrency](35-concurrency.md), and
[Deployment](37-deployment.md).

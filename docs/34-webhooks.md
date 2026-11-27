# Outbound webhooks

<!-- doctest:strict -->

`useWebhooks` adds an in-memory subscription registry and a dispatcher backed
by the [job queue](18-background-jobs.md). Dispatch writes one job per matching
subscription. A worker later signs and POSTs each delivery; the HTTP request
that enqueues it still pays for the database writes.

## Wiring

Configure jobs before webhooks, in both the producer and worker bootstrap.
`useWebhooks` registers `gebweb.webhook.send` automatically. Starting jobs later
does not retroactively register this handler.

This complete example uses an in-memory queue and a local receiver. In a real
application, use a durable database shared by producer and worker processes,
then run the worker independently with the same handler configuration.

<!-- doctest:run -->
```gb
import db;
import gebweb;
import gebweb.jobs as jobs;
import http;
import io;

let receiver = http.listen(
    "127.0.0.1:0",
    func(dict<string, any> request): dict<string, any> { return {"status": 204, "body": ""}; }
);
let conn = db.connect("sqlite", ":memory:");
try {
    let app = gebweb.app([], {"docs": false});
    gebweb.useJobs(app, conn, {"maxAttempts": 5});
    gebweb.useWebhooks(app, {"maxAttempts": 5});
    gebweb.subscribeWebhook(
        app,
        "user.created",
        "http://" + http.serverAddr(receiver) + "/hook",
        "example-secret"
    );

    io.println(gebweb.dispatchWebhook(app, "user.created", {"id": "u1"}));
    gebweb.runWorker(app, {"drainOnce": true});
    io.println(jobs.stats(app.jobConfig)["completed"]);
} finally {
    conn.close();
    http.shutdown(receiver, 1000);
}
```

```text
1
1
```

For a continuously running worker, omit `drainOnce`. See the jobs chapter for
worker shutdown, queue migrations, concurrency and operational configuration.

## Subscriptions

`subscribeWebhook(app, event, url, secret)` appends a subscription; it does not
deduplicate registrations. Events match exactly. An empty secret selects
unsigned delivery. Use a nonempty, randomly generated secret for authenticated
receivers, and keep subscription management behind application authorization.

The registry is local to the app process. Persist subscriptions in your own
model and replay them at startup, or pass `seed: list<WebhookSubscription>` to
`useWebhooks`. There is no built-in persistent subscription store. Configure
subscriptions before serving concurrent requests, or provide your own
synchronization for changes.

`unsubscribeWebhook(app, event, url)` removes all exact event/URL matches and
returns whether anything was removed. Already queued jobs retain their URL,
secret and payload; removing a subscription does not cancel them.

## Dispatch

`dispatchWebhook(app, event, payload)` returns the number of enqueued deliveries.
It returns zero when webhooks are unconfigured or no subscription matches; with
webhooks configured but no job queue it throws. Fan-out consists of separate
enqueues: a later failure can leave earlier deliveries queued. Application
writes and webhook enqueues do not form an automatic transaction or outbox.

Each POST contains JSON with this shape:

```json
{"event":"user.created","data":{"id":"u1"},"at":1795680000}
```

`at` is generated during each delivery attempt. A retry can have a different
body and signature. Delivery is at least once; receivers need an application
identifier in `data` and durable deduplication before performing repeat-sensitive
effects. The envelope does not provide a delivery ID.

Subscription URLs are outbound HTTP destinations without framework destination
validation. Accept trusted endpoints and enforce your own URL and network egress
policy. Queue payloads include the subscription secret, so protect queue storage
and operational access to it.

## Signatures

With a nonempty secret, the sender adds `X-Gebweb-Signature: sha256=<hex>`, an
HMAC-SHA256 over the exact JSON body. Every delivery also adds
`X-Gebweb-Timestamp`, containing Unix seconds. Empty-secret deliveries omit the
signature header.

Verify the original body before parsing or changing it. This receiver example
uses a fixed example secret; load the real shared secret from protected
configuration. Put it in `src/webhook_receiver.gb`:

<!-- doctest:file webhook_receiver.gb -->
```gb
import gebweb;

@Controller("/hooks")
class WebhookReceiver {
    @Post("/inbound")
    func receive(gebweb.Request request): dict<string, any> {
        string signature = request.header("X-Gebweb-Signature") ?? "";
        if (!gebweb.verifyWebhookSignature(request.text(), "example-secret", signature)) {
            return gebweb.html("Invalid signature", 401);
        }
        return gebweb.html("Accepted", 202);
    }
}
```

This runnable check exercises the receiver with valid and invalid signatures:

<!-- doctest:run -->
```gb
/** @allowUnusedImports */
import gebweb;
import gebweb.webhooks as webhooks;
import io;
import log;
import webhook_receiver;

let app = gebweb.app();
gebweb.useLogger(app, log.stderr());
let client = gebweb.TestClient(app);
string body = '{"id":"u1"}';
string signature = webhooks.defaultSign(body, "example-secret");
io.println(client.request("POST", "/hooks/inbound", body, {
    "Content-Type": "application/json",
    "X-Gebweb-Signature": signature,
}).status);
io.println(client.request("POST", "/hooks/inbound", body, {
    "Content-Type": "application/json",
    "X-Gebweb-Signature": "invalid",
}).status);
```

```text
202
401
```

For a [CSRF-enabled application](23-csrf.md), exempt the signed external
callback with `@CsrfExempt` on this handler. Keep signature verification in
place; external senders do not have a browser CSRF token.

A runnable verification example:

<!-- doctest:run -->
```gb
import gebweb;
import gebweb.webhooks as webhooks;
import io;

string body = '{"event":"created","data":{"id":"u1"}}';
string signature = webhooks.defaultSign(body, "example-secret");
io.println(gebweb.verifyWebhookSignature(body, "example-secret", signature));
io.println(gebweb.verifyWebhookSignature(body + " ", "example-secret", signature));
```

```text
true
false
```

The helper uses a constant-time comparison. It verifies only the default
signature format, without freshness or replay protection. Empty secret and empty
header compare equal, so a receiver must require a configured nonempty secret.
The timestamp header is not part of the HMAC input. `timestampSkewSeconds` is
stored configuration but is not enforced by either delivery or verification.
Implement a receiver policy using authenticated event data and durable replay
tracking when required.

Custom `signatureHeader` and `timestampHeader` options change outgoing names.
A `signer(body, secret): string` replaces the default signer for nonempty-secret
subscriptions. Pair it with your own receiver verification; the default helper
does not invoke custom signers.

## Retries and dead-lettering

A final 2xx response succeeds. A non-2xx response or network error throws and
lets the job queue retry. The HTTP client follows redirects, so the final
response determines success. Queue configuration controls retry count and
backoff; by default there are five attempts, with retry delays drawn from
`[1000, 5000, 30000, 120000, 600000]` milliseconds. The fifth failed attempt
ends the job, so the last delay is not used by that default limit.

The webhook `maxAttempts` option is a separate threshold for calling
`deadLetter(event, payload, lastError): void`. It does not configure queue
retries. Set it to the job queue's attempt limit to run the callback on the
final failed attempt. A lower threshold calls it on multiple failures; a higher
threshold can prevent it from running. Keep the callback idempotent and persist
failure details if operators need to replay deliveries. Callback errors also
escape through the worker failure path.

## Reference

| Helper | Behavior |
|--------|----------|
| `gebweb.useWebhooks(app, opts = {})` | Store config and register the built-in handler if jobs are already configured. |
| `gebweb.subscribeWebhook(app, event, url, secret = "")` | Append and return a `WebhookSubscription`; throws without webhook setup. |
| `gebweb.unsubscribeWebhook(app, event, url)` | Remove all matches; false without setup. |
| `gebweb.dispatchWebhook(app, event, payload)` | Enqueue one delivery per match and return the count. |
| `gebweb.verifyWebhookSignature(body, secret, headerValue)` | Compare the default HMAC signature. |
| `webhooks.deliverNow(cfg, sub, event, payload)` | Return `http.Response` on success; throw on delivery failure. Import `gebweb.webhooks as webhooks`. |

| `useWebhooks` option | Default | Purpose |
|----------------------|---------|---------|
| `seed` | `[]` | Initial `list<WebhookSubscription>`. |
| `signatureHeader` | `X-Gebweb-Signature` | Outgoing signature header. |
| `timestampHeader` | `X-Gebweb-Timestamp` | Outgoing timestamp header. |
| `signer` | `null` | Custom signing callable. |
| `deadLetter` | `null` | Failure callback at or above the webhook threshold. |
| `maxAttempts` | `5` | Callback threshold; keep aligned with queue retries. |
| `timestampSkewSeconds` | `300` | Stored only; no freshness check. |

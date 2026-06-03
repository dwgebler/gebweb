# Outbound webhooks

`useWebhooks` adds a subscription registry and a job-queue-backed
dispatcher to the app. Subscribers ask for an event by URL;
publishers fire payloads via `dispatchWebhook`. Each delivery
runs as a separate background job so failures retry without
blocking the request thread.

## Wiring

```gb
import gebweb;

let app = gebweb.app([HomeController()]);
gebweb.useJobs(app, jobsConn);
gebweb.useWebhooks(app, {
    "deadLetter": func(string event, dict<string, any> payload, string lastError): void {
        deadLetterRepo.save(event, payload, lastError);
    }
});
```

`useWebhooks` registers its own `@Job("gebweb.webhook.send")`
handler internally; you do not need to mention it when you call
`useJobs(app, ..., [yourHandlers])`.

The job queue must already be wired - the dispatcher refuses to
run without it.

## Subscriptions

```gb
gebweb.subscribeWebhook(app, "user.created",
    "https://customer.example.com/hooks/user", "shared-secret-1");

gebweb.subscribeWebhook(app, "user.created",
    "https://otheraccount.example.com/webhooks/user", "shared-secret-2");
```

The 1.1 line stores subscriptions in memory on the app. Persist
them to a database in your own model and seed the registry at
startup by replaying them; a built-in persistent store is scoped
for 1.2.

`gebweb.unsubscribeWebhook(app, event, url)` removes a single
subscription and returns true when it was present.

## Dispatch

```gb
class SignupController {
    @Post("/signup")
    func signup(SignupForm form): dict<string, any> {
        let user = userRepo.create(form);
        gebweb.dispatchWebhook(app, "user.created", {
            "id":    user.id,
            "email": user.email,
            "at":    datetime.nowUnix()
        });
        return {"id": user.id};
    }
}
```

`dispatchWebhook` returns the number of subscribers that were
queued. Each delivery becomes its own job; the request returns
immediately.

The outgoing body is:

```json
{
  "event": "user.created",
  "data": {"id": "u-1", "email": "...", "at": 1748880000},
  "at":   1748880001
}
```

## Signatures

By default every outgoing request carries:

- `X-Gebweb-Signature: sha256=<hex>` - HMAC-SHA256 over the body
  with the subscription's shared secret.
- `X-Gebweb-Timestamp: <unix-seconds>` - the time the request
  was built.

A subscription with an empty secret omits the signature.

Override the header name with `opts.signatureHeader` (use
`X-Hub-Signature-256` to mimic GitHub) and replace the signer
entirely by passing `opts.signer`:

```gb
gebweb.useWebhooks(app, {
    "signatureHeader": "X-Hub-Signature-256",
    "signer": func(string body, string secret): string {
        return "sha256=" + crypt.hmacSha256(secret, body);
    }
});
```

The receiver-side helper:

```gb
class WebhookReceiver {
    @Post("/hooks/inbound")
    func receive(dict<string, any> request): dict<string, any> {
        let body = request["body"] as string;
        let sig = (request["headers"] as dict<string, any>)["X-Gebweb-Signature"] as string;
        if (!gebweb.verifyWebhookSignature(body, "shared-secret", sig)) {
            throw gebweb.unauthorised("bad signature");
        }
        ...
    }
}
```

`verifyWebhookSignature` is a constant-time compare under the
hood so timing oracles cannot leak the expected signature.

## Retries and dead-lettering

Deliveries inherit the job queue's retry policy. The default
schedule is `[1s, 5s, 30s, 2m, 10m]` for five attempts before the
job is marked failed. Each attempt that returns 2xx counts as
success; any other status (or network error) raises an
exception, triggering the next retry.

On the final attempt the configured `deadLetter` callback fires
(when supplied) with the event name, original payload, and the
last error message. Persist these to a database, file, or
operational queue so they can be replayed by hand.

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.useWebhooks(app, opts)` | Mount the dispatcher and register the built-in job handler. |
| `gebweb.subscribeWebhook(app, event, url, secret)` | Add a subscription. |
| `gebweb.unsubscribeWebhook(app, event, url)` | Remove a subscription. |
| `gebweb.dispatchWebhook(app, event, payload)` | Queue one job per matching subscription. |
| `gebweb.verifyWebhookSignature(body, secret, headerValue)` | Receiver-side HMAC check. |
| `gebweb.webhooks.deliverNow(cfg, sub, event, payload)` | Synchronous one-shot delivery for tests / debugging. |

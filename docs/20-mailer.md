# Mailer

<!-- doctest:strict -->

A `Mailable` describes a recipient, subject and template context. A transport
receives the rendered message and delivers it. `gebweb.send(app, mail)` delivers
synchronously unless a job queue is configured, in which case it enqueues the
rendered message for a worker.

Use the memory transport to test email content without an SMTP server or an
external provider. The same mailable can then use SMTP, SES or a custom
transport in the application.

## A mailable and its template

Put this template in `templates/emails/welcome.html`:

<!-- doctest:file templates/emails/welcome.html -->
```html
<p>Hi {{ name }}, welcome aboard!</p>
```

Define the message in `src/welcome_mail.gb`:

<!-- doctest:file welcome_mail.gb -->
```gb
import gebweb;

export class WelcomeMail extends gebweb.Mailable {
    string name;

    func WelcomeMail(string to, string name) {
        parent();
        this.to = to;
        this.name = name;
    }

    func subject(): string {
        return "Welcome, " + this.name;
    }

    func template(): string {
        return "emails/welcome.html";
    }

    func context(): dict<string, any> {
        return {"name": this.name};
    }
}
```

Then configure views and a transport before sending:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import welcome_mail;

let app = gebweb.app();
let transport = gebweb.memoryMailer();
gebweb.useViews(app, "templates");
gebweb.useMailer(app, transport);
let message = welcome_mail.WelcomeMail("ada@example.test", "Ada");
message.fromOverride = "Team <team@example.test>";
gebweb.send(app, message);

let sent = transport.sent[0];
io.println(transport.sent.length());
io.println(sent["to"]);
io.println(sent["subject"]);
io.println((sent["body"] as string).trim());
io.println(sent["from"]);
transport.clear();
io.println(transport.sent.length());
```

```text
1
ada@example.test
Welcome, Ada
<p>Hi Ada, welcome aboard!</p>
Team <team@example.test>
0
```

`parent()` initializes `to` and `fromOverride` to empty strings. Set `to` on
every message; an empty recipient is rejected before delivery. A non-empty
`fromOverride` takes precedence over the SMTP/SES transport's default sender.

| Mailable member | Base default | Purpose |
|-----------------|--------------|---------|
| `to: string` | `""` | One recipient passed to the transport. |
| `fromOverride: string` | `""` | Per-message sender; empty means use the transport default. |
| `subject(): string` | `""` | Subject line. |
| `template(): string` | `""` | View template name; empty produces an empty body. |
| `context(): dict<string, any>` | `{}` | Values passed to that template. |

Templates use the configured view engine and its HTML escaping. Do not build
HTML by interpolating untrusted names directly. `context()` runs when `send`
renders a non-empty template; it is not a deferred worker callback. See
[Views and templates](04a-views.md).

The framework contract carries a single recipient, HTML body and sender. It
has no mailable fields for attachments, CC/BCC, Reply-To or a plain-text
alternative. Those need a different application abstraction or a custom
transport with its own explicit contract.

## Synchronous and queued delivery

| App configuration | What `send` does |
|-------------------|------------------|
| No mailer | Raise an error. |
| Mailer, no job queue | Render and call the transport before returning. Transport errors propagate to the caller. |
| Mailer and job queue | Render and enqueue `_gebweb_mailer_deliver`, then return. A worker later calls its configured transport. |

With jobs, configure **jobs first, then mailer**. `useMailer` registers the
built-in delivery handler when it sees an existing queue. Configure it once,
before any worker is prepared or started; repeated registration can conflict
with the existing handler. Adding jobs after the mailer does not retroactively
register that delivery handler.

This example uses the same template and message, with an in-memory SQLite
queue. Nothing is delivered until the finite worker drain:

<!-- doctest:run -->
```gb
import db;
import gebweb;
import gebweb.jobs as jobsmod;
import io;
import welcome_mail;

let conn = db.connect("sqlite", ":memory:");
let app = gebweb.app();
let transport = gebweb.memoryMailer();
gebweb.useViews(app, "templates");
gebweb.useJobs(app, conn, {"pollIntervalMs": 1});
gebweb.useMailer(app, transport);
gebweb.send(app, welcome_mail.WelcomeMail("bea@example.test", "Bea"));
io.println(transport.sent.length());
io.println(jobsmod.stats(app.jobConfig as jobsmod.JobConfig)["pending"]);

gebweb.runWorker(app, {"drainOnce": true});
io.println(transport.sent.length());
io.println(transport.sent[0]["subject"]);
io.println(jobsmod.stats(app.jobConfig as jobsmod.JobConfig)["completed"]);
conn.close();
```

```text
0
1
1
Welcome, Bea
1
```

The queue stores `{to, subject, body, from}`. Changing the template or mailable
instance afterward does not change that queued content. Missing templates and
other render failures occur in the producer's `send` call; transport failures
occur in the worker and follow the job queue's retry/backoff policy.

A separate worker process must configure the same queue and mailer before
calling `gebweb.workerCli(app, args)`. The payload does not carry SMTP passwords
or a provider client: the worker uses its own configured transport. An embedded
worker is another option; see [Background jobs](18-background-jobs.md#in-process-worker).
A worker restricted by job name must select `_gebweb_mailer_deliver` to deliver
these messages.

In an application, configure views, jobs and mailer in `build(conn)` and close
the database through the entry point's shutdown hook. Keep provider credentials
in application configuration rather than a mailable or queued payload. See
[Getting started](01-getting-started.md), [Configuration](29-services-yaml.md)
and the module worker example in [Background jobs](18-background-jobs.md).

## Choosing a transport

| Factory | Behavior |
|---------|----------|
| `gebweb.memoryMailer()` | Capture messages locally in `.sent`; `.clear()` empties them. |
| `gebweb.logMailer()` | Print recipient, subject, HTML and any sender override to stdout. |
| `gebweb.smtpMailer(opts)` | Send through the stdlib SMTP transport. |
| `gebweb.sesMailer(opts)` | Send a signed SES v2 HTTP request. |
| Custom `gebweb.MailerContract` | Implement delivery for an application-specific backend. |

Memory and log transports do not deliver email externally. The log transport
prints complete message content, so use it with appropriate development data.
MemoryMailer serializes its writes and `clear` with a mutex; inspect `.sent`
after sends or a finite worker drain have completed, rather than reading it
while concurrent deliveries are changing it.

### SMTP

Constructing the transport does not contact the server. This runnable example
checks the configured sender and TLS mode without sending email:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let transport = gebweb.smtpMailer({
    "host": "smtp.example.test",
    "port": 587,
    "username": "noreply",
    "password": "example-only",
    "from": "Team <noreply@example.test>",
    "startTLS": true,
});
io.println(transport.defaultFrom);
io.println(transport.inner.transport.config["startTLS"]);
io.println(transport.inner.transport.config["tls"]);
```

```text
Team <noreply@example.test>
true
false
```

| SMTP option | Default | Effect |
|-------------|---------|--------|
| `host` | Required | SMTP hostname. |
| `port` | Required | SMTP TCP port. |
| `username` | Required | Authentication username; pass `""` when no authentication is configured. |
| `password` | Required | Authentication password; pass `""` alongside an empty username. |
| `from` | `""` | Default sender, used unless the message overrides it. |
| `startTLS` | `true` | Request a STARTTLS upgrade. |
| `tls` | `false` | Connect using implicit TLS; setting it to `true` disables STARTTLS. |

`gebweb.smtpMailer` forwards these listed options only. Select the TLS mode and
port required by your SMTP service; the port alone does not choose the mode.
Configure a sender default or a per-message override before delivering.

### AWS SES

SES uses an explicitly supplied access key and secret to sign a POST to
`/v2/email/outbound-emails`. The default host is regional. Construction is local,
so this example does not contact AWS:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let transport = gebweb.sesMailer({
    "region": "eu-west-2",
    "accessKey": "EXAMPLE_ACCESS_KEY",
    "secretKey": "example-only-secret",
    "from": "Team <noreply@example.test>",
});
io.println(transport.endpoint);
io.println(transport.defaultFrom);
```

```text
https://email.eu-west-2.amazonaws.com
Team <noreply@example.test>
```

| SES option | Default | Effect |
|------------|---------|--------|
| `region` | Required | Signing region and default endpoint region. |
| `accessKey` | Required | Signing access key. |
| `secretKey` | Required | Signing secret. |
| `from` | `""` | Default sender; delivery fails if neither this nor an override supplies one. |
| `endpoint` | `https://email.<region>.amazonaws.com` | Base endpoint; supply scheme and host without a trailing slash or API path. |

This factory does not discover AWS credentials or provide a session-token
option. Load supported credentials explicitly through the application's secret
configuration. Provision sender identities and service permissions separately;
the transport only performs the signed send request.

Each delivery sends UTF-8 subject and HTML data in the SES v2 JSON shape.
A non-2xx result throws `RuntimeError` containing the status and response body;
network failures propagate too. A successful API request means the provider
accepted it, not that the recipient has received it. Endpoint overrides are
useful for compatible local services, but must preserve the expected API path
and signing configuration.

## Custom transports

Implement `gebweb.MailerContract` with exactly this delivery signature:
`deliver(string to, string subject, string htmlBody, string fromOverride): void`.
The transport receives rendered fields and does not render templates itself.
An empty sender override means use its default sender, if it has one.

This local transport emits one JSON log record instead of contacting a mail
provider. It demonstrates the complete contract without credentials or network
access:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import json;
import welcome_mail;

class JsonLogMailer implements gebweb.MailerContract {
    func deliver(string to, string subject, string htmlBody, string fromOverride): void {
        io.println(
            json.stringify({
                "to": to,
                "subject": subject,
                "html": htmlBody.trim(),
                "from": fromOverride,
            })
        );
    }
}

let app = gebweb.app();
gebweb.useViews(app, "templates");
gebweb.useMailer(app, JsonLogMailer());
gebweb.send(app, welcome_mail.WelcomeMail("cy@example.test", "Cy"));
```

```text
{"from":"","html":"<p>Hi Cy, welcome aboard!</p>","subject":"Welcome, Cy","to":"cy@example.test"}
```

A remote transport must check its provider's response and throw when delivery
was not accepted; returning normally marks a queued attempt completed. Keep
transport state safe for concurrent calls because requests and job workers can
share the instance. Supply bounded network operations through the provider
client you choose.

## Delivery guarantees and testing

- Assert recipient, subject, rendered body and sender with the memory transport.
  Test missing/invalid application data before enqueueing. Empty `to` is rejected,
  but the mailable layer is not an application-specific address validator.
- Queued delivery is at least once. A provider may accept a message before the
  worker records completion, so retry can send a duplicate. Design notification
  workflows with that possibility in mind; `send` does not expose a per-message
  deduplication option or provider idempotency key.
- A queued send is not atomic with a separate application database write. Use
  an outbox or deliberately shared transaction when both must commit together.
- Rendering freezes the message's content at enqueue time. Do not queue secrets
  or request-owned objects as message data, and apply suitable retention to
  rendered content in queue tables and logs.
- Read queue status and failures when testing asynchronous delivery; a successful
  `send` means enqueue succeeded, not that the email was accepted by a provider.

## Reference

| API | Contract |
|-----|----------|
| `gebweb.Mailable` | Base message: recipient, sender override, subject, template and context. |
| `gebweb.MailerContract` | Transport interface for four rendered string fields. |
| `gebweb.useMailer(app, transport)` | Configure one transport; register the mail job when jobs are already configured. |
| `gebweb.send(app, mail)` | Render then deliver or enqueue; returns `void`. |
| `gebweb.memoryMailer()` | In-memory capture transport with `.sent` and `.clear()`. |
| `gebweb.logMailer()` | Development stdout transport. |
| `gebweb.smtpMailer(opts)` | SMTP transport; options listed above. |
| `gebweb.sesMailer(opts)` | SES v2 transport; options listed above. |

Related chapters: [Views and templates](04a-views.md),
[Testing](14-testing.md), [Background jobs](18-background-jobs.md),
[Configuration and secrets](29-services-yaml.md), and
[Deployment](37-deployment.md).

# Mailer

Gebweb's mailer abstraction is a thin layer over the stdlib SMTP
module: declarative `Mailable` subclasses describe each email,
a pluggable transport (SMTP / memory / log / custom) handles
delivery, and `gebweb.send(app, mailable)` automatically defers
to the background-job worker when one is registered.

## Wiring

```gb
import gebweb;

let app = gebweb.app([HomeController()]);
gebweb.useViews(app, "templates");
gebweb.useMailer(app, gebweb.smtpMailer({
    "host": "smtp.example.com",
    "port": 587,
    "username": "noreply",
    "password": "...",
    "from": "Acme <noreply@acme.dev>",
    "startTLS": true,
}));
```

For dev / tests pick a different transport:

| Transport | When | Behaviour |
|-----------|------|-----------|
| `gebweb.smtpMailer(opts)` | Production (any SMTP) | Delivers via stdlib SMTP. |
| `gebweb.sesMailer(opts)` | Production on AWS | Sigv4-signed POST to the SES v2 SendEmail endpoint. |
| `gebweb.memoryMailer()` | Tests | Captures every call to `.sent`; inspect from tests. |
| `gebweb.logMailer()` | Dev | Writes each message to stdout. |
| Custom | Anywhere | Implement `gebweb.MailerContract` (`deliver(to, subject, htmlBody, fromOverride)`). |

### AWS SES

`gebweb.sesMailer(opts)` signs each request with AWS Signature V4
and POSTs to `https://email.<region>.amazonaws.com/v2/email/
outbound-emails`. No long-lived connection; each delivery is one
HTTPS request.

```gb
gebweb.useMailer(app, gebweb.sesMailer({
    "region":    "eu-west-2",
    "accessKey": gebweb.parameter(app, "aws.accessKey") as string,
    "secretKey": gebweb.parameter(app, "aws.secretKey") as string,
    "from":      "Acme <noreply@acme.dev>"
}));
```

`opts.endpoint` overrides the default regional host (useful for
VPC interface endpoints or LocalStack):

```gb
gebweb.sesMailer({
    "region":    "us-east-1",
    "accessKey": "...",
    "secretKey": "...",
    "from":      "noreply@acme.dev",
    "endpoint":  "https://email-fips.us-east-1.amazonaws.com"
});
```

Sender identity (`from`) must be a verified SES address or come
from a verified domain. The address can be supplied via `opts.from`
as the default for every delivery, or per-message through a
`Mailable` instance setting `fromOverride`. A 4xx response (signing
mismatch, unverified sender, rate limit) surfaces as a
`RuntimeError` carrying the HTTP status and the SES error body.

## Defining a Mailable

```gb
class WelcomeMail extends gebweb.Mailable {
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

`template()` returns a view template name (resolved through
`gebweb.view(app, name, ctx)` - so `useViews` must be wired up
first). `context()` is the render context for that template.

## Sending

```gb
gebweb.send(app, WelcomeMail(user.email, user.name));
```

`send` automatically picks the right delivery mode:

- **When a job queue is registered** (`gebweb.useJobs`): the
  rendered message is enqueued as a `_gebweb_mailer_deliver`
  background job. The handler is registered automatically when
  you call `useMailer` after `useJobs`. Failures retry through
  the standard job retry/backoff policy.
- **Otherwise**: delivery is synchronous on the calling thread.

## Custom transports

Implement `gebweb.MailerContract` for any backend that isn't
covered by the built-ins (Mailgun, SendGrid, Postmark, ...):

```gb
class PostmarkMailer implements gebweb.MailerContract {
    string apiToken;

    func PostmarkMailer(string apiToken) {
        this.apiToken = apiToken;
    }

    func deliver(string to, string subject, string htmlBody, string fromOverride): void {
        let resp = http.postJson("https://api.postmarkapp.com/email", {
            "From": fromOverride.isEmpty() ? "noreply@app.dev" : fromOverride,
            "To": to, "Subject": subject, "HtmlBody": htmlBody,
        });
        if ((resp["status"] as int) >= 400) {
            throw errors.new("RuntimeError", "postmark: " + (resp["body"] as string));
        }
    }
}

gebweb.useMailer(app, PostmarkMailer(env.get("POSTMARK_TOKEN")));
```

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.Mailable` | Base class for email definitions. Override `subject()`, `template()`, `context()`; set `to` (and optional `fromOverride`). |
| `gebweb.MailerContract` | Transport interface: `deliver(to, subject, body, fromOverride)`. |
| `gebweb.useMailer(app, transport)` | Attach a transport; auto-registers the delivery job handler if a job queue exists. |
| `gebweb.smtpMailer(opts)` | SMTP transport. Opts: host, port, username, password, from, startTLS, tls. |
| `gebweb.sesMailer(opts)` | AWS SES v2 transport. Opts: region, accessKey, secretKey, from, endpoint?. |
| `gebweb.memoryMailer()` | Test transport. Captures sends in `.sent`. |
| `gebweb.logMailer()` | Dev transport. Writes to stdout. |
| `gebweb.send(app, mailable)` | Render + deliver (sync) or enqueue (async via the worker). |

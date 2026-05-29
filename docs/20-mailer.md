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
| `gebweb.smtpMailer(opts)` | Production | Delivers via stdlib SMTP. |
| `gebweb.memoryMailer()` | Tests | Captures every call to `.sent`; inspect from tests. |
| `gebweb.logMailer()` | Dev | Writes each message to stdout. |
| Custom | Anywhere | Implement `gebweb.MailerContract` (`deliver(to, subject, htmlBody, fromOverride)`). |

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
| `gebweb.memoryMailer()` | Test transport. Captures sends in `.sent`. |
| `gebweb.logMailer()` | Dev transport. Writes to stdout. |
| `gebweb.send(app, mailable)` | Render + deliver (sync) or enqueue (async via the worker). |

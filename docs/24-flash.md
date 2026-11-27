# Flash messages

<!-- doctest:strict -->

Flash messages are session-backed notifications for a post-redirect-get flow:
a handler adds a message to its response, the browser follows the redirect,
and a request-aware view renders the queued messages. The response then saves
session state without those messages, so a later request using the updated
cookie does not show them again.

They are for notices such as "Saved" or "Please check your input", not durable
business events or an exactly-once delivery mechanism.

## A complete redirect flow

Put this template in `templates/notices.html`. Categories are arbitrary strings;
the view decides which categories to display and how to style them:

<!-- doctest:file templates/notices.html -->
```html
{% for message in flashes.success %}<p class="success">{{ message }}</p>{% endfor %}
{% for message in flashes.info %}<p class="info">{{ message }}</p>{% endfor %}
<form method="post" action="/notice">
    <input name="name" required>
    <button>Submit</button>
</form>
```

This application acknowledges a submitted name and redirects with two
notifications. It uses a file session store. The small wrapper applies the same
cookie policy on every save, including the framework's later flash consumption.
Put it in `src/notice_app.gb`:

<!-- doctest:file notice_app.gb -->
```gb
import gebweb;
import web.session as session;

class BrowserSessions {
    session.FileSessionStore inner;
    bool secureCookies;

    func BrowserSessions(string directory, bool secureCookies) {
        this.inner = session.fileSessionStore(directory, 3600);
        this.secureCookies = secureCookies;
    }

    func load(dict<string, any> request): dict<string, any> {
        return this.inner.load(request);
    }

    func save(dict<string, any> response, dict<string, any> data, dict<string, any> options): dict<string, any> {
        let cookieOptions = options.copy();
        cookieOptions["httpOnly"] = true;
        cookieOptions["sameSite"] = "Lax";
        cookieOptions["secure"] = this.secureCookies;
        return this.inner.save(response, data, cookieOptions);
    }

    func clear(dict<string, any> response, dict<string, any> request): dict<string, any> {
        return this.inner.clear(response, request);
    }
}

class NameInput {
    @Assert.notBlank
    string name;
}

@Controller("/notice")
class NoticeController extends gebweb.Controller {
    @Get("")
    func page(gebweb.Request request): gebweb.Response {
        return this.view(request, "notices.html", {});
    }

    @Get("/status")
    func status(): dict<string, bool> {
        return {"ok": true};
    }

    @Post("")
    func submit(gebweb.Request request, NameInput body): dict<string, any> {
        let response = this.redirectWithFlash(
            request,
            "/notice",
            "success",
            "Hello, " + body.name + "!"
        );
        return this.flash(request, response, "info", "Messages are shown once.");
    }
}

export func build(string sessionDirectory, bool secureCookies = false): gebweb.GebwebApp {
    let app = gebweb.app();
    gebweb.useViews(app, "templates");
    gebweb.useSession(app, BrowserSessions(sessionDirectory, secureCookies));
    return app;
}
```

Use a temporary directory for tests and a persistent, application-owned location
for deployment. Set `secureCookies` to `true` for an HTTPS browser application.
The wrapper is application code, not an extra Gebweb configuration option.
For cookie-authenticated forms, also configure [CSRF protection](23-csrf.md);
the test here isolates flash behavior.

A browser carries session cookies automatically. `TestClient` does not, so the
test must keep each updated `name=value` pair. This helper belongs in
`src/flash_cookies.gb`:

<!-- doctest:file flash_cookies.gb -->
```gb
export func pair(any header, string name = "geb_sid"): string {
    let entries = header instanceof list ? header as list<any> : [header];
    for (entry in entries) {
        string value = (entry as string).split(";")[0];
        if (value.startsWith(name + "=")) {
            return value;
        }
    }
    throw RuntimeError("session cookie missing");
}
```

This sequence includes an intervening JSON request, which does not consume the
notifications. Only the view request does:

<!-- doctest:run -->
```gb
import flash_cookies;
import gebweb;
import io;
import notice_app;

let client = gebweb.TestClient(notice_app.build(io.tempDir("flash-example-")));
let redirect = client.post("/notice", {"name": "Ada"});
io.println(redirect.status);
io.println(redirect.headers["Location"]);
string cookie = flash_cookies.pair(redirect.headers["Set-Cookie"]);
let status = client.request("GET", "/notice/status", null, {"Cookie": cookie});
io.println(status.headers.contains("Set-Cookie"));

let first = client.request("GET", "/notice", null, {"Cookie": cookie});
io.println(first.text().contains("Hello, Ada!"));
io.println(first.text().contains("Messages are shown once."));
string updated = flash_cookies.pair(first.headers["Set-Cookie"]);
let second = client.request("GET", "/notice", null, {"Cookie": updated});
io.println(second.text().contains("Hello, Ada!"));
io.println(second.headers.contains("Set-Cookie"));
```

```text
303
/notice
false
true
true
false
false
```

The message data is stored server-side by these session stores; the cookie
carries a session ID, not the banner text. Treat that cookie as session state
and return the newest value on subsequent requests.

## Setup and controller helpers

`gebweb.useSession(app, store)` installs the flash, old-input and validation-error
view injectors and their response cleanup. `gebweb.useSessionAuth` performs the
same setup when configuring legacy session authentication. Combining those
setup calls does not install the response middleware twice.

For composed authentication with `sessionAuthenticator`, configure `useSession`
explicitly when the app also needs flash/form view state. Choose one store and
cookie identity for the browser flow. Register it during the app's `build(conn)`
setup, before serving with `gebweb.cli`; see
[Getting started](01-getting-started.md) and [Authentication](40-authentication.md).

| Controller helper | Result |
|-------------------|--------|
| `this.redirectWithFlash(request, location, category, message)` | A 303 redirect dictionary with a queued flash. |
| `this.flash(request, response, category, message)` | Append a flash to an existing response dictionary. |
| `this.view(request, template, context)` | Render with request-aware injectors and return an HTML response. |

These helpers require a controller extending `gebweb.Controller`, registered
through the framework so it is bound to its app. `redirectWithFlash` uses 303;
call `flash` with your own response dictionary when another status is needed.

Multiple flash calls composing one response retain earlier messages and the
other session data. Keep passing the returned response onward: each helper can
update its cookie headers. Flash writes and rendering are not a transaction
with a separate application database update.

## The facade API and cookie options

A plain controller or service can use:

`gebweb.flash(app, request, response, category, message, options = {})`.

The facade expects raw request/response dictionaries. Convert rich objects at
the boundary, for example `request.toDict()` and `response.toDict()`:

<!-- doctest:fragment context=script -->
```gb
@Controller("/profile")
class ProfileNotices {
    gebweb.GebwebApp app;

    func ProfileNotices(gebweb.GebwebApp app) {
        this.app = app;
    }

    @Post("/review")
    func review(gebweb.Request request): dict<string, any> {
        let response = http.redirect("/notice", 303);
        return gebweb.flash(
            this.app,
            request.toDict(),
            response.toDict(),
            "info",
            "Please review your profile."
        );
    }
}
```

The optional dictionary is forwarded to the session store's `save`, rather than
being a global flash configuration. The built-in stores use lower-camel-case
cookie option keys; these differ from `useCsrf`'s cookie options:

| Option | Built-in store default | Effect |
|--------|------------------------|--------|
| `path` | `"/"` | Cookie path. |
| `maxAge` | Store TTL | Browser retention in seconds. |
| `domain` | Omitted | Cookie domain. |
| `sameSite` | Omitted | SameSite attribute, such as `Lax`. |
| `secure` | Omitted/false | HTTPS-only cookie when enabled. |
| `httpOnly` | Omitted/false | Hide the cookie from JavaScript when enabled. |

A per-flash options dictionary applies to that save only. Automatic flash
consumption performs another save with an empty options dictionary. Apply
persistent cookie policy in the store, as `BrowserSessions` does, so it survives
both writes. Do not assume flags set on the redirect persist automatically on
the next session cookie.

## Rendering and consumption

The injected `flashes` value is a `dict<string, list<string>>`, grouped by
category. Messages within each category retain append order. Use whatever
category names your application understands; the framework does not assign
severity or styling.

Values are HTML-escaped by the view engine. This request demonstrates that a
name containing markup stays text in the banner:

<!-- doctest:run -->
```gb
import flash_cookies;
import gebweb;
import io;
import notice_app;

let client = gebweb.TestClient(notice_app.build(io.tempDir("flash-escape-")));
let redirect = client.post("/notice", {"name": "<b>Ada</b>"});
let page = client.request("GET", "/notice", null, {
    "Cookie": flash_cookies.pair(redirect.headers["Set-Cookie"]),
});
io.println(page.text().contains("&lt;b&gt;Ada&lt;/b&gt;"));
io.println(page.text().contains("<b>Ada</b>"));
```

```text
true
false
```

The consume sequence in a normal request is:

1. A request-aware render runs the `flashes` injector, reading the incoming
   session and marking the request for consumption when messages exist.
2. The template receives the grouped values. Reading them does not immediately
   mutate that request's session snapshot.
3. Session response middleware saves state without the flash queue and returns
   the updated cookie.

Use `this.view(request, ...)`, `gebweb.htmlView(app, request.toDict(), ...)` or
`gebweb.view(app, name, context, request.toDict())` so the request reaches the
injectors. Rendering a template without a request does not perform that flow.
An ordinary JSON response also does not consume flashes.

The injector runs even if the template does not display `flashes`; a page that
omits the banner can still consume it. Explicitly supplying a `flashes` key in
the view context overrides the injector. Multiple renders within one request
may read the same incoming messages; this is not a per-template delivery counter.
Concurrent requests or replayed old cookies are not an exactly-once mechanism.
For the normal one-shot browser flow, retain the response's latest session cookie.

## Session backends and lower-level access

The framework accepts a store with `load(request)` and
`save(response, data, options)`. The source-distributed `web.session` module
provides file, Redis and database stores. Authentication/logout integrations
may also need `clear(response, request)`.

| Store factory | Required configuration |
|---------------|------------------------|
| `session.fileSessionStore(directory, ttl, cookieName = "geb_sid")` | Application-owned directory and TTL in seconds. |
| `session.redisSessionStore(client, prefix, ttl, cookieName = "geb_sid")` | Redis client, key prefix and TTL. |
| `session.databaseSessionStore(conn, table, ttl, cookieName = "geb_sid")` | Database handle, table and TTL; initialize its table before use. |

Use shared persistence for browser sessions that must work across application
instances. Session data can expire before a user follows a redirect. Do not
store secrets, large payloads or the authoritative result of a business operation
only in a flash message.

For explicit access outside the automatic view flow, the underlying helpers
operate on raw request/response dictionaries:

| Helper | Behavior |
|--------|----------|
| `session.flashes(store, request)` | Read a raw list of `{category, message}` entries; do not clear it. |
| `session.withFlash(store, response, request, category, message, options)` | Append to the session loaded from that request and save it. |
| `session.clearFlashes(store, response, request, options)` | Remove flash data and return a saved response. |

These are lower-level store operations. They do not run Gebweb's request-aware
injector or response middleware. Prefer the framework/controller helpers for
composing application responses, and preserve updated cookie state whenever
you manage the flow manually.

## Reference

| API | Purpose |
|-----|---------|
| `gebweb.useSession(app, store)` | Register session storage and one-shot view-state handling. |
| `gebweb.flash(app, request, response, category, message, options = {})` | Append a flash and return an updated response dictionary. |
| `Controller.flash`, `Controller.redirectWithFlash` | Bound-controller helpers accepting rich or raw requests. |
| `flashes` | Grouped view context for request-aware rendering. |
| `session.flashes`, `withFlash`, `clearFlashes` | Lower-level session operations. |

Related chapters: [Views and templates](04a-views.md),
[Testing](14-testing.md), [CSRF](23-csrf.md), [Forms](25-forms.md),
[Authentication](40-authentication.md), and [Deployment](37-deployment.md).

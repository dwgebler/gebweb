# Flash Messages

Flash messages are session-backed one-shot values intended for
post-redirect-get banners ("Saved!", "Please log in"). A flash
written in handler N is rendered exactly once on handler N+1; after
that it's gone.

Gebweb wraps the stdlib `web.session` flash primitives and auto-
injects the queue into view contexts under `flashes`.

## Setup

Flash requires a session store. Register it once at app startup:

```gb
import gebweb;
import web.session as session;
import io;

let app = gebweb.app([HomeController()]);
gebweb.useViews(app, "templates");
gebweb.useSession(app, session.fileSessionStore(io.tempDir("app-sess-*"), 3600));
```

The stdlib provides `FileSessionStore` / `RedisSessionStore` /
`DatabaseSessionStore`; any value with `load(request)` and
`save(response, data, opts)` methods works. `gebweb.useSessionAuth`
also calls `useSession` internally, so apps that already wire
session-based auth need no extra step.

## Writing a flash

```gb
@Post("/posts")
func create(dict<string, any> req): dict<string, any> {
    /* ... persist the post ... */
    let response = gebweb.redirect("/posts");
    return gebweb.flash(app, req, response, "success", "Post created.");
}
```

`gebweb.flash(app, request, response, category, message, options?)`
returns the response with an updated `Set-Cookie` carrying the
appended message. Categories are arbitrary; common conventions are
`"success"`, `"error"`, `"info"`, `"warning"`.

## Reading in templates

The current flash queue is auto-injected into view contexts under
`flashes` as a `dict<string, list<string>>` keyed by category.

```html
{% for msg in flashes.success %}
    <div class="banner banner-ok">{{ msg }}</div>
{% endfor %}
{% if flashes.error %}
    <div class="banner banner-err">{{ flashes.error|first }}</div>
{% endif %}
```

Render through `gebweb.htmlView(app, request, name, ctx)` (or
`gebweb.view(app, name, ctx, request)` directly) so the framework
threads the request through and the injector fires.

## One-shot semantics

A flash written on request N is consumed on the next request that
calls `gebweb.htmlView` / `gebweb.view(..., request)`. The consume
step happens in a registered after-middleware that fires on every
response, clearing the queue from the session and writing the
empty payload back. Two consecutive renders won't show the same
message twice.

## Manual API (escape hatch)

For tooling or background workers that touch flash storage directly,
the stdlib helpers are public:

```gb
import web.session as session;

session.withFlash(store, response, request, "info", "Hello", {});
let queue = session.flashes(store, request);
session.clearFlashes(store, response, request, {});
```

`gebweb.flash(...)` is a thin wrapper that pulls the store from the
app and forwards.

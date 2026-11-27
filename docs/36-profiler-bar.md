# Dev profiler bar

<!-- doctest:strict -->

`gebweb.useProfilerBar(app)` adds a collapsible toolbar to in-memory HTML
responses. Use it during development to inspect request elapsed time, recorded
timings and process resource measurements. It is disabled unless the environment
is explicitly development, or you enable it through an option.

## Mounting

Put this application in `src/profiler_app.gb`. The controller records timings
on the current request. The build function accepts profiler options so tests
can explicitly enable the bar without changing the process environment.

<!-- doctest:file profiler_app.gb -->
```gb
import gebweb;
import log;
import time;

class StatusReply {
    bool ok = true;
}

@Controller("/reports")
class ReportController {
    @Get("")
    func report(gebweb.Request request): dict<string, any> {
        string title = gebweb.measureTiming(request.toDict(), "report.load", func(): string { return "Development report"; }) as string;
        int start = time.monotonic();
        string body = "<html><body><h1>" + title + "</h1></body></html>";
        gebweb.recordTiming(request.toDict(), "report.render", time.monotonic() - start);
        return gebweb.html(body);
    }

    @Get("/data")
    func data(): StatusReply {
        return StatusReply();
    }
}

export func build(dict<string, any> profilerOptions = {}): gebweb.GebwebApp {
    let app = gebweb.app();
    gebweb.useLogger(app, log.stderr());
    gebweb.useProfilerBar(app, profilerOptions);
    return app;
}
```

The following check forces the bar on, confirms it appears before `</body>`,
and checks that JSON and explicitly disabled HTML remain unchanged. It checks
stable behavior instead of machine-dependent timing values.

<!-- doctest:run -->
```gb
import gebweb;
import io;
import profiler_app;

let app = profiler_app.build({"enabled": true});
let client = gebweb.TestClient(app);
let page = client.get("/reports");
string body = page.text();
io.println(page.status);
io.println(body.contains("id=\"gebweb-profiler-bar\""));
io.println(body.indexOf("id=\"gebweb-profiler-bar\"") < body.lastIndexOf("</body>"));
io.println(body.contains("report.load") && body.contains("report.render"));
io.println(client.get("/reports/data").text());

let disabled = profiler_app.build({"enabled": false});
io.println(gebweb.TestClient(disabled).get("/reports").text().contains("gebweb-profiler-bar"));
```

```text
200
true
true
true
{"ok":true}
false
```

Open an HTML route in a browser to use the bar. Click Time, Memory or Request
to expand that panel; clicking its segment again collapses it. Only one panel
is open at a time.

## When it runs

| Option | Default | Effect |
|--------|---------|--------|
| `enabled` | Environment gate | Boolean override, evaluated when the middleware is mounted. |

Without an override, only `GEBWEB_ENV=dev` and `GEBWEB_ENV=development` enable
the bar, case-insensitively. Unset, production, staging and unknown values keep
it off. Disabled mounting returns the app without registering profiler
middleware. Changing the environment afterwards does not change an existing
mount. Mount it once during application setup.

The enabled middleware takes a starting snapshot for requests, including
requests that eventually return non-HTML. Injection requires a `Content-Type`
starting with `text/html` and an in-memory body. JSON and other content types
pass through; streamed file responses are explicitly skipped even when their
type is HTML. The check is based on content type, rather than status: an HTML
error or redirect response can also receive a bar.

The bar is inserted before the last literal `</body>`. Without that exact
closing tag, it is appended to the body, including for HTML fragments. It is a
string insertion, not an HTML parser.

## The panels

| Panel | Measurements |
|-------|--------------|
| Time | Elapsed milliseconds from the profiler's before hook to its response hook, plus recorded labels and durations. |
| Memory | Process heap delta over that interval, sampled peak heap, process CPU time delta and GC cycle delta. |
| Request | Method, path, status and response content type as seen by the response hook. |

The elapsed time does not include sending the response to the client or the
bar's own rendering. Middleware order affects which work falls inside the
interval. It is not a browser page-load measurement.

Memory, CPU and GC values are process-wide, so concurrent requests and
background tasks contribute. Heap delta can be negative after collection.
Peak heap is the highest allocation value observed by profiler samples in the
process, not a guaranteed continuous high-water mark or a per-request peak.
Use these figures as development clues rather than isolated request accounting
or production capacity measurements. The bar itself adds profiling and HTML
rendering overhead.

## Recording timeline entries

`recordTiming(requestDict, label, durationMs)` appends an integer millisecond
duration. `measureTiming(requestDict, label, callback)` runs the callback,
records its elapsed duration and returns its result. A thrown callback error
escapes without recording that entry.

With a typed `gebweb.Request`, pass `request.toDict()` to these public helpers,
as the controller above does. Keep timings on the active request; do not share
a timing dictionary between requests or mutate its list concurrently from
background tasks. Labels are HTML-escaped when the bar renders them.

The timeline is empty until application code records entries. The bar does not
automatically trace database queries or template rendering. It shares its
request timing list with `gebweb.useServerTiming(app)`, which separately emits
a `Server-Timing` header; mounting the bar alone does not emit that header.

## Content Security Policy

When `useSecurity` generates a request CSP nonce, the bar attaches that nonce
to its inline `<style>` and `<script>`. Include the nonce source in both
`styleSrc` and `scriptSrc`; a policy that disallows the tags can prevent the
bar from working even though its HTML is present.

This runnable check verifies that both tags use the nonce in the policy:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import profiler_app;
import re;

let app = profiler_app.build({"enabled": true});
gebweb.useSecurity(app, {
    "csp": {
        "defaultSrc": ["'self'"],
        "scriptSrc": ["'self'", "'nonce'"],
        "styleSrc": ["'self'", "'nonce'"],
    },
});
let response = gebweb.TestClient(app).get("/reports");
string policy = response.headers["Content-Security-Policy"] as string;
let found = re.match("'nonce-([^']+)'", policy) as dict<string, any>;
string nonce = found["groups"][1] as string;
io.println(!nonce.isEmpty());
io.println(response.text().contains("<script nonce=\"" + nonce + "\">"));
io.println(response.text().contains("<style nonce=\"" + nonce + "\">"));
```

```text
true
true
true
```

## Requirements

Use the framework's current supported Geblang version, 1.47.0 or later. The bar
uses the runtime's monotonic clock and profiler module.

Keep it disabled on public production routes. The environment gate is not
authentication: `enabled: true` overrides it even in production and exposes
request details and application timing labels to the response recipient.
Avoid serving cached development HTML after disabling the bar. For operational
monitoring, see [observability](32-health-checks.md) instead.

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.useProfilerBar(app, opts = {})` | Mount the development bar when enabled; return the app. |
| `gebweb.recordTiming(requestDict, label, durationMs)` | Append a timeline entry to the request. |
| `gebweb.measureTiming(requestDict, label, callback)` | Time a successful callback and return its result. |
| `gebweb.useServerTiming(app)` | Emit the shared timing entries as a response header. |

See [security](27-security.md) for CSP configuration and
[middleware](09-middleware.md) for response-hook ordering.

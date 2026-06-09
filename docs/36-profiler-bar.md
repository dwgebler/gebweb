# Dev profiler bar

`gebweb.useProfilerBar(app)` mounts a collapsible profiler toolbar that
injects itself into the bottom of every HTML response. It is a
development aid: at a glance you see how long a request took, where the
time went, how much memory it used, and the basic request facts. It is
enabled in non-prod environments and turns into a no-op in production.

## Mounting

One call after building the app wires it up:

```gb
import gebweb;

class HomeController {
    @Get("/")
    func index(): string {
        return gebweb.html("<html><body><h1>Home</h1></body></html>");
    }
}

let app = gebweb.app([HomeController()]);
gebweb.useProfilerBar(app);
gebweb.serve(app, ":8080");
```

Open any HTML page and the bar appears pinned to the bottom of the
viewport. Click a segment to expand its panel.

## When it runs

By default the bar is gated on the environment: it is enabled whenever
`GEBWEB_ENV` is not `prod` (and `GEBWEB_ENV` defaults to `prod` when
unset). That means it is off in production with no cost, since mounting
registers no middleware at all when disabled.

Override the gate explicitly with the `enabled` option:

```gb
gebweb.useProfilerBar(app, {"enabled": true});   /* force on  */
gebweb.useProfilerBar(app, {"enabled": false});  /* force off */
```

The bar only ever touches HTML responses. A response whose
`Content-Type` does not start with `text/html` (JSON, files, streams,
redirects) passes through untouched, so it never corrupts an API
response or a download.

## The panels

The bar shows three panels, each summarised in the bar itself and
expandable for detail:

- **Time** - total request time in milliseconds, plus a table of every
  recorded timeline entry (label and duration).
- **Memory** - heap delta for the request and the process peak heap,
  plus CPU time and GC cycles.
- **Request** - method, path, status code, and response content-type.

## Recording timeline entries

The Time panel's table is fed by timings your handlers record. Call
`gebweb.recordTiming` with the request, a label, and a duration in
milliseconds:

```gb
class ReportController {
    @Get("/report")
    func report(dict<string, any> request): string {
        gebweb.recordTiming(request, "query", 18);
        gebweb.recordTiming(request, "render", 4);
        return gebweb.html("<html><body>Report</body></html>");
    }
}
```

Each recorded entry becomes a row in the Time panel. The bar works fine
with no recorded timings: the table is simply empty and only the total
request time is shown.

## Requirements

The profiler bar relies on the language runtime's monotonic clock and
the `profiler` module, so it requires geblang >= 1.14.0.

## Reference

- `gebweb.useProfilerBar(GebwebApp app, dict<string, any> opts = {})` -
  mount the bar. `opts["enabled"]` (bool) overrides the environment
  gate. Returns the app.
- `gebweb.recordTiming(dict<string, any> request, string label, int
  durationMs)` - record one timeline entry for the current request.

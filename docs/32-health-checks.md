# Health checks

<!-- doctest:strict -->

`gebweb.useHealth` mounts GET endpoints for liveness and readiness. Each
request runs the registered probes of that kind and aggregates their results:
200 when all pass, 503 when any fail. Configure your deployment's monitor paths
and response policy explicitly; the framework does not configure an
orchestrator or load balancer.

## Wiring

Put these probes in `src/site_probes.gb`. Liveness only checks that this code
can run. Readiness checks a real database and an application-owned queue state:

<!-- doctest:file site_probes.gb -->
```gb
import db;
import gebweb;
import store;

export class SiteProbes {
    db.Connection conn;
    store.Store state;

    func SiteProbes(db.Connection conn, store.Store state) {
        this.conn = conn;
        this.state = state;
    }

    @HealthCheck(name: "process", kind: "liveness")
    func process(): bool {
        return true;
    }

    @HealthCheck(name: "database", kind: "readiness", timeout: 2000)
    func database(): gebweb.ProbeResult {
        try {
            this.conn.queryOne("SELECT 1");
            return gebweb.ProbeResult(true, "Database reachable", {"dependency": "database"});
        } catch (Error error) {
            return gebweb.failing("Database readiness failed");
        }
    }

    @HealthCheck(name: "queue", kind: "readiness")
    func queue(): bool {
        return this.state.get("queueReady") == true;
    }
}
```

Pass already-constructed instances to `useHealth`. It scans their decorated
methods; it does not discover arbitrary probe classes from source directories:

<!-- doctest:run -->
```gb
import db;
import gebweb;
import io;
import site_probes;
import store;

let conn = db.connect("sqlite", ":memory:");
try {
    let state = store.Store();
    let app = gebweb.app([], {"docs": false});
    gebweb.useHealth(app, [site_probes.SiteProbes(conn, state)]);
    let client = gebweb.TestClient(app);
    io.println(client.get("/healthz").status);
    let response = client.get("/readyz");
    let body = response.json() as dict<string, any>;
    let checks = body["checks"] as dict<string, any>;
    io.println(response.status);
    io.println(body["status"]);
    io.println(checks["database"]["status"]);
    io.println(checks["queue"]["status"]);
    io.println(checks["database"]["details"]["dependency"]);
    io.println(checks.contains("process"));
    state.set("queueReady", true);
    io.println(client.get("/readyz").status);
} finally {
    conn.close();
}
```

```text
200
503
fail
ok
fail
database
false
200
```

The unset queue state fails readiness without affecting liveness. Setting it
ready changes the next probe result; health results are not cached. In an
application, put setup in `build(conn)`, register resource cleanup with
`onShutdown`, and use the [standard entry point](01-getting-started.md).

| `useHealth` option | Default | Meaning |
|--------------------|---------|---------|
| `livenessPath` | `"/healthz"` | GET route running liveness probes. |
| `readinessPath` | `"/readyz"` | GET route running readiness probes, or reporting drain state. |

Reserve distinct paths for these endpoints and configure them once during
startup. An empty set of matching probes returns 200 with empty `checks`; a
healthy endpoint alone does not prove that dependency probes were registered.

## Writing probes

`@HealthCheck` marks an instance method that can be called without arguments:

| Argument | Default | Meaning |
|----------|---------|---------|
| `name` | `<Class>.<method>` | Key in `checks`; use explicit unique names for stable monitoring. |
| `kind` | `"readiness"` | Use exactly `"liveness"` or `"readiness"`. |
| `timeout` | `5000` | Per-probe wait timeout in milliseconds. Choose a positive bound. |

The first positional string can supply the name. Use named `kind` and
`timeout`; they are not positional fields. Duplicate names overwrite entries
in the `checks` dictionary, so give each probe its own name.

Return a `bool` or `gebweb.ProbeResult`:

| Return | Meaning |
|--------|---------|
| `true` / `false` | Pass / fail, without a message or details. |
| `gebweb.passing(message = "")` | Successful structured result. |
| `gebweb.failing(message)` | Failed structured result. |
| `gebweb.ProbeResult(ok, message = "", details = {})` | Explicit result with optional structured diagnostics. |

Other returned values are currently treated as passing; do not rely on a
missing/incorrect return type to fail a check. Throwing an `Error` becomes a
failed check whose message is the exception message. Avoid exposing raw
connection strings, tokens or upstream error bodies: return a safe diagnostic
as the database probe does above.

Probes run sequentially, each through an asynchronous task with its own wait
timeout. One failure does not skip the remaining probes. The endpoint's total
latency can approach the sum of probe waits, rather than the largest one.
Keep probes cheap, read-only and safe when monitor requests overlap.

The health timeout does not forcibly stop the underlying operation. Give
database/network calls their own deadlines and a cleanup/ownership policy;
repeated health polling must not accumulate permanently blocked work.

## Response shape

Each matching check has `status` (`"ok"` or `"fail"`) and integer
`duration_ms`. Nonempty `message` and `details` are included when supplied.
Timings vary, so these examples assert stable fields rather than exact JSON
with a guessed duration:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class DiagnosticProbe {
    @HealthCheck(name: "dependency")
    func dependency(): gebweb.ProbeResult {
        throw RuntimeError("Dependency unavailable");
    }
}

let app = gebweb.app([], {"docs": false});
gebweb.useHealth(app, [DiagnosticProbe()]);
let response = gebweb.TestClient(app).get("/readyz");
let body = response.json() as dict<string, any>;
let check = (body["checks"] as dict<string, any>)["dependency"] as dict<string, any>;
io.println(response.status);
io.println(body["status"]);
io.println(check["status"]);
io.println(check["message"]);
io.println(check["duration_ms"] instanceof int);
```

```text
503
fail
fail
Dependency unavailable
true
```

A timed-out check also has `status: "fail"`, with its timeout error message;
there is no separate `"timeout"` status in this response contract.

## Liveness vs readiness

Use liveness as a process-health signal and readiness as a traffic-admission
signal. Keep liveness independent of external dependency outages, so a lost
database does not encourage restarting an otherwise working process.
Readiness can check the dependencies essential to serving your traffic.

Health routes run through app middleware. An app-wide login gate, required
tenant resolver or rate limit can reject a monitor before probes run. Decide
which endpoints monitors may reach and configure those gates/network access
accordingly. Probe details may reveal internal state; expose only diagnostics
appropriate for their audience. See [Middleware](09-middleware.md),
[Multi-tenancy](31-multi-tenancy.md) and [Security](27-security.md).

## The ops bundle: useOps

`useOps(app, instances = [], opts = {})` calls both `useHealth` and
`useMetrics`. It passes the same flat options dictionary to each; `path`
controls metrics, not the health paths:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app([], {"docs": false});
gebweb.useOps(app, [], {"livenessPath": "/live", "readinessPath": "/ready", "path": "/observe"});
let client = gebweb.TestClient(app);
io.println(client.get("/live").status);
io.println(client.get("/ready").status);
io.println(client.get("/observe").headers["Content-Type"]);
io.println(client.get("/healthz").status);
```

```text
200
200
text/plain; version=0.0.4
404
```

Use `useHealth` and `useMetrics` separately when you want only one surface.
Do not install the bundle and then register the same endpoints again.

## Metrics

`gebweb.useMetrics(app, {"path": "/metrics"})` mounts a Prometheus text
endpoint; `/metrics` is the default. Controller-route adapters record requests
while it is configured, including route-level auth/CSRF/rate-limit responses.
Labels use route templates rather than per-record paths:

<!-- doctest:run -->
```gb
import gebweb;
import io;

@Controller("/widgets")
class MetricsWidgets {
    @Get("/{id}")
    func show(string id): dict<string, any> {
        return {"id": id};
    }
}

let app = gebweb.app();
gebweb.useMetrics(app);
let client = gebweb.TestClient(app);
client.get("/widgets/one");
client.get("/widgets/two");
let response = client.get("/metrics");
string metrics = response.text();
io.println(response.status);
io.println(
    metrics.contains(
        'gebweb_http_requests_total{method="GET",route="/widgets/{id}",status="200"} 2'
    )
);
io.println(
    metrics.contains(
        'gebweb_http_request_duration_milliseconds_count{method="GET",route="/widgets/{id}"} 2'
    )
);
io.println(metrics.contains("gebweb_http_requests_in_flight 0"));
```

```text
200
true
true
true
```

| Metric | Meaning |
|--------|---------|
| `gebweb_http_requests_total{method,route,status}` | Completed controller-adapter responses by status. |
| `gebweb_http_request_duration_milliseconds_sum` / `_count` | Adapter duration totals/counts per method and route. |
| `gebweb_http_requests_in_flight` | Controller adapters currently executing. |

These counters belong to one app instance/process and reset with it. They are
not a count of every incoming connection/request: unknown routes, static
assets, health/metrics routes and rejection by app-wide before middleware do
not run these controller adapters. Status/duration are measured at the adapter
boundary, before any later app response middleware changes. Streaming response
lifetimes are not represented by the adapter's duration.

Protect `/metrics` at the proxy or with app middleware when it is internal.
Choose monitoring frequency and probe deadlines together to avoid turning
observability into a dependency load generator.

## Graceful shutdown and drain

`gebweb.shutdown(app, {"timeoutMs": 10000})` marks the app draining and attempts
to stop tracked listeners/workers and run cleanup within the shutdown budget.
It returns false when phases fail or remain incomplete; it does not forcibly
terminate uncooperative workers or timed-out hooks, which may continue running.
The readiness handler returns 503 `{"status":"draining"}` instead of running
probes once draining begins. Liveness does not automatically become unhealthy.

This in-process example checks the state change without opening a listener:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import log;

let app = gebweb.app([], {"docs": false});
gebweb.useLogger(app, log.stderr());
gebweb.useHealth(app, []);
let client = gebweb.TestClient(app);
io.println(client.get("/readyz").status);
io.println(gebweb.shutdown(app, {"timeoutMs": 1000}));
io.println(gebweb.isDraining(app));
io.println(client.get("/readyz").text());
```

```text
200
true
true
{"status":"draining"}
```

`TestClient` can still dispatch after shutdown; a real server stops accepting
connections. Shutdown does not promise a separate period during which a load
balancer can poll readiness before the listener stops. Coordinate traffic
withdrawal with deployment policy. `gebweb.cli` installs shutdown handling;
use `onShutdown` for resource cleanup and `isDraining` in application-owned
worker loops. See [Deployment](37-deployment.md#shutdown-and-drain) and
[Background jobs](18-background-jobs.md).

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.useHealth(app, instances, opts = {})` | Register probe instances and mount health routes. |
| `@HealthCheck(name: ..., kind: ..., timeout: ...)` | Mark a no-argument probe method. |
| `gebweb.ProbeResult(ok, message = "", details = {})` | Structured result type. |
| `gebweb.passing(message = "")` / `gebweb.failing(message)` | Result conveniences. |
| `gebweb.useMetrics(app, opts = {})` | Mount per-controller-route metrics. |
| `gebweb.useOps(app, instances = [], opts = {})` | Install health and metrics endpoints. |
| `gebweb.shutdown(app, opts = {})` | Drain/shut down; return whether shutdown completed cleanly. |
| `gebweb.isDraining(app)` | Read the app's drain state. |

See [Middleware](09-middleware.md), [Resource ownership](39-best-practices.md#close-request-resources-and-observe-background-work),
[Deployment](37-deployment.md) and [Testing](14-testing.md).

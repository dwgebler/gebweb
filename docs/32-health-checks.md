# Health checks

Liveness and readiness endpoints discoverable via decorators.
Kubernetes, ECS, and most reverse proxies expect `/healthz` and
`/readyz` to return 200 when the app is OK and 503 when it isn't.
Gebweb mounts both, runs every registered probe on a hit, and
aggregates the results into a JSON body.

## Wiring

```gb
import gebweb;

let app = gebweb.app([HomeController()]);
gebweb.useHealth(app, [DatabaseProbe(conn), QueueProbe(brokerConn)]);
```

Each instance is scanned for `@HealthCheck` methods. The default
endpoints are `/healthz` (liveness) and `/readyz` (readiness);
override via `{"livenessPath": "...", "readinessPath": "..."}`.

## Writing probes

```gb
class DatabaseProbe {
    db.Conn conn;
    func DatabaseProbe(db.Conn conn) { this.conn = conn; }

    @HealthCheck(name: "db", kind: "readiness", timeout: 2000)
    func ping(): gebweb.ProbeResult {
        try {
            this.conn.queryOne("SELECT 1");
            return gebweb.passing();
        } catch (Error e) {
            return gebweb.failing("DB ping failed: " + e.message);
        }
    }
}

class MemoryProbe {
    @HealthCheck(name: "memory", kind: "liveness")
    func memory(): bool {
        return sys.freeMemoryMb() > 64;
    }
}
```

`@HealthCheck` accepts three named args:

| Arg | Default | Meaning |
|-----|---------|---------|
| `name` | `<Class>.<method>` | Identifier surfaced on the response body. |
| `kind` | `"readiness"` | `"liveness"` or `"readiness"`. Liveness runs on `/healthz`, readiness on `/readyz`. |
| `timeout` | `5000` | Per-probe timeout in ms. A timing-out probe fails the overall check with `status: "timeout"`. |

A probe method returns `gebweb.ProbeResult` for the structured
case (status + message + details) or a plain `bool`. Throwing is
equivalent to returning a failing result whose message is the
exception text.

## Response shape

```json
{
  "status": "ok",
  "checks": {
    "db": {"status": "ok", "duration_ms": 4},
    "memory": {"status": "ok", "duration_ms": 0}
  }
}
```

When any probe fails the response uses status 503 and the
top-level `status` becomes `"fail"`. Each entry carries the probe
duration so observability tools can latch onto degradation
trends.

## Liveness vs readiness

- **Liveness** answers "should this process be killed?" Probes
  should be cheap and never touch external dependencies; failing
  liveness will cause the orchestrator to restart the pod.
- **Readiness** answers "is this process ready to serve
  traffic?" Probes touch the database, message broker, downstream
  services. Failing readiness pulls the pod out of the load
  balancer pool without restarting it.

The default kind is `readiness` because that's the more useful
probe in practice; mark the cheap process-health probes as
`liveness` explicitly.

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.useHealth(app, instances, opts)` | Mount `/healthz` + `/readyz` and discover probes. |
| `@HealthCheck(name, kind, timeout)` | Mark a method as a probe. |
| `gebweb.ProbeResult` | `bool ok`, `string message`, `dict<string, any> details`. |
| `gebweb.passing(message)` | Convenience constructor. |
| `gebweb.failing(message)` | Convenience constructor. |

## The ops bundle: useOps

`gebweb.useOps(app, instances, opts)` mounts everything a production
deployment expects in one call: the liveness and readiness probes
above plus a Prometheus metrics endpoint:

```gb
gebweb.useOps(app, [DatabaseProbe(conn)]);
```

## Metrics

`gebweb.useMetrics(app)` (included in `useOps`) serves Prometheus
text format at `/metrics` (override with `{"path": "..."}`):

- `gebweb_http_requests_total{method,route,status}` - request counts
  labelled by route template (bounded cardinality, not raw paths).
- `gebweb_http_request_duration_milliseconds_sum` / `_count` - latency
  totals per route for rate and average-latency queries.
- `gebweb_http_requests_in_flight` - current in-flight gauge.

Every route records automatically, including early rejections from
rate limiting or CSRF. Protect the endpoint at the proxy or with
middleware if it should not be public.

## Graceful shutdown and drain

`gebweb.shutdown(app, {"timeoutMs": 10000})` drains the app: the
readiness endpoint flips to 503 with `{"status": "draining"}` so
load balancers and rolling deploys stop sending traffic, in-flight
requests finish within the deadline, then the listener closes and
`gebweb.serve` (or `http.wait` on a `gebweb.listen` handle) returns.
It is safe to call from a signal handler; `gebweb.cli` installs
exactly that for SIGINT and SIGTERM.

Long-running loops (job workers, scheduler ticks) should poll
`gebweb.isDraining(app)` and exit when it flips.

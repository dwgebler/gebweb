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

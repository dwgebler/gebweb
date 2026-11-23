# Deployment: the standard server entrypoint

`gebweb.cli(app, opts)` is the standard way to serve an app in
production. It gives every Gebweb binary the same operational
surface: port and bind-address flags with environment fallbacks,
HTTPS via LetsEncrypt or a generated local certificate, a startup
banner, `--help`, `--version`, and a graceful drain on SIGINT / SIGTERM.

```gb
import gebweb;

let app = gebweb.app([HomeController()]);
gebweb.useOps(app);
gebweb.cli(app, {"name": "myapp", "port": 8080});
```

```text
./myapp                       # http://localhost:8080
./myapp --port 9000           # flag beats env beats opts
./myapp --port=9000           # the same, in key=value form
./myapp --self-signed         # https://localhost:443, generated cert
./myapp --domain example.com  # LetsEncrypt on :443, HTTP redirect on :80
./myapp --help                # prints the option table
./myapp --version             # app, Geblang and package versions
```

Because `main(list<string> args)` takes arguments, a built binary passes
`--help` and `--version` straight to the app. The runtime's own flags are
`--geblang-version` and `--geblang-notices` (third-party licence notices).

## Checking arguments before setup

`gebweb.cli` checks its arguments when it starts serving, which is after the
rest of `main` has run. When setup needs secrets, opens databases or creates
files, call `cli.checkArgs` first so `--help`, `--version` and mistyped options
are answered before any of that:

```gb
import gebweb.cli as cli;

export func main(list<string> args): int {
    cli.checkArgs(args, {"name": "myapp"});
    let secret = loadSecretOrFail();
    # ... build the app ...
    gebweb.cli(app, {"name": "myapp"});
    return 0;
}
```

## Development mode

`--dev`, or `GEBWEB_ENV=dev` / `GEBWEB_ENV=development` (any case), sets
`dev: true` in the resolved configuration and adds `(development mode)` to the
serving banner. When `cli.checkArgs` sees `--dev` it also switches the process
environment to `dev`, so everything that reads `gebweb.currentEnv()` agrees:
the [profiler bar](36-profiler-bar.md) mounts, `services_dev.yaml` overlays
load, and 500 responses carry error details (see
[Responses](04-responses.md#development-error-details)). Call `checkArgs` before
building the app so those setup-time decisions see it. Beyond that, the app
decides what dev mode relaxes. A typical use is accepting a built-in development secret so a built
binary can run locally, while production (no flag, `GEBWEB_ENV` unset or any
other value) keeps refusing it:

```gb
let cfg = cli.checkArgs(args, {"name": "myapp"});
if (!(cfg["dev"] as bool) && secret.isEmpty()) {
    io.stderrPrintln("myapp: SECRET must be set");
    return 2;
}
```

`checkArgs(args, opts)` prints the option table and exits 0 for `--help`,
prints the version block and exits 0 for `--version`, and prints each problem
plus the option table to stderr and exits 2 for an unknown option or a missing
value. Otherwise it returns the resolved configuration. `opts` takes the same
defaults as `gebweb.cli`.

## Options and precedence

Resolution order is flags > environment > `opts` defaults. Every option that
takes a value accepts `--option value` or `--option=value`.

| Flag | Environment | `opts` key | Default | Meaning |
|------|-------------|-----------|---------|---------|
| `--port N` | `GEBWEB_PORT` (or `GEBWEB_HTTP_PORT`) | `port` | `8080` | plain-HTTP port |
| `--tls-port N` | `GEBWEB_TLS_PORT` | `tlsPort` | `443` | TLS port |
| `--host ADDR` | `GEBWEB_HOST` | `host` | all interfaces | bind address |
| `--domain HOST` | `GEBWEB_DOMAIN` | `domain` | - | LetsEncrypt autocert host |
| `--self-signed` | `GEBWEB_SELF_SIGNED` | `selfSigned` | off | local HTTPS, generated cert |
| `--no-tls` | `GEBWEB_NO_TLS` | `noTls` | off | force plain HTTP |
| `--acme-email A` | `GEBWEB_ACME_EMAIL` | `acmeEmail` | - | ACME contact address |
| `--acme-cache DIR` | `GEBWEB_ACME_CACHE` | `acmeCache` | - | certificate cache dir |
| `--http MODE` | `GEBWEB_HTTP` | `http` | see below | plain-HTTP port behaviour when TLS is active |
| `--cert-out FILE` | `GEBWEB_CERT_OUT` | `certOut` | - | write the server certificate PEM to FILE |
| `--trusted-proxies LIST` | `GEBWEB_TRUSTED_PROXIES` | `server.trustedProxies` | - | comma-separated proxy IPs/CIDRs whose `X-Forwarded-*` headers are honoured |
| `--help` | - | - | - | print the option table |
| `--version` | - | - | - | print the app, Geblang and bundled package versions |
| `--dev` | `GEBWEB_ENV` (`dev` or `development`) | - | off | development mode, see below |

`opts.name` sets the binary's display name in the banner and help
text.

`opts.server` is a dict of `http.listen` options applied to every
listener the entrypoint starts (the main listener, the TLS listener and
the plain-HTTP sidecar), for example `maxConns`, `maxBodyBytes`,
`trustedProxies`, the read and write timeouts (`readHeaderTimeoutMs`,
`readBodyTimeoutMs`, `idleTimeoutMs`, `writeTimeoutMs`) or the WebSocket
timeouts (`webSocketPingIntervalMs`, `webSocketIdleTimeoutMs`,
`webSocketWriteTimeoutMs`):

```gb
gebweb.cli(app, {"port": 8080, "server": {"trustedProxies": ["10.0.0.0/8"], "maxConns": 512}});
```

Behind a reverse proxy or load balancer, set the trusted proxies (here,
with the flag or `GEBWEB_TRUSTED_PROXIES`, which replace
`server.trustedProxies`). Only then does `request.clientIp()` - and so the
rate limiter, audit logs and anything keyed on the client - see the
real client address instead of the proxy's.

## HTTPS modes

**Production (LetsEncrypt):** pass `--domain example.com` (or set
`GEBWEB_DOMAIN`). The app serves HTTPS on the TLS port with
certificates obtained and renewed automatically, and a second
listener on the plain-HTTP port answers every request with a 301 to
the HTTPS host. Set `--acme-cache` to a persistent directory so
restarts reuse certificates instead of re-requesting them (ACME
providers rate-limit issuance), and `--acme-email` to receive expiry
notices. The domain must resolve to the host and ports 80/443 must
be reachable - that is an ACME requirement, not a Gebweb one.

**Local development (self-signed):** pass `--self-signed`. The app
serves HTTPS with a certificate generated at startup for
`localhost`. Browsers warn on first visit because the certificate is
not chain-trusted; export it with `--cert-out` and add it to your
local trust store to make the warnings (and `curl -k`) go away:

```text
./myapp --self-signed --cert-out dev-cert.pem

# verify without -k:
curl --cacert dev-cert.pem https://localhost/...

# trust it system-wide:
#   Debian/Ubuntu: sudo cp dev-cert.pem /usr/local/share/ca-certificates/myapp-dev.crt
#                  sudo update-ca-certificates
#   macOS:         sudo security add-trusted-cert -d -k /Library/Keychains/System.keychain dev-cert.pem
#   Firefox:       Settings > Certificates > Import (trusts its own store)
```

The certificate is regenerated each start, so re-export after a
restart (or keep the file and re-trust only when it changes). Useful
whenever a feature needs a secure context (cookies with `Secure`,
service workers, HTTP/2). Programs using the engine directly get the
same PEM from `http.serverCert(handle)` on a `http.listen` handle.

**The plain-HTTP port alongside TLS:** `--http` controls what the
plain-HTTP port does while TLS is serving:

- `off` - no HTTP listener (the self-signed default).
- `redirect` - a 301 redirect to the TLS host, preserving path and
  query (the LetsEncrypt default; the redirect targets the autocert
  domain, or the request's own Host header in self-signed mode).
- `serve` - the full app on both ports at once (plain HTTP for
  internal callers, TLS for external ones).

```text
./myapp --self-signed --http redirect   # 443 TLS + 80 redirecting
./myapp --self-signed --http serve      # the app on both 80 and 443
./myapp --domain example.com --http off # TLS only, no port-80 listener
```

Every listener the entrypoint starts is tracked for graceful drain,
so `gebweb.shutdown` (and SIGTERM) stops redirect and dual-port
listeners along with the TLS server.

`--no-tls` overrides both modes - handy when TLS terminates at a
proxy in one environment but not another, with the same binary.

Apps that need certificate options the entrypoint does not expose
(mTLS client CAs, explicit cert/key pairs) call `gebweb.serve(app,
addr, {"tls": {...}})` directly with the full engine TLS option set
(see the geblang `http` module reference).

## Shutdown and drain

On SIGINT or SIGTERM the entrypoint calls
`gebweb.shutdown(app, {"timeoutMs": 10000})` and logs each phase:

```text
myapp: received SIGINT, shutting down (timeout 10s)
myapp: stopped accepting connections; in-flight requests finished (120ms)
myapp: job worker stopped (30ms)
myapp: shutdown hook "cache" done (2ms)
myapp: shutdown hook "db" done (5ms)
myapp: stopped cleanly in 160ms
```

Readiness flips to 503 `{"status": "draining"}` so load balancers stop
routing new traffic, the listeners stop accepting connections, in-flight
requests and the in-process job worker get the rest of the 10-second
budget, then the shutdown hooks run. The process exits 0 when every phase
finished in time, and 1 when a phase timed out or a hook failed (the last
line then reads `stopped with N problem(s)`). A second Ctrl+C or SIGTERM
during shutdown exits immediately with code 130. Pair with
`gebweb.useOps(app)` so the orchestrator actually polls `/readyz`.

### Shutdown hooks

Register cleanup with `gebweb.onShutdown(app, name, hook, opts)` during
setup:

```gb
gebweb.onShutdown(app, "db", func(): void {
    conn.close();
});
gebweb.onShutdown(app, "cache", func(): void {
    cache.flush();
}, {"timeoutMs": 2000});
```

Each hook runs once, after the listeners and the job worker have stopped,
latest registered first (like `defer`), so something registered later can
still use what was registered earlier. `timeoutMs` caps one hook; without it
a hook may use whatever is left of the overall budget. A hook that throws or
runs out of time is logged and the remaining hooks still run. Registering a
hook after the app starts serving throws a `RuntimeError`.

`gebweb.shutdown(app, opts)` returns `true` for a clean shutdown and `false`
otherwise, and `opts.reason` prefixes the first log line. An app that serves
with `gebweb.serve` and calls `gebweb.shutdown` from its own signal handler
gets the same phases: `serve` returns only after the hooks have finished.

The `app.stopping` and `app.stopped` events still fire (see
[Events](19-events.md#framework-lifecycle-events)); use them to observe
shutdown and `onShutdown` to release resources.

Long-running loops the entrypoint does not own (job workers,
scheduler ticks) should poll `gebweb.isDraining(app)` and exit when
it flips. The in-process job worker
(`useJobs(app, conn, {"inProcessWorker": true})`) does this itself, and
shutdown waits up to the same deadline for its in-flight jobs.

## Logging

Framework messages go through one framework logger built on the stdlib
`log` module: the serving banner, shutdown phases, discovery summary,
failing listeners and finalizers, development errors, failed scheduled
tasks and schedule updates, job-worker and broker warnings (reconnects,
drain deadlines), and OIDC sign-in failures. Command-line usage errors and
`--help` output stay plain console output. By default it writes
readable lines prefixed with the app name: information to stdout, warnings
and errors to stderr. Set `GEBWEB_LOG_FORMAT=json` for one JSON object per
line on stdout, with structured fields such as `durationMs`, `status` and
`hook`.

Supply your own destination with `gebweb.useLogger(app, handle)`, for
example `log.file("/var/log/myapp.log")`, and log your own messages through
the same handle with `gebweb.logger(app)`:

```gb
import log;

log.info(gebweb.logger(app), "cache warmed", {"entries": 120});
```

In the dev environment every request is logged as one line
(`GET /docs/ 200 (4ms)`). An app that installs the `requestLog`
middleware keeps its own access log instead, without the extra line.

## Operational notes

- Build a deployable binary with `gebweb build` (or `geblang build`);
  the app's `main(args)` receives every argument, including `--help`
  and `--version`; the runtime answers `--geblang-version` and
  `--geblang-notices` as the first argument.
- Memory: a VM-mode server under heavy sustained load keeps a pool of
  per-route execution state; on memory-constrained containers set
  `GOMEMLIMIT` (for example `GOMEMLIMIT=512MiB`) to bound the
  process.
- Throughput: request handling allocates many short-lived values, so
  the garbage collector setting matters. On a small JSON route at 64
  concurrent clients, `GOGC=400` served about 20% more requests per
  second than the default `GOGC=100`, with lower p99 latency, but peak
  memory roughly doubled (about 200 MB to 450 MB). Start with
  `GOGC=200` plus a `GOMEMLIMIT` below your container limit, then
  measure (`GOGC=200 GOMEMLIMIT=1GiB ./myapp`).
- Request bodies are capped at 10 MB by default
  (`gebweb.useMaxBodyBytes` / `@MaxBody` adjust; see the security
  chapter).

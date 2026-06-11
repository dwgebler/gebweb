# Deployment: the standard server entrypoint

`gebweb.cli(app, opts)` is the standard way to serve an app in
production. It gives every Gebweb binary the same operational
surface: port and bind-address flags with environment fallbacks,
HTTPS via LetsEncrypt or a generated local certificate, a startup
banner, `--help`, and a graceful drain on SIGINT / SIGTERM.

```gb
import gebweb;

let app = gebweb.app([HomeController()]);
gebweb.useOps(app);
gebweb.cli(app, {"name": "myapp", "port": 8080});
```

```text
./myapp                       # http://localhost:8080
./myapp --port 9000           # flag beats env beats opts
./myapp --self-signed         # https://localhost:443, generated cert
./myapp --domain example.com  # LetsEncrypt on :443, HTTP redirect on :80
./myapp --help                # prints the option table
```

## Options and precedence

Resolution order is flags > environment > `opts` defaults.

| Flag | Environment | `opts` key | Default | Meaning |
|------|-------------|-----------|---------|---------|
| `--port N` | `GEBWEB_HTTP_PORT` | `port` | `8080` | plain-HTTP port |
| `--tls-port N` | `GEBWEB_TLS_PORT` | `tlsPort` | `443` | TLS port |
| `--host ADDR` | `GEBWEB_HOST` | `host` | all interfaces | bind address |
| `--domain HOST` | `GEBWEB_DOMAIN` | `domain` | - | LetsEncrypt autocert host |
| `--self-signed` | `GEBWEB_SELF_SIGNED` | `selfSigned` | off | local HTTPS, generated cert |
| `--no-tls` | `GEBWEB_NO_TLS` | `noTls` | off | force plain HTTP |
| `--acme-email A` | `GEBWEB_ACME_EMAIL` | `acmeEmail` | - | ACME contact address |
| `--acme-cache DIR` | `GEBWEB_ACME_CACHE` | `acmeCache` | - | certificate cache dir |
| `--http MODE` | `GEBWEB_HTTP` | `http` | see below | plain-HTTP port behaviour when TLS is active |
| `--help` | - | - | - | print the option table |

`opts.name` sets the binary's display name in the banner and help
text.

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
`localhost`. Browsers warn on first visit (the certificate is not
chain-trusted); accept it or pin it for the session. Useful when a
feature needs a secure context (cookies with `Secure`, service
workers, HTTP/2).

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

On SIGINT or SIGTERM the entrypoint prints a drain message and calls
`gebweb.shutdown(app, {"timeoutMs": 10000})`: readiness flips to 503
`{"status": "draining"}` so load balancers stop routing new traffic,
in-flight requests get up to the deadline to finish, then the
listener closes and the process exits 0. Pair with
`gebweb.useOps(app)` so the orchestrator actually polls `/readyz`.

Long-running loops the entrypoint does not own (job workers,
scheduler ticks) should poll `gebweb.isDraining(app)` and exit when
it flips.

## Operational notes

- Build a deployable binary with `gebweb build` (or `geblang build`);
  built binaries answer `--help`, `--version`, and `--notices`
  natively and pass everything after `--` through to the app.
- Memory: a VM-mode server under heavy sustained load keeps a pool of
  per-route execution state; on memory-constrained containers set
  `GOMEMLIMIT` (for example `GOMEMLIMIT=512MiB`) to bound the
  process.
- Request bodies are capped at 10 MB by default
  (`gebweb.useMaxBodyBytes` / `@MaxBody` adjust; see the security
  chapter).

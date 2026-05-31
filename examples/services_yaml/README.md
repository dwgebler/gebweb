# services.yaml end-to-end demo

A small two-route app that wires parameters, services entries with
arg overrides, an interface binding, per-env overlays, and the
encrypted-file secrets provider.

## What it shows

- `config/services.yaml` carries the prod layout: real
  `HttpFeedClient` with an `apiKey` resolved from
  `%secret(feed.api_key)%`, a parameter chain that builds the
  endpoint from `%feed.host%`, and a `bindings:` entry choosing
  `HttpFeedClient` for `FeedClient`.
- `config/services_dev.yaml` rebinds `FeedClient -> StubFeedClient`
  and overrides `feed.host`. Selected when `GEBWEB_ENV=dev`.
- `main.gb` only attaches the secrets provider when the env is
  not `dev`, so the stub path needs no vault.

## Run with the stub (no secrets)

```sh
cd examples/services_yaml
GEBWEB_ENV=dev geblang main.gb
curl http://127.0.0.1:8080/feed
# {"items":["stub:item-a","stub:item-b","stub:item-c"]}
```

## Run against the live binding (with the encrypted vault)

```sh
cd examples/services_yaml
../../../gebweb secrets init
../../../gebweb secrets set feed.api_key sk_live_demo
GEBWEB_SECRETS_KEY=$(cat config/secrets.key) geblang main.gb
curl http://127.0.0.1:8080/feed
# {"items":["live:https://demo.example.com/v1/items?key=sk_live_demo"]}
```

## Tests

```sh
cd examples/services_yaml
geblang test tests/
```

The test suite exercises the dev overlay path so the assertions
don't depend on a populated vault.

## Files

- `config/services.yaml` - base config: parameters, services,
  bindings.
- `config/services_dev.yaml` - dev overlay (stub binding).
- `src/feed.gb` - `FeedClient` interface + two implementations.
- `src/consumer.gb` - the autowired consumer that the YAML
  registers as `app.feed`.
- `src/controllers.gb` - HTTP controller exposing `/feed`.
- `main.gb` - entry point.

See chapter [services.yaml](../../docs/29-services-yaml.md) in the
manual for the full reference.

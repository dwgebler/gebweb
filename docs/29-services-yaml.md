# services.yaml: parameters, services, bindings, secrets

Gebweb apps can move runtime configuration out of code into a single
`config/services.yaml` file. The YAML loader populates the parameter
store, registers services with constructor arg overrides, binds
interfaces to chosen implementations, and resolves secrets through
a pluggable provider. The same `gebweb.app(controllers)` builder
auto-loads `config/services.yaml` when it's present, so the only
change to make is creating the file.

This chapter walks the surface in order: parameters, services
entries, interface bindings, tags, per-environment overlays, the
secrets vault, and the CLI tooling that drives it.

## The shape

```yaml
# config/services.yaml

imports:
  - { resource: services_local.yaml, optional: true }

parameters:
  app.name: "summariser"
  aws.region: "us-east-1"
  aws.access_key: "%env(AWS_ACCESS_KEY)%"
  aws.secret_key: "%secret(aws.secret_key)%"
  bedrock.endpoint: "https://bedrock-runtime.%aws.region%.amazonaws.com"

services:
  AnthropicClient:
    args:
      apiKey: "%secret(anthropic.key)%"
      model:  "claude-opus-4-8"

  app.summariser:
    class: Summariser
    args:
      client: "@AnthropicClient"
    tags:    ["app.background"]
    shared:  true

  summariser.alias: "@app.summariser"

bindings:
  "llm.Client": "@AnthropicClient"
```

Five sections, each independently optional:

- `imports:` pull other YAML files in first; the importing file's
  values layer on top of any conflicting keys from imports.
- `parameters:` typed primitive values, queried via
  `gebweb.parameter(app, key)`.
- `services:` register classes by id with constructor arg
  overrides, tags, and scope.
- `bindings:` map interface names to a service id so autowire
  picks the right implementation.
- Anything else is ignored; future versions may add new top-level
  sections.

## Parameters

The `parameters:` section is a flat map of `key: value`. Keys are
treated as strings; the dot in `aws.region` is part of the name,
not a nested path.

```yaml
parameters:
  app.name: "summariser"
  app.port: 8080
  app.features: ["search", "billing"]
```

Read a value back with `gebweb.parameter(app, key)` (returns `any`)
or pull one into a constructor via `@Param`:

```gb
class StartupBanner {
    string name;
    int port;
    func StartupBanner(@Param("app.name") string name,
                       @Param("app.port") int port) {
        this.name = name;
        this.port = port;
    }
}
```

### Marker substitution

String values may embed markers that resolve on read:

| Marker | Resolves to |
|--------|-------------|
| `%env(NAME)%` | the `NAME` process environment variable (throws when unset) |
| `%secret(name)%` | the value from the registered `SecretsProvider` (throws when no provider is registered) |
| `%other.key%` | the value of another parameter (cycles throw) |
| `%%` | a literal `%` |

A string that is exactly a single marker (`"%aws.region%"`) keeps
the referenced value's native type; embedded markers (`"https://%aws.region%/.amazonaws.com"`) compose textually.

```yaml
parameters:
  port: 8080
  exposed_port: "%port%"           # 8080 as int
  url: "https://api:%port%/v1"     # "https://api:8080/v1" string
```

## Services entries

Every entry under `services:` is either a dict (a service entry)
or a string of the form `"@target"` (an alias).

```yaml
services:
  YamlDb:
    args:
      url: "sqlite::memory:"
  UserRepo:
    args:
      db:        "@YamlDb"
      tableName: "users"
  primary.db: "@YamlDb"            # alias short form
```

| Field | Default | Purpose |
|-------|---------|---------|
| `class:` | the id | The class to construct. Lets you give a class multiple ids (with different args). |
| `args:` | empty | Per-parameter overrides. Values support `%marker%` interpolation and `@service-ref` for service references. |
| `tags:` | empty | Tags for collection via `gebweb.taggedServices(app, name)`. |
| `shared:` | `true` | `true` is singleton (same instance every resolve); `false` is transient (fresh instance per resolve). |
| `aliases:` | empty | Extra ids that resolve to this service. |

Services without `args:` autowire their constructor parameters the
same way `gebweb.resolve(app, X)` does. The entry is then equivalent
to applying the `@Service("id")` decorator on the class. Use
`args:` only when a parameter needs an explicit value or service
reference.

### Args values

```yaml
services:
  Foo:
    args:
      literal:    "string"
      number:     8080
      dict:       { a: 1, b: 2 }
      from_env:   "%env(API_KEY)%"
      from_param: "%aws.region%"
      injected:   "@Bar"
```

For each constructor parameter, the loader looks up its name in
`args:`; if absent, it autowires the parameter from its declared
type. Mix and match freely.

## Bindings

When a constructor parameter is typed with an interface and the
service registry has more than one implementation, you must bind
the interface to a specific id:

```yaml
services:
  EmailNotifier: {}
  SlackNotifier: {}
  AlertService: {}

bindings:
  Notifier: "@EmailNotifier"
```

With exactly one implementation registered, the binding is
auto-applied at first resolve (no entry needed). With zero or
multiple impls and no binding, the loader throws a clear error
listing the candidates:

```
DI: interface 'Notifier' has 2 implementations (EmailNotifier,
SlackNotifier) but no binding; bind it via services.yaml
`bindings:` or di.bindInterface
```

Inherited interfaces count: a service whose parent class declares
the interface is treated as an implementer.

### Programmatic bindings

The same registry is exposed via `di.bindInterface` for code that
needs to skip YAML:

```gb
di.bindInterface(app.container, "Notifier", "EmailNotifier");
```

Order of precedence at resolve time:

1. `interfaceInstances` (set by `useLlm` and friends)
2. `interfaceBindings` (YAML + explicit `bindInterface` calls)
3. Auto-bind on single-impl
4. Throw

## Tags and `taggedServices`

```yaml
services:
  EmailNotifier:
    tags: ["app.notifier"]
  SlackNotifier:
    tags: ["app.notifier"]
  WelcomeMailer:
    tags: ["app.background", "app.notifier"]
```

```gb
let notifiers = gebweb.taggedServices(app, "app.notifier");
let ids       = gebweb.taggedServiceIds(app, "app.notifier");
let tags      = gebweb.tagsForService(app, "WelcomeMailer");
```

`taggedServices` returns instances in registration order. Each
instance is built through `gebweb.service(app, id)` so the per-
service shared / transient policy still applies (a transient
service yields fresh instances on every call).

`@Tag("name")` on an `@Service`-decorated class adds the class
to the tag registry the same way a YAML `tags:` entry would.
Framework-reserved tags (`event.subscriber`, `job.handler`,
`message.handler`, `scheduled`) are handled by their dedicated
registrars and do not leak into `taggedServices`.

## Per-environment overlays

The active environment comes from `GEBWEB_ENV` (defaults to `prod`).
After loading the base file, the loader looks for a sibling overlay
named `<stem>_<env>.<ext>` and merges it on top.

```
config/services.yaml          # always loaded
config/services_dev.yaml      # loaded when GEBWEB_ENV=dev
config/services_prod.yaml     # loaded when GEBWEB_ENV=prod
```

```yaml
# config/services_dev.yaml
parameters:
  log_level: "debug"
services:
  EmailNotifier:
    class: LogNotifier
```

Overlay maps merge recursively; overlay scalars replace base
scalars.

`gebweb.currentEnv()` exposes the active name for code that needs
to branch on it.

## Imports

```yaml
imports:
  - shared.yaml
  - { resource: optional_local.yaml, optional: true }
```

Imports are resolved relative to the file that declares them.
Cycles throw at load time. A required file that's missing throws;
`optional: true` silently skips a missing file. The importing
file's sections layer on top of its imports.

## Secrets

`%secret(name)%` markers resolve through a `SecretsProvider` you
register with `gebweb.useSecrets(app, provider)`. Gebweb ships a
built-in encrypted-file provider:

```gb
gebweb.useSecrets(app, gebweb.encryptedFileSecrets());
```

The factory reads `config/secrets.enc` for the vault body and the
AES-256 key from `GEBWEB_SECRETS_KEY` (base64) or
`config/secrets.key`. Custom providers implement the interface:

```gb
class VaultClient implements gebweb.SecretsProvider {
    func getSecret(string name): string { /* ... */ }
    func hasSecret(string name): bool   { /* ... */ }
}
```

The wire format used by the built-in provider:

- AES-256-GCM over a JSON dict of name -> value
- `nonce (12 bytes) || ciphertext`, base64-encoded, chunked at 80
  columns
- Wrapped between `-----BEGIN GEBWEB SECRETS-----` and
  `-----END GEBWEB SECRETS-----`

### `gebweb secrets` CLI

The CLI manages the vault without leaving the shell:

```sh
gebweb secrets init                    # generate key + empty vault
gebweb secrets set stripe.key sk_test_abc
gebweb secrets get stripe.key
gebweb secrets list
gebweb secrets edit                    # decrypt to $EDITOR temp file
gebweb secrets --key-file path init    # override key file
gebweb secrets --file path init        # override vault file
```

`init` writes `config/secrets.key` at mode `0600` and seeds an
empty `config/secrets.enc`. The instruction it prints, to add the
key file to `.gitignore` and supply `GEBWEB_SECRETS_KEY` in
production, is the recommended deployment flow.

`edit` opens `$VISUAL` (then `$EDITOR`) on a JSON pretty-print of
the current vault in a temp file with mode `0600`, re-encrypts on
save, and removes the temp file. JSON parse errors abort the
update.

## Reference

| API | Purpose |
|-----|---------|
| `gebweb.loadConfig(app, path = "config/services.yaml")` | Explicitly load YAML config into the app. `gebweb.app()` auto-loads `config/services.yaml` when the file exists. |
| `gebweb.currentEnv()` | Active env name from `GEBWEB_ENV` (defaults to `prod`). |
| `gebweb.parameter(app, key, value)` | Set a parameter. |
| `gebweb.parameter(app, key)` | Get a parameter (substitutes markers on read). |
| `gebweb.service(app, id)` | Resolve a service by id. |
| `gebweb.hasService(app, id)` | True when an id is registered. |
| `gebweb.serviceIds(app)` | All registered service ids (aliases not included). |
| `gebweb.taggedServices(app, tag)` | Instances of every service carrying `tag`. |
| `gebweb.taggedServiceIds(app, tag)` | Ids of every service carrying `tag`. |
| `gebweb.tagsForService(app, id)` | Tags attached to one service. |
| `gebweb.interfaceBindings(app)` | All registered interface -> service-id bindings. |
| `gebweb.useSecrets(app, provider)` | Register a `SecretsProvider`. |
| `gebweb.encryptedFileSecrets(path = "config/secrets.enc")` | Built-in provider; key from `GEBWEB_SECRETS_KEY` env var or `config/secrets.key`. |
| `gebweb.encryptSecrets(dict, key)` | Encode a dict to the wire format (used by tooling). |
| `gebweb secrets <init|edit|set|get|list>` | CLI for managing the encrypted vault. |

### Errors

| Symptom | Likely cause |
|---------|--------------|
| `gebweb config: env var 'X' is not set` | A `%env(X)%` marker was read but `X` is unset / empty. |
| `gebweb config: %secret(name)% requires a secrets provider` | A `%secret(...)%` marker was read but `gebweb.useSecrets` has not been called. |
| `gebweb config: cyclic parameter reference at 'k'` | A parameter substitution cycle. |
| `gebweb config: services entry 'id' references unknown class 'X'` | The `class:` field (or the id when no `class:` is given) does not resolve to a known class. |
| `DI: service id 'x' is already registered to a different class` | Two YAML entries (or YAML + `@Service`) bind the same id to different classes. |
| `DI: alias 'a' shadows an existing service id` | An alias name collides with a real service id. |
| `DI: interface 'I' has N implementations (...) but no binding` | More than one impl registered, no `bindings:` entry chose one. |
| `gebweb secrets: AES-256 key must be 32 bytes` | Wrong key length supplied to the provider or `encryptSecrets`. |
| `gebweb secrets: authentication failed` | Ciphertext was tampered with or the key doesn't match the file. |

# services.yaml: parameters, services, bindings, secrets

<!-- doctest:strict -->

`gebweb.app()` loads `config/services.yaml` from the working directory when it
exists. The file can populate parameters, register services, bind interfaces
and import other configuration files. Classes must already be loaded through
the import graph; YAML names do not import source modules or scan directories.

## The shape

Each section is optional:

| Section | Purpose |
|---------|---------|
| `imports` | Load other YAML files first, relative to the declaring file. |
| `parameters` | Flat string keys mapped to values, including lists and dictionaries. |
| `services` | Service IDs mapped to class/configuration entries or aliases. |
| `bindings` | Interface names mapped to registered service IDs. |

Unknown top-level sections are ignored. Check spelling: they do not produce a
configuration validation error.

Start with `config/services.yaml`:

<!-- doctest:file config/services.yaml -->
```yaml
parameters:
  app.name: catalog
  app.port: 8080
  copied.port: "%app.port%"
  app.url: "http://localhost:%app.port%"
  progress: "50%%"
```

Read the automatically loaded values:

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app([], {"docs": false});
io.println(gebweb.getParameter(app, "app.name"));
io.println(gebweb.getParameter(app, "copied.port") instanceof int);
io.println(gebweb.getParameter(app, "app.url"));
io.println(gebweb.getParameter(app, "progress"));
```

```text
catalog
true
http://localhost:8080
50%
```

The dot is part of a key, not a nested path. Use
`gebweb.loadConfig(app, path)` for an additional or differently named file.
Load configuration during setup before resolving consumers, not on each
request. Reloading a file does not rebuild existing singleton services, and
registering an already-bound ID to a different class is an error.

## Parameters

Set a value in code with `gebweb.parameter(app, key, value)` and read it with
`gebweb.getParameter(app, key)`. The getter returns `any`, resolves markers
recursively through list/dictionary values and throws for an unknown key.
Marker-free containers can be shared by reference; treat configuration as
read-only after setup.

### Marker substitution

| Marker | Resolution |
|--------|------------|
| `%env(NAME)%` | Nonempty process environment string; unset or empty throws. |
| `%secret(name)%` | String returned by the registered secrets provider; missing provider throws. |
| `%other.key%` | Another parameter, recursively; unknown keys and cycles throw. |
| `%%` | Literal percent sign. |

An entire single marker preserves the referenced type, as `copied.port` does
above. Embedded markers compose text. Environment values are strings: a
parameter containing `%env(PORT)%` does not automatically become an integer.

Constructor `@Param` uses the same resolver:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class StartupBanner {
    string name;
    int port;

    func StartupBanner(@Param("app.name") string name, @Param("app.port") int port) {
        this.name = name;
        this.port = port;
    }
}

let app = gebweb.app([], {"docs": false});
let banner = gebweb.resolve(app, StartupBanner) as StartupBanner;
io.println(banner.name + ":" + banner.port.toString());
```

```text
catalog:8080
```

### Environment files

`%env(NAME)%` reads the process, not a `.env` file. At application startup,
load a local file explicitly before building the app:

<!-- doctest:fragment context=script -->
```gb
import dotenv;

if (io.exists(".env")) {
    dotenv.loadAndApply(".env");
}
```

`loadAndApply` overwrites existing environment variables. Keep this a local
convenience; do not ship `.env` in a release or commit credentials. Follow the
[standard entry point](01-getting-started.md), including `cli.checkArgs` before
building the app so `--dev` takes effect before configuration is loaded.

## Services entries

For the following examples, put these classes in `src/catalog_services.gb`.
The delivery classes return labels rather than sending messages:

<!-- doctest:file catalog_services.gb -->
```gb
export interface Delivery {
    func label(): string;
}

export class EmailDelivery implements Delivery {
    string name;

    func EmailDelivery(string name) {
        this.name = name;
    }

    func label(): string {
        return "email:" + this.name;
    }
}

export class WebhookDelivery implements Delivery {
    func label(): string {
        return "webhook";
    }
}

export class AlertService {
    Delivery delivery;

    func AlertService(Delivery delivery) {
        this.delivery = delivery;
    }

    func label(): string {
        return this.delivery.label();
    }
}

export class Scratch {
    int count = 0;
}
```

Create `config/catalog.yaml`. It is explicitly loaded in these examples; a
real app can place its service entries in `config/services.yaml` instead:

<!-- doctest:file config/catalog.yaml -->
```yaml
services:
  delivery.email:
    class: EmailDelivery
    args:
      name: "%app.name%"
    tags: [app.delivery]
  delivery.webhook:
    class: WebhookDelivery
    tags: [app.delivery]
  app.alert:
    class: AlertService
    aliases: [alert]
  app.scratch:
    class: Scratch
    shared: false
  delivery.primary: "@delivery.email"
bindings:
  Delivery: "@delivery.email"
```

Import the classes before loading the file. Resolve by service ID when using
its YAML arguments or scope:

<!-- doctest:run -->
```gb
import catalog_services;
import gebweb;
import io;

let app = gebweb.app([], {"docs": false});
gebweb.loadConfig(app, "config/catalog.yaml");
let alert = gebweb.service(app, "alert") as catalog_services.AlertService;
io.println(alert.label());
io.println(gebweb.hasService(app, "delivery.primary"));
io.println((gebweb.service(app, "delivery.primary") as catalog_services.EmailDelivery).label());
let first = gebweb.service(app, "app.scratch") as catalog_services.Scratch;
first.count = 7;
io.println((gebweb.service(app, "app.scratch") as catalog_services.Scratch).count);
```

```text
email:catalog
true
email:catalog
0
```

| Entry field | Default | Purpose |
|-------------|---------|---------|
| `class` | The service ID | Name of an already-loaded class. |
| `args` | Omitted | Named constructor argument overrides; missing arguments use normal DI resolution. |
| `tags` | `[]` | Names for tagged service collection. |
| `shared` | `true` | Singleton when true; a fresh instance per resolution when false. |
| `aliases` | `[]` | Additional IDs resolving to this service. |

An entry can instead be `alias: "@target"`. Aliases redirect lookup; they are
not additional services in `serviceIds`. An alias cannot shadow a real service
ID, and alias cycles fail on resolution.

### Args values

A top-level argument string beginning with `@` resolves a service ID. Other
values use parameter-marker resolution, including markers within collections:

<!-- doctest:file config/alternate.yaml -->
```yaml
services:
  app.directAlert:
    class: AlertService
    args:
      delivery: "@delivery.webhook"
```

<!-- doctest:run -->
```gb
import catalog_services;
import gebweb;
import io;

let app = gebweb.app([], {"docs": false});
gebweb.loadConfig(app, "config/catalog.yaml");
gebweb.loadConfig(app, "config/alternate.yaml");
io.println((gebweb.service(app, "app.directAlert") as catalog_services.AlertService).label());
```

```text
webhook
```

Argument keys must match constructor parameter names. An `@service` reference
nested inside a list or dictionary is not recursively resolved as a service;
that recursion applies to percent markers. Primitive parameters without an
override normally need `@Param` or a default; DI does not invent their values.

## Bindings

The `Delivery` binding selects `delivery.email` for the interface-typed
constructor parameter above. A binding may use `"@id"` or `"id"`; its target
must exist and implement the named interface. Inherited class interfaces
count. Use names matching the loaded declaration/constructor metadata, not an
unrelated interface with the same short name.

With one registered implementation, DI can select it at first resolution.
With several and no binding, configuration/discovery conflict detection throws;
choose the implementation before resolving consumers. Resolution prefers an
explicit interface instance (for example from `useLlm`), then an explicit
binding, then the sole implementation.

### Programmatic bindings

The equivalent code, after registering the services, is:

<!-- doctest:fragment context=app -->
```gb
import gebweb.di as di;

di.bindInterface(app.container, "Delivery", "delivery.email");
```

`bindInterface` is idempotent for the same target; attempting to rebind to a
different ID throws. See [Dependency injection](06a-dependency-injection.md)
for registration and constructor rules.

## Tags and `taggedServices`

Tags collect related services without choosing an interface binding:

<!-- doctest:run -->
```gb
import catalog_services;
import gebweb;
import io;

let app = gebweb.app([], {"docs": false});
gebweb.loadConfig(app, "config/catalog.yaml");
io.println(gebweb.taggedServiceIds(app, "app.delivery").join(","));
for (service in gebweb.taggedServices(app, "app.delivery")) {
    io.println((service as catalog_services.Delivery).label());
}
io.println(gebweb.tagsForService(app, "delivery.email").join(","));
```

```text
delivery.email,delivery.webhook
email:catalog
webhook
app.delivery
```

Collection preserves registration order and resolves each service normally,
including transient scope. `@Tag` on an `@Service` class also registers tags.
Framework-reserved tags such as `event.subscriber`, `job.handler`,
`message.handler` and `scheduled` belong to their dedicated registrars.

A transient service is fresh on each resolution, not automatically once per
HTTP request. A singleton holding a transient dependency still holds that
particular instance. Controllers and shared services can serve concurrent
requests; use request scope or concurrency-safe state rather than the mutable
counter used only for this single-threaded demonstration.

## Per-environment overlays

`currentEnv()` normalizes the environment for framework behavior, but overlay
filenames use the trimmed, case-preserved raw `GEBWEB_ENV` value for backwards
compatibility. Unset or blank uses `prod`:

| `GEBWEB_ENV` | `currentEnv()` | Overlay for `services.yaml` |
|--------------|----------------|-------------------------------|
| Unset / blank | `prod` | `services_prod.yaml` |
| `dev` | `dev` | `services_dev.yaml` |
| `development` | `dev` | `services_development.yaml` |
| `production` | `prod` | `services_production.yaml` |
| `DEV` | `dev` | `services_DEV.yaml` |

Use consistent short lowercase names in deployments. This example uses an
isolated base file so it does not change the earlier service registrations:

<!-- doctest:file config/environment.yaml -->
```yaml
parameters:
  label: base
  options: {retries: 2, verbose: false}
```

<!-- doctest:file config/environment_dev.yaml -->
```yaml
parameters:
  label: development
  options: {verbose: true}
```

<!-- doctest:run -->
```gb
import gebweb;
import io;
import sys;

sys.setenv("GEBWEB_ENV", "dev");
let app = gebweb.app([], {"docs": false});
gebweb.loadConfig(app, "config/environment.yaml");
io.println(gebweb.currentEnv());
io.println(gebweb.getParameter(app, "label"));
let options = gebweb.getParameter(app, "options") as dict<string, any>;
io.println(options["retries"]);
io.println(options["verbose"]);
```

```text
dev
development
2
true
```

Overlay dictionaries merge recursively. Other values, including lists, replace
the base value. Loading occurs at setup; changing `GEBWEB_ENV` does not reload
existing configuration or reconstruct services.

## Imports

Imports are processed in order, followed by the importing file's own values,
then its sibling environment overlay. Later values take precedence when merged.
A required missing import or an import cycle throws; `optional` only permits
a missing file, not malformed contents.

<!-- doctest:file config/shared.yaml -->
```yaml
parameters:
  import.label: shared
  import.retained: true
```

<!-- doctest:file config/imports.yaml -->
```yaml
imports:
  - shared.yaml
  - {resource: optional_local.yaml, optional: true}
parameters:
  import.label: local
```

<!-- doctest:run -->
```gb
import gebweb;
import io;

let app = gebweb.app([], {"docs": false});
gebweb.loadConfig(app, "config/imports.yaml");
io.println(gebweb.getParameter(app, "import.label"));
io.println(gebweb.getParameter(app, "import.retained"));
```

```text
local
true
```

## Secrets

Register a `gebweb.SecretsProvider` with `useSecrets` before reading a secret
marker or constructing a service that needs one. Merely loading a parameter
containing `%secret(name)%` does not read the secret.

The built-in provider uses AES-256-GCM and caches the decrypted vault on first
read. Here is a complete round trip using a temporary vault and a fresh key;
no actual secret or key is printed:

<!-- doctest:run -->
```gb
import gebweb;
import gebweb.secretstore as secretstore;
import io;
import path;
import secrets;

let directory = io.tempDir("manual-vault-");
try {
    let key = secrets.randomBytes(32);
    string file = path.join(directory, "secrets.enc");
    io.writeText(file, gebweb.encryptSecrets({"mail.token": "example-only-value"}, key));
    let provider = secretstore.EncryptedFileSecretsProvider(file, key);
    let app = gebweb.app([], {"docs": false});
    gebweb.parameter(app, "mail.token", "%secret(mail.token)%");
    gebweb.useSecrets(app, provider);
    io.println(provider.hasSecret("mail.token"));
    io.println(gebweb.getParameter(app, "mail.token") == "example-only-value");
    io.println(provider.hasSecret("missing"));
} finally {
    io.remove(directory);
}
```

```text
true
true
false
```

The convenience `gebweb.encryptedFileSecrets(path = "config/secrets.enc")`
loads a base64 key from nonempty `GEBWEB_SECRETS_KEY`, otherwise from
`config/secrets.key`. The key must decode to 32 bytes. The custom vault path
does not change the default key path. The factory reads the key immediately;
the encrypted file is decrypted on first access. There is no automatic vault
refresh; recreate the provider/application when rotating it.

Custom providers implement `getSecret(string name): string` and
`hasSecret(string name): bool`. Throw when a requested secret is missing. Keep
provider access bounded and safe for shared use across requests.

### `gebweb secrets` CLI

Run these commands from the project root. Flags follow the subcommand:

```sh
gebweb secrets init
gebweb secrets set mail.token example-only-value
gebweb secrets list
gebweb secrets get mail.token
gebweb secrets edit
gebweb secrets init --key-file private/vault.key --file config/vault.enc
```

| Command / option | Behavior |
|------------------|----------|
| `init` | Generate a key and empty vault; refuse overwrites unless `--force`. |
| `set name value` | Add or replace a value and re-encrypt. |
| `get name` | Print the plaintext value; avoid logs/history captures. |
| `list` | Print sorted secret names. |
| `edit` | Open JSON in `$VISUAL`, then `$EDITOR`; re-encrypt valid saved JSON. |
| `--key-file PATH` | Alternate key file; a nonempty `GEBWEB_SECRETS_KEY` still takes precedence. |
| `--file PATH` | Alternate encrypted vault file. |

`init` creates files with mode `0600`. `edit` uses a temporary plaintext file
with mode `0600`, removed when the command finishes. `set` is convenient for
examples; its plaintext argument can enter shell history/process listings.
Use `edit` or an appropriate deployment secret mechanism for real credentials.
Keep the key out of source control and deployment bundles. Store the encrypted
vault separately from its key and arrange production key injection explicitly.

The runtime/tooling wire format is base64-encoded `12-byte nonce || ciphertext`
(including the authentication tag), wrapped at 80 columns between
`-----BEGIN GEBWEB SECRETS-----` and `-----END GEBWEB SECRETS-----`.

## Typed configuration binding

`bindConfig` builds and validates a configuration object at startup. Nonempty
environment values win over same-named parameter fields. Absent optional fields
retain their class initializer, normally null. Required fields must resolve even
if a class declares a default.

<!-- doctest:run -->
```gb
import gebweb;
import io;
import sys;

class CatalogConfig {
    @Assert.range(min: 1)
    int poolSize;
    bool debugMode;
    list<string> features;
    ?string sentryDsn;
}

let app = gebweb.app([], {"docs": false});
gebweb.parameter(app, "poolSize", 4);
sys.setenv("MANUALCFG_POOL_SIZE", "12");
sys.setenv("MANUALCFG_DEBUG_MODE", "yes");
sys.setenv("MANUALCFG_FEATURES", '["search","billing"]');
let config = gebweb.bindConfig(app, CatalogConfig, {"prefix": "MANUALCFG"}) as CatalogConfig;
io.println(config.poolSize);
io.println(config.debugMode);
io.println(config.features.join(","));
io.println(config.sentryDsn == null);
```

```text
12
true
search,billing
true
```

| Binding rule | Behavior |
|--------------|----------|
| `prefix` option | Defaults to `APP`; `poolSize` becomes `APP_POOL_SIZE`. |
| Resolution | Nonempty environment string, then parameter under exact field name, then absent. |
| Environment coercion | Integers, decimal/float, booleans (`true/1/yes`, `false/0/no`, case-insensitive). Lists/dictionaries parse JSON. |
| Parameter values | Passed directly; supply the declared type. This path does not run percent-marker substitution. |
| Validation | `@Assert` failures throw during `bindConfig`. |

Call binding once during setup and pass the resulting configuration to services.
It does not register the object in DI automatically. Resolve any marker-bearing
parameter explicitly with `getParameter` before using it as a typed field.

## Errors

| Symptom | Check |
|---------|-------|
| Unknown class during YAML loading | Import its module before building/loading configuration. |
| Missing env/secret/parameter marker | Supply the value/provider before reading or resolving consumers. |
| Cyclic parameter or import error | Remove the configuration dependency cycle. |
| ID registered to a different class | Use distinct IDs; loading another file is not service replacement. |
| Multiple interface implementations | Add the explicit `bindings` entry. |
| Missing typed configuration field | Supply the named environment variable or correctly typed parameter. |
| Invalid key / authentication failure | Check the decoded key length, vault/key pairing and ciphertext integrity. |

## Reference

| API | Purpose |
|-----|---------|
| `loadConfig(app, path = "config/services.yaml")` | Load parameters/services/bindings and imports/overlay; returns the app. |
| `currentEnv()` | Normalized framework environment name. |
| `parameter(app, key, value)` / `getParameter(app, key)` | Store a value / read with marker resolution. |
| `service(app, id)` / `hasService(app, id)` | Resolve a service / check registration. |
| `serviceIds(app)` | Registered service IDs, excluding aliases. |
| `taggedServices(app, tag)` / `taggedServiceIds(app, tag)` | Tagged instances / IDs in registration order. |
| `tagsForService(app, id)` | Tags for one service. |
| `interfaceBindings(app)` | Interface-name to service-ID bindings. |
| `useSecrets(app, provider)` | Attach a secrets provider. |
| `encryptedFileSecrets(path)` / `encryptSecrets(values, key)` | Read a vault provider / create encrypted vault text. |
| `bindConfig(app, ConfigClass, opts = {})` | Hydrate and validate a typed configuration object. |

These APIs are on `gebweb`. See [Dependency injection](06a-dependency-injection.md),
[CLI](15-cli.md), [Deployment](37-deployment.md) and [Security](27-security.md).

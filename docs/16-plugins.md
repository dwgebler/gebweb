# Plugins

Plugins are the standard extension point for Gebweb. They let
sibling packages register middleware, mount routes, install
authenticators, or wire any other framework hook through the
existing public app API. The OAuth2 / OIDC integration is shipped
this way; first-party features that aren't part of every app
(mailer transports, storage backends) follow the same pattern.

## The contract

A plugin is any value implementing the `PluginContract` interface:

    interface PluginContract {
        func name(): string;
        func install(any app): void;
    }

`install` runs once when `gebweb.plugin(app, instance)` is called.
The plugin can call any public `gebweb.*` helper (use,
useAuthenticator, useCacheStore, registerPerRequest, mount routes,
register a per-request scope, etc.) - it is just code that runs at
configuration time, after the controllers have been registered but
before the server starts.

The `app` parameter is typed as `any` so the `gebweb.plugin`
module doesn't import the `gebweb.app` module (which would form a
cycle). Cast inside `install` if you need a typed reference:

    func install(any app): void {
        let typed = app as gebweb.GebwebApp;
        gebweb.use(typed, this.middleware());
    }

## The convenience base class

`gebweb.Plugin` is a no-op base class that satisfies the contract
so plugins only need to override what they care about:

    import gebweb;

    class MetricsPlugin extends gebweb.Plugin {
        dict<string, any> config;

        func MetricsPlugin(dict<string, any> config) {
            parent("metrics");
            this.config = config;
        }

        func install(any app): void {
            gebweb.use(app, this.middleware());
        }

        func middleware(): callable {
            return func(any req, any res): any {
                /* record latency, status, ... */
                return res;
            };
        }
    }

    let app = gebweb.app([MyController()]);
    gebweb.plugin(app, MetricsPlugin({"namespace": "myapp"}));

## Registering and looking up

`gebweb.plugin(app, instance)` appends the plugin to
`app.plugins` and calls `instance.install(app)`. Plugins can be
looked up by name from elsewhere in the app:

    let metrics = gebweb.findPlugin(app, "metrics");
    if (metrics != null) {
        /* metrics is typed as ?PluginContract; cast to your class */
    }

`findPlugin` returns `null` when no plugin of that name is
registered. The name is whatever the plugin returns from `name()`;
collisions are not enforced (the first registered plugin of a
given name wins on lookup).

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.plugin(app, instance)` | Register and install a plugin. Returns the app. |
| `gebweb.findPlugin(app, name)` | Look up a registered plugin by name. Returns `?PluginContract`. |
| `gebweb.Plugin` | No-op base class for plugins. Subclass and override `install`. |
| `gebweb.PluginContract` | The interface plugins must satisfy. |

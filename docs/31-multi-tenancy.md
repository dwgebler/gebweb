# Multi-tenancy

A multi-tenant Gebweb app resolves the active tenant per request,
exposes it to handlers, and (in the shared-schema model) scopes
queries by `tenant_id`. The framework provides the resolver
plumbing and a handful of helpers; the storage layer remains the
app's responsibility.

## Wiring

```gb
import gebweb;

let app = gebweb.app([HomeController()]);
gebweb.useTenant(app, func(dict<string, any> request): ?gebweb.Tenant {
    let headers = request["headers"] as dict<string, any>;
    if (!headers.contains("Host")) {
        return null;
    }
    let host = headers["Host"] as string;
    let dot = host.indexOf(".");
    if (dot <= 0) {
        return null;
    }
    return gebweb.Tenant(host.substring(0, dot));
});
```

The resolver is any callable returning a `Tenant` (or null). When
null is returned and `opts.required` is true (the default), the
middleware short-circuits with a 400 Problem Details response.
Pass `{"required": false}` to let unauthenticated routes through
without a tenant.

The `Tenant` value lives on `request["_tenant"]`.

| Field | Type | Description |
|-------|------|-------------|
| `id` | `string` | Internal key (UUID, slug, numeric). |
| `name` | `string` | Optional human-readable label. |
| `attributes` | `dict<string, any>` | Anything else the resolver wanted to attach (plan tier, feature flags). |

## Tenant injection

Handlers declare `gebweb.Tenant` as a typed parameter:

```gb
class WidgetController {
    @Get("/widgets")
    func list(gebweb.Tenant tenant): list<Widget> {
        return widgetRepo.findAllForTenant(tenant.id);
    }
}
```

Services that don't receive the request directly can resolve from
the request dict via `gebweb.currentTenant(request)` and
`gebweb.currentTenantId(request)`.

## Shared-schema helpers

The 1.1 line supports the most common SaaS shape: one database,
one schema, a `tenant_id` column on every multi-tenant table.

```gb
import db;
import gebweb;
import gebweb.query as query;
import gebweb.repository as repository;
import secrets;

class Widget {
    string id;
    string tenant_id;
    string name;

    func Widget(string id, string name, string tenant_id = "") {
        this.id = id;
        this.name = name;
        this.tenant_id = tenant_id;
    }
}

func toWidget(dict<string, any> row): Widget {
    return Widget(row["id"] as string, row["name"] as string, row["tenant_id"] as string);
}

class WidgetRepo {
    db.Connection conn;

    func WidgetRepo(db.Connection conn) {
        this.conn = conn;
    }

    func save(Widget w, gebweb.Tenant tenant): Widget {
        if (w.id == "") {
            w.id = secrets.randomHex(8);
        }
        gebweb.stampTenant(w, tenant);
        this.conn.exec(
            "INSERT INTO widgets (id, tenant_id, name) VALUES (?, ?, ?)",
            w.id,
            w.tenant_id,
            w.name
        );
        return w;
    }

    func list(repository.Page page, gebweb.Tenant tenant): list<Widget> {
        let stmt = gebweb.scopedQuery(query.Query("widgets"), tenant)
            .orderBy(query.asc("id"))
            .limit(page.size)
            .offset(page.offset)
            .select([]);
        let rows = this.conn.query(stmt["text"] as string, ...(stmt["params"] as list<any>));
        return rows.all().map(toWidget);
    }

    func find(string id, gebweb.Tenant tenant): ?Widget {
        let row = this.conn.queryOne("SELECT * FROM widgets WHERE id = ?", id);
        if (row == null) {
            return null;
        }
        let w = toWidget(row);
        return gebweb.tenantOwns(w, tenant) ? w : null;
    }
}
```

| Helper | Behaviour |
|--------|-----------|
| `gebweb.stampTenant(entity, tenant)` | Sets `entity.tenant_id`. |
| `gebweb.tenantOwns(entity, tenant)` | True when `entity.tenant_id` equals `tenant.id`. Use as a guard before returning an entity fetched by id. |
| `gebweb.scopedQuery(query, tenant)` | Appends `tenant_id = ?` to a `query.Query`. |

## Per-tenant DB connections

The 1.1 line does not include a connection-per-tenant DI
helper. The recommended pattern: write a service that looks up the
right `db.Connection` for the resolved tenant and inject the service
into your repositories.

A first-class `useTenantDatabase(app, connectionFor)` helper plus
the tenant-aware migration runner are scoped for 1.2.

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.useTenant(app, resolver, opts)` | Mount the resolver middleware. |
| `gebweb.Tenant` | Typed parameter; injected from `request["_tenant"]`. |
| `gebweb.currentTenant(request)` | Read the resolved tenant; null when absent. |
| `gebweb.currentTenantId(request)` | Convenience accessor; empty string when absent. |
| `gebweb.stampTenant(entity, tenant)` | Set `tenant_id`. |
| `gebweb.tenantOwns(entity, tenant)` | Ownership guard. |
| `gebweb.scopedQuery(query, tenant)` | Add `tenant_id = ?` to a query. |

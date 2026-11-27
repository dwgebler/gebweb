# Multi-tenancy

<!-- doctest:strict -->

Gebweb resolves a tenant per request and injects it into handlers. It does not
automatically authorize the caller for that tenant, filter every database
query or partition every cache. Keep those decisions explicit at the request
and storage boundaries.

## Wiring

Put this application in `src/tenant_app.gb`. The resolver maps known hosts to
internal tenant IDs; an arbitrary subdomain does not create a tenant. The
fixed example API keys stand in for a real authenticator and membership store:

<!-- doctest:file tenant_app.gb -->
```gb
import gebweb;

class Member {
    string id;
    list<string> tenantIds;
    string activeTenant;

    func Member(string id, list<string> tenantIds, string activeTenant) {
        this.id = id;
        this.tenantIds = tenantIds;
        this.activeTenant = activeTenant;
    }
}

@Controller("")
class TenantController extends gebweb.Controller {
    @Get("/whoami")
    @Auth
    func whoami(Member member, gebweb.Tenant tenant): dict<string, any> {
        if (!member.tenantIds.contains(tenant.id)) {
            this.forbidden("Tenant membership required");
        }
        return {"user": member.id, "tenant": tenant.id, "name": tenant.name};
    }

    @Get("/status")
    func status(gebweb.Request request): dict<string, any> {
        return {"tenant": gebweb.currentTenantId(request.toDict())};
    }
}

export func build(bool required = true): gebweb.GebwebApp {
    let app = gebweb.app();
    gebweb.useTenant(app, func(gebweb.Request request): ?gebweb.Tenant {
        string host = (request.header("Host") ?? request.host()).lower();
        if (host == "acme.example.test") {
            return gebweb.Tenant("acme", "Acme", {"plan": "standard"});
        }
        if (host == "beta.example.test") {
            return gebweb.Tenant("beta", "Beta", {"plan": "standard"});
        }
        return null;
    }, {"required": required});
    gebweb.useAuthenticator(app, Member, func(gebweb.Request request): ?any {
        string tenantId = gebweb.currentTenantId(request.toDict());
        string key = request.header("X-API-Key") ?? "";
        if (key == "example-acme-key") {
            return Member("ada", ["acme"], tenantId);
        }
        if (key == "example-both-key") {
            return Member("lin", ["acme", "beta"], tenantId);
        }
        return null;
    });
    gebweb.useUserScope(app, func(any user): string {
        let member = user as Member;
        return member.id + "|" + member.activeTenant;
    });
    return app;
}
```

Use the normal [application entry point](01-getting-started.md) in production.
Resolve host names through application-owned configuration/database records;
validate the requested tenant against the authenticated user's membership.
The example intentionally requires exact hosts without ports. Define your
own accepted host/port normalization and trusted-proxy policy rather than
blindly trusting a forwarded host or tenant header.

Exercise both tenant resolution and authorization:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import tenant_app;

let client = gebweb.TestClient(tenant_app.build());
let acme = {"Host": "acme.example.test", "X-API-Key": "example-acme-key"};
io.println(client.request("GET", "/whoami", null, acme).text());
io.println(
    client.request("GET", "/whoami", null, {"Host": "beta.example.test", "X-API-Key": "example-acme-key"}).status
);
io.println(client.request("GET", "/whoami", null, {"Host": "acme.example.test"}).status);
io.println(client.request("GET", "/whoami", null, {"Host": "unknown.example.test"}).status);
```

```text
{"name":"Acme","tenant":"acme","user":"ada"}
403
401
400
```

Selecting a tenant and authenticating a user are separate decisions. A known
host with a valid credential for another tenant still gets 403 from the
membership check. No protected data is cached in this example.

| `useTenant` option | Default | Meaning |
|--------------------|---------|---------|
| `required` | `true` | A resolver result of null returns 400 Problem Details before routing. False continues without a tenant. |

The resolver receives a rich `gebweb.Request`, not a raw dictionary. Use its
case-insensitive `header` accessor and `toDict()` when calling helpers that
accept dictionaries. Configure one resolver during startup, before requests.
It runs as application before-middleware, in registration order; a preceding
gate can short-circuit before it runs.

## Tenant injection

`gebweb.Tenant` is an alias of the framework tenant class:

| Field | Type | Meaning |
|-------|------|---------|
| `id` | `string` | Stable internal key. Represent numeric identifiers as strings. |
| `name` | `string` | Display label, default empty. |
| `attributes` | `dict<string, any>` | Application metadata, default empty. |

Handler parameters receive the value stored at `request["_tenant"]`.
`currentTenant(request.toDict())` returns it or null;
`currentTenantId(request.toDict())` returns its ID or an empty string.
These helpers require the request: they do not read a global current tenant.
Pass the resolved, authorized tenant explicitly into repository/service calls.

When tenants are optional, public endpoints must handle their absence. Setting
`required: false` changes tenant resolution, not authentication requirements:

<!-- doctest:run -->
```gb
import gebweb;
import io;
import tenant_app;

let required = gebweb.TestClient(tenant_app.build());
let optional = gebweb.TestClient(tenant_app.build(false));
io.println(required.get("/status").status);
io.println(optional.get("/status").text());
io.println(optional.request("GET", "/status", null, {"Host": "beta.example.test"}).text());
```

```text
400
{"tenant":""}
{"tenant":"beta"}
```

An endpoint needing a tenant should enforce that requirement even in an
optional app. Do not store the current tenant on a singleton service or shared
controller: those instances can handle several tenants concurrently.

## Shared-schema helpers

A shared-schema design places a `tenant_id` column on each tenant-owned table.
Include that column in relevant uniqueness constraints, indexes, foreign keys
and every read/write predicate. Gebweb supplies three explicit helpers:

| Helper | Behavior |
|--------|----------|
| `stampTenant(entity, tenant)` | Overwrite the entity's declared `tenant_id` field; return the same entity. |
| `tenantOwns(entity, tenant)` | Compare the entity's `tenant_id` with `tenant.id`. |
| `scopedQuery(query, tenant)` | Add an AND predicate for `tenant_id`; return the same mutable query. |

Stamp new objects with the server-resolved tenant. Stamping an existing object
is not an ownership check and can overwrite its previous owner. Do not accept
tenant ownership from JSON/form fields or reuse one mutable query across
requests.

Put this custom SQLite repository in `src/widget_repo.gb`. Every operation
takes a tenant explicitly, including updates/deletes and lookup by ID. IDs may
repeat in different tenants:

<!-- doctest:file widget_repo.gb -->
```gb
import db;
import gebweb;
import gebweb.query as query;

export class Widget {
    string id;
    string name;
    string tenant_id = "";

    func Widget(string id, string name) {
        this.id = id;
        this.name = name;
    }
}

export class WidgetRepo {
    db.Connection conn;

    func WidgetRepo(db.Connection conn) {
        this.conn = conn;
    }

    func insert(Widget widget, gebweb.Tenant tenant): void {
        gebweb.stampTenant(widget, tenant);
        this.conn.exec(
            "INSERT INTO widgets (tenant_id, id, name) VALUES (?, ?, ?)",
            widget.tenant_id,
            widget.id,
            widget.name
        );
    }

    func find(string id, gebweb.Tenant tenant): ?Widget {
        let row = this.conn.queryOne(
            "SELECT id, tenant_id, name FROM widgets WHERE tenant_id = ? AND id = ?",
            tenant.id,
            id
        );
        if (row == null) {
            return null;
        }
        let widget = Widget(row["id"] as string, row["name"] as string);
        widget.tenant_id = row["tenant_id"] as string;
        return widget;
    }

    func list(gebweb.Tenant tenant): list<Widget> {
        let statement = gebweb.scopedQuery(query.Query("widgets"), tenant)
            .orderBy(query.asc("id")).select([]);
        let rows = this.conn.query(
            statement["text"] as string,
            ...(statement["params"] as list<any>)
        );
        list<Widget> widgets = [];
        for (row in rows.all()) {
            let widget = Widget(row["id"] as string, row["name"] as string);
            widget.tenant_id = row["tenant_id"] as string;
            widgets.push(widget);
        }
        return widgets;
    }

    func rename(string id, string name, gebweb.Tenant tenant): bool {
        let result = this.conn.exec(
            "UPDATE widgets SET name = ? WHERE tenant_id = ? AND id = ?",
            name,
            tenant.id,
            id
        );
        return (result["rowsAffected"] as int) > 0;
    }

    func delete(string id, gebweb.Tenant tenant): bool {
        let statement = gebweb.scopedQuery(query.Query("widgets").where(query.eq("id", id)), tenant).delete();
        let result = this.conn.exec(
            statement["text"] as string,
            ...(statement["params"] as list<any>)
        );
        return (result["rowsAffected"] as int) > 0;
    }
}
```

Use parameter syntax appropriate to the database driver. This example uses
SQLite `?` parameters and is a custom repository, not a drop-in implementation
of the automatic `@ApiResource` repository contract. The framework does not
insert these tenant arguments into arbitrary repository methods.

The following complete program checks two tenants using the same ID, plus a
record belonging only to Beta:

<!-- doctest:run -->
```gb
import db;
import gebweb;
import io;
import widget_repo;

let conn = db.connect("sqlite", ":memory:");
try {
    conn.exec(
        "CREATE TABLE widgets (tenant_id TEXT NOT NULL, id TEXT NOT NULL, name TEXT NOT NULL, PRIMARY KEY (tenant_id, id))"
    );
    let repo = widget_repo.WidgetRepo(conn);
    let acme = gebweb.Tenant("acme");
    let beta = gebweb.Tenant("beta");
    let created = widget_repo.Widget("shared", "Acme widget");
    repo.insert(created, acme);
    repo.insert(widget_repo.Widget("shared", "Beta widget"), beta);
    repo.insert(widget_repo.Widget("beta-only", "Private widget"), beta);
    io.println(created.tenant_id);
    io.println(repo.list(acme).length());
    io.println((repo.find("shared", acme) as widget_repo.Widget).name);
    io.println(repo.find("beta-only", acme) == null);
    io.println(repo.rename("shared", "Acme revised", acme));
    io.println((repo.find("shared", beta) as widget_repo.Widget).name);
    io.println(repo.rename("beta-only", "Wrong tenant", acme));
    io.println(repo.delete("beta-only", acme));
    io.println(gebweb.tenantOwns(repo.find("shared", beta), acme));
    io.println(repo.delete("shared", acme));
    io.println(repo.list(beta).length());
} finally {
    conn.close();
}
```

```text
acme
1
Acme widget
true
true
Beta widget
false
false
false
true
2
```

Call these methods only after the handler's membership/authorization check.
For storage operations, put ownership predicates in SQL rather than reading
an unscoped row and checking it later. Scope counts, joins, relation lookups,
exports and bulk operations too. Multi-step invariants need transactions and
constraints; `tenantOwns` alone cannot make a read-then-write sequence atomic.
See [Repositories](06-repositories.md) and
[Database connections](06b-database-connections.md).

## Caches, idempotency and background work

The example puts the selected tenant on a fresh per-request `Member` value and
sets `useUserScope` after authentication setup. The composite user/tenant scope
partitions authenticated `@Cache` entries and `@Idempotent` records. The scope
extractor receives the user, not the request: populate that value through
trusted authentication/resolution logic, not a shared mutable user object.

A cache hit can skip handler-level membership checks. Before adding `@Cache`
to protected tenant data, enforce membership before cache lookup through the
authenticator or a route policy (`@Can`), not only inside the handler. Do not
assume a tenant resolver is an authorization gate.

Public response caches also need a tenant dimension, such as a validated host
or tenant-selection header in `@Cache(vary: [...])`. Tenant metadata is not
automatically added to cache keys. The same applies to CDN cache keys and
application-owned caches, file paths and object-storage keys. See
[Caching](10-caching.md), [Idempotency](10-caching.md#idempotent-writes) and
[Authentication](40-authentication.md).

Jobs and event handlers do not inherit an HTTP request's `_tenant`. Put the
stable tenant ID in their data, resolve it from trusted records when handling
work, and pass it into scoped storage operations. Do not serialize a database
connection or rely on a process-global current tenant.

## Per-tenant DB connections

There is no built-in connection-per-tenant resolver or tenant migration runner.
A service can map an authorized tenant ID to a configured connection/pool;
request handlers then pass that connection or the tenant to repositories.
Keep credentials and database names in application-owned configuration, bound
pool counts, and define shutdown ownership. Do not create a new unbounded pool
for every request or derive database identifiers directly from HTTP input.

For separate schemas/databases, plan migrations, backups and connection limits
for every tenant.

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.useTenant(app, resolver, opts = {})` | Mount request-phase tenant resolution; returns the app. |
| `gebweb.Tenant(id, name = "", attributes = {})` | Construct a tenant value. |
| `gebweb.currentTenant(request)` | Tenant or null from a request dictionary. |
| `gebweb.currentTenantId(request)` | Tenant ID or empty string. |
| `gebweb.stampTenant(entity, tenant)` | Set the declared `tenant_id` field; return the entity. |
| `gebweb.tenantOwns(entity, tenant)` | Compare entity ownership. |
| `gebweb.scopedQuery(query, tenant)` | Add `tenant_id` equality to the existing query predicate. |

See [Middleware](09-middleware.md), [Dependency injection](06a-dependency-injection.md),
[Authentication](40-authentication.md) and [Security](27-security.md).

# Application best practices

<!-- doctest:strict -->

Each section below states a practice, explains what goes wrong without it, and
shows code to avoid next to code to prefer. The "avoid" examples are valid
Geblang that runs; the trouble they cause shows up under concurrent load, on a
second instance, or on a failure path, which is why they are easy to ship.
The [bookmarks tutorial](38-building-bookmarks.md) applies all of them in one
complete application.

## Keep boundaries typed and explicit

Data from a client is untrusted until it has been checked against a type and
its rules. Gebweb does that check for you when a handler declares a typed body
parameter: the body is bound to an instance of the class, every `@Assert` rule
runs, and the handler only ever sees valid input. A handler that reads the
body itself gets none of that, and every service it passes the raw value to
has to repeat the checks or trust that someone else did.

Avoid reading the body by hand and passing loosely typed values inward:

<!-- doctest:fragment context=script -->
```gb
class TaskService {
    func create(dict<string, any> data): dict<string, any> {
        return {"title": data["title"], "priority": data["priority"]};
    }
}

class TaskController extends gebweb.Controller {
    TaskService tasks;

    func TaskController(TaskService tasks) {
        parent();
        this.tasks = tasks;
    }

    @Post("/tasks")
    func create(gebweb.Request req): dict<string, any> {
        return this.tasks.create(req.json() as dict<string, any>);
    }
}
```

A missing `title`, a `priority` of `"urgent!!"`, or an extra `ownerId` field
all reach the service, and the response echoes whatever the client sent.

Prefer an input class with rules, a separate output class, and typed service
methods:

<!-- doctest:run -->
```gb
import gebweb;
import io;

class NewTask {
    @Assert.notBlank
    @Assert.maxLength(200)
    string title;

    @Assert.range(min: 1, max: 5)
    int priority;
}

class Task {
    int id;
    string title;
    int priority;

    func Task(int id, string title, int priority) {
        this.id = id;
        this.title = title;
        this.priority = priority;
    }
}

class TaskService {
    func create(NewTask input): Task {
        return Task(1, input.title, input.priority);
    }
}

@Controller("/tasks")
class TaskController extends gebweb.Controller {
    TaskService tasks;

    func TaskController(TaskService tasks) {
        parent();
        this.tasks = tasks;
    }

    @Post("")
    func create(@Body NewTask input): gebweb.Response {
        let task = this.tasks.create(input);
        return this.created(task, "/tasks/${task.id}");
    }
}

let client = gebweb.TestClient(gebweb.app());
let ok = client.post("/tasks", {"title": "Write docs", "priority": 2, "ownerId": 99});
io.println("${ok.status} ${ok.text()}");
let bad = client.post("/tasks", {"title": "", "priority": 9});
for (e in bad.json()["errors"] as list<any>) {
    io.println("${bad.status} ${e["field"]}: ${e["message"]}");
}
```

Output:

```text
201 {"id":1,"priority":2,"title":"Write docs"}
422 priority: must be between 1 and 5
422 title: must not be blank
```

The unknown `ownerId` field never reaches the code, and an invalid body is
rejected with every failure listed before the handler runs. `TaskService`
takes a `NewTask`, so it can be called the same way from a job, a test or a
command-line tool. Keep parsing at the HTTP boundary, decisions in services,
and SQL in repositories. See [Parameter binding](03-parameter-binding.md),
[Validation](05-validation.md) and [Responses](04-responses.md).

## Own application construction in one place

An app wired by top-level code is hard to test: the test cannot import the
module without opening the production database and starting the server.
Configuration read deep inside the wiring is invisible from the outside and
differs between the server and the tests.

Avoid wiring at the top level of a module:

<!-- doctest:fragment context=script -->
```gb
class NoteRepository {
    db.Connection conn;

    func NoteRepository(db.Connection conn) {
        this.conn = conn;
    }
}

let conn = db.connect("sqlite", sys.getenv("DATABASE_URL") ?? "notes.db");
let app = gebweb.app();
gebweb.registerInstance(app, NoteRepository, NoteRepository(conn));
gebweb.cli(app, {"name": "notes"});
```

Prefer a `build` function that receives every dependency and configuration
value, an entry point that is the only code reading the environment, and tests
that call the same `build`:

<!-- doctest:fragment context=script -->
```gb
class NoteRepository {
    db.Connection conn;

    func NoteRepository(db.Connection conn) {
        this.conn = conn;
    }
}

func build(db.Connection conn, string csrfSecret): gebweb.GebwebApp {
    let app = gebweb.app();
    gebweb.registerInstance(app, NoteRepository, NoteRepository(conn));
    gebweb.useCsrf(app, {"secret": csrfSecret});
    return app;
}

class NotesTest extends test.Test {
    db.Connection conn;
    gebweb.TestClient client;

    func setUp(): void {
        this.conn = db.connect("sqlite", ":memory:");
        this.client = gebweb.TestClient(build(this.conn, "test-secret"));
    }

    func tearDown(): void {
        this.conn.close();
    }
}
```

[Getting started](01-getting-started.md#a-first-api) shows the full shape:
`app.gb` holds `build`, and `main.gb` reads configuration, calls `build` and
serves. Register infrastructure-backed services in `build` before the first
request, and import every module that holds controllers or handlers there,
because discovery only sees loaded classes.

## Choose a lifetime before choosing a binding

Controllers and ordinary services are created once and shared by every
request, and requests run in parallel. A value that belongs to one request
must therefore never be stored in a field of a shared object, because the
next request overwrites it while the first is still using it.

| Value | Lifetime and owner |
| --- | --- |
| Database pool or reusable client | Created by the entry point; shared until shutdown. |
| Read-only configuration | Built at startup; shared, never mutated. |
| Current request, user or route inputs | Handler parameters and locals. |
| Mutable state for one request | A `registerPerRequest` service, resolved during the request. |
| Transaction, cursor or open file | The operation that acquires it; released before it returns. |
| State that must survive a restart | The database or another backing store. |

Avoid keeping the current user in a controller field:

<!-- doctest:fragment context=script -->
```gb
class Account {
    string id;
}

class ProfileController extends gebweb.Controller {
    ?Account current = null;

    @Get("/profile")
    @Auth
    func show(Account user): dict<string, any> {
        this.current = user;
        return this.summary();
    }

    func summary(): dict<string, any> {
        return {"id": (this.current as Account).id};
    }
}
```

Two users loading their profiles at the same moment can each receive the
other's data. Prefer passing the value along:

<!-- doctest:fragment context=script -->
```gb
class Account {
    string id;
}

class ProfileController extends gebweb.Controller {
    @Get("/profile")
    @Auth
    func show(Account user): dict<string, any> {
        return this.summary(user);
    }

    func summary(Account user): dict<string, any> {
        return {"id": user.id};
    }
}
```

`registerPerRequest` gives one instance per request to every resolve made
while that request is handled. A controller is constructed only once, so a
per-request service injected into its constructor is the first instance
forever; resolve per-request services during the request instead. See
[Dependency injection](06a-dependency-injection.md#lifecycle-model).

## Make shared writes one operation

A plain `dict`, `list` or object field shared by concurrent requests is not
safe to modify: two requests writing it at once can lose updates or crash the
process. A read followed by a write is two operations even when each one is
safe on its own, so another request can act between them.

Avoid counting in a shared dictionary:

<!-- doctest:fragment context=script -->
```gb
@Service
class HitCounter {
    dict<string, int> hits = {};

    func record(string page): int {
        int next = (this.hits[page] ?? 0) + 1;
        this.hits[page] = next;
        return next;
    }
}
```

Prefer `store.Store`, whose operations are atomic. `incr` adds in one step;
`update` applies a function to the current value in one step; `compareAndSet`
changes a value only if it still holds what you last read:

<!-- doctest:run -->
```gb
import io;
import store;

class HitCounter {
    store.Store hits = store.Store();

    func record(string page): int {
        return this.hits.incr(page);
    }

    func recordBest(string page, int score): int {
        return this.hits.update("best:" + page, func(any current): any {
            int previous = current == null ? 0 : current as int;
            return score > previous ? score : previous;
        }) as int;
    }
}

let counter = HitCounter();
io.println(counter.record("/home"));
io.println(counter.record("/home"));
io.println(counter.recordBest("/home", 40));
io.println(counter.recordBest("/home", 25));
```

Output:

```text
1
2
40
40
```

A `Store` lives in one process. When several instances serve the app, keep
shared counters and invariants in the database or the cache store, and guard
an invariant that spans several rows with a transaction. See
[Concurrency and shared state](35-concurrency.md).

## Give database resources an owner

Every database resource has exactly one owner: the code that acquired it. The
owner releases it, on every path including errors and early returns, and
nothing else does. Code that only uses a resource never releases it.

| Resource | Owner | Released by | Never |
| --- | --- | --- | --- |
| `db.Connection` (the pool) | The entry point that opened it | A `gebweb.onShutdown` hook | Closed by a repository, handler or job |
| `db.Transaction` | The method that called `begin` | `commit` or `rollback` before that method returns | Stored in a field or left open across requests |
| `db.Rows` (a cursor) | The function that ran the query | Reading it to the end, or `defer rows.close()` | Abandoned half-read on an early return |

An unreleased transaction or cursor keeps one of the pool's connections busy.
Under load the pool runs out, and every request that needs the database waits.
A pool closed by the wrong code fails every request that comes after it.

Avoid closing the shared pool in a repository, or keeping a transaction in a
field:

<!-- doctest:fragment context=script -->
```gb
class OrderRepository {
    db.Connection conn;
    ?db.Transaction tx = null;

    func OrderRepository(db.Connection conn) {
        this.conn = conn;
    }

    func startOrder(): void {
        this.tx = this.conn.begin();
    }

    func addLine(int orderId, string sku): void {
        (this.tx as db.Transaction).exec(
            "INSERT INTO lines (order_id, sku) VALUES (?, ?)",
            orderId,
            sku
        );
    }

    func finish(): void {
        (this.tx as db.Transaction).commit();
        this.conn.close();
    }
}
```

The repository is shared, so concurrent requests share one `tx` field, and the
first `finish` closes the pool for everyone.

Prefer a transaction that begins and ends in one method, passed to anything
that must take part in it. Here a transfer that fails halfway leaves both
balances as they were:

<!-- doctest:run -->
```gb
import db;
import io;

class AccountRepository {
    db.Connection conn;

    func AccountRepository(db.Connection conn) {
        this.conn = conn;
    }

    func balance(string id): int {
        let row = this.conn.queryOne("SELECT balance FROM accounts WHERE id = ?", id);
        return (row as dict<string, any>)["balance"] as int;
    }

    func adjust(db.Transaction tx, string id, int amount): void {
        tx.exec("UPDATE accounts SET balance = balance + ? WHERE id = ?", amount, id);
        let row = tx.query("SELECT balance FROM accounts WHERE id = ?", id).first();
        if (((row as dict<string, any>)["balance"] as int) < 0) {
            throw ValueError("insufficient funds in ${id}");
        }
    }

    func transfer(string from, string to, int amount): void {
        let tx = this.conn.begin(2000);
        try {
            this.adjust(tx, to, amount);
            this.adjust(tx, from, -amount);
            tx.commit();
        } catch (Error e) {
            tx.rollback();
            throw e;
        }
    }
}

let conn = db.connect("sqlite", ":memory:");
defer conn.close();
conn.exec("CREATE TABLE accounts (id TEXT PRIMARY KEY, balance INTEGER NOT NULL)");
conn.exec("INSERT INTO accounts (id, balance) VALUES ('alice', 100), ('bob', 0)");
let accounts = AccountRepository(conn);

accounts.transfer("alice", "bob", 30);
try {
    accounts.transfer("alice", "bob", 500);
} catch (ValueError e) {
    io.println("refused: ${e.message}");
}
io.println("alice ${accounts.balance("alice")}, bob ${accounts.balance("bob")}");
```

Output:

```text
refused: insufficient funds in alice
alice 70, bob 30
```

The failed transfer had already credited Bob before the debit failed; the
rollback undid it. Queries made through `conn` while a transaction is open run
on another connection and are not part of it, so pass the `db.Transaction` to
every step that belongs to the operation, as `adjust` does. Keep slow work
such as HTTP calls or email outside the transaction, because it holds its
connection until it ends.

For cursors, `queryOne` and `all()` read the whole result and release the
connection at once; use them for bounded results and add paging when a result
can grow with your data. When you stream with `for`, close the cursor on every
exit:

<!-- doctest:fragment context=script -->
```gb
func firstOverdue(db.Connection conn, int today): ?string {
    let rows = conn.query("SELECT id, due FROM invoices ORDER BY due");
    defer rows.close();
    for (row in rows) {
        if ((row["due"] as int) < today) {
            return row["id"] as string;
        }
    }
    return null;
}
```

Without the `defer`, the early `return` would leave the cursor, and its
connection, open. The pool itself is opened and closed by the entry point, as
in [Getting started](01-getting-started.md#the-entry-point); see
[Database connections and concurrency](06b-database-connections.md) for sizing
it.

Keep migration ids stable once applied and add a new migration for every
schema change. Decide whether deployment or startup runs migrations: the
bookmarks app calls `conn.migrate` in `build`, and
[Database migrations](17-migrations.md) describes the CLI workflow.

## Separate identity, authorization and request integrity

Authentication tells you who is calling. It does not tell you whether that
caller may see or change a particular record: an id in the path or body only
says which record was asked for. Check ownership in the query that loads the
record, using the identity the authenticator resolved.

Avoid loading a record by id alone:

<!-- doctest:fragment context=script -->
```gb
class Account {
    string id;
}

class DocumentController extends gebweb.Controller {
    db.Connection conn;

    func DocumentController(db.Connection conn) {
        parent();
        this.conn = conn;
    }

    @Get("/documents/{id}")
    @Auth
    func show(Account user, int id): dict<string, any> {
        let row = this.conn.queryOne("SELECT id, title FROM documents WHERE id = ?", id);
        if (row == null) {
            this.notFound("no document ${id}");
        }
        return row as dict<string, any>;
    }
}
```

Every signed-in user can read every document by guessing ids. Prefer scoping
the query to the caller:

<!-- doctest:run -->
```gb
import db;
import gebweb;
import io;

class Account {
    string id;

    func Account(string id) {
        this.id = id;
    }
}

class DocumentRepository {
    db.Connection conn;

    func DocumentRepository(db.Connection conn) {
        this.conn = conn;
    }

    func findOwned(int id, string ownerId): ?dict<string, any> {
        return this.conn.queryOne(
            "SELECT id, title FROM documents WHERE id = ? AND owner_id = ?",
            id,
            ownerId
        );
    }
}

@Controller("/documents")
class DocumentController extends gebweb.Controller {
    DocumentRepository documents;

    func DocumentController(DocumentRepository documents) {
        parent();
        this.documents = documents;
    }

    @Get("/{id}")
    @Auth
    func show(Account user, int id): dict<string, any> {
        let found = this.documents.findOwned(id, user.id);
        if (found == null) {
            this.notFound("no document ${id}");
        }
        return found as dict<string, any>;
    }
}

func accountForKey(string key): ?Account {
    return key.startsWith("key-") ? Account(key.substring(4)) : null;
}

let conn = db.connect("sqlite", ":memory:");
defer conn.close();
conn.exec(
    "CREATE TABLE documents (id INTEGER PRIMARY KEY, owner_id TEXT NOT NULL, title TEXT NOT NULL)"
);
conn.exec("INSERT INTO documents (id, owner_id, title) VALUES (1, 'alice', 'Alice plan')");

let app = gebweb.app();
gebweb.registerInstance(app, DocumentRepository, DocumentRepository(conn));
gebweb.useApiKeyAuth(app, Account, accountForKey);
let client = gebweb.TestClient(app);

for (key in ["key-alice", "key-bob"]) {
    let r = client.request("GET", "/documents/1", null, {"X-API-Key": key});
    io.println("${key}: ${r.status}");
}
```

Output:

```text
key-alice: 200
key-bob: 404
```

Answering 404 rather than 403 avoids confirming that the record exists. For
rules that are more than "the owner may", write an authorization policy; see
[Authentication](08-auth.md#authorization-policies).

Browser requests carry cookies automatically, so a form or script on another
site can make a signed-in user's browser send a write request. Turn on
`gebweb.useCsrf` with a random, nonblank secret for every app that
authenticates browsers with cookies, and keep it on for JSON routes those
browsers call. Keep GET handlers free of changes. A route that must accept
writes from elsewhere, such as a payment provider's webhook, can be marked
`@CsrfExempt`, but only when it verifies the sender another way, such as a
signature check; the exemption itself authenticates nothing. See
[CSRF protection](23-csrf.md).

Set cookie flags where each cookie is issued: the CSRF options configure only
the CSRF cookie, not the session cookie. Check `Secure`, `HttpOnly`,
`SameSite`, path and expiry on real responses from the deployment you serve.

## Preserve the intended browser flow

Server-rendered pages rely on per-request context: the CSRF token, flash
messages, and the submitted values and errors after a failed form. That
context reaches a template only when the handler passes the active request to
`this.view`:

<!-- doctest:fragment context=controller -->
```gb
@Get("/widgets/new")
func newForm(gebweb.Request request): http.Response {
    return this.view(request, "widgets/new.html", {"title": "New widget"});
}
```

Keep automatic HTML escaping on for stored values and submitted input, and use
the `raw` filter only for HTML you produced and trust.

After a failed form submission Gebweb stores the submitted values and errors
in the session and redirects back, and the next render consumes them. Decide
which fields may be stored that way; a form with a password or card number
can return its own error response instead. Test the redirect and the render
after it, so one-time state does not linger. Sessions can rotate their cookie,
so carry the latest `Set-Cookie` values into the next request, in tests as in
browsers. See [Forms](25-forms.md) and [Flash messages](24-flash.md).

## Cache a response only within its actual scope

A cached response is replayed to everyone whose request produces the same
cache key, so the key must include everything the response depends on: the
path and query, any header that changes it, and the user or tenant it
belongs to.

Avoid caching a route whose content depends on who is asking without telling
the cache:

<!-- doctest:fragment context=script -->
```gb
class FeedController extends gebweb.Controller {
    @Get("/feed")
    @Cache(ttl: 60)
    func feed(gebweb.Request req): dict<string, any> {
        return {"for": req.cookie("user") ?? "guest"};
    }
}
```

The first visitor's feed is served to every visitor for a minute. Prefer
authenticating, which partitions the cache by user, and naming the headers
that vary:

<!-- doctest:fragment context=script -->
```gb
class Account {
    string id;
}

class FeedController extends gebweb.Controller {
    @Get("/feed")
    @Auth
    @Cache(ttl: 60, vary: ["Accept-Language"])
    func feed(Account user): dict<string, any> {
        return {"for": user.id};
    }
}
```

When one user can see different data in different contexts, such as per
tenant, give the cache a scope that includes it with `gebweb.useUserScope`.
Do not cache pages that carry a CSRF token, flash messages or a CSP nonce:
the cached copy would hand one visitor's token or stale nonce to the next.
Test invalidation through the route that changes the data, not only by
waiting for the TTL. See [Caching](10-caching.md).

## Validate configuration after loading it

`secrets.requireEnv` and `sys.getenv` tell you whether a variable exists, not
whether its value is usable: an empty string is present, and a port of `"abc"`
is a string like any other. A bad value then fails far from its cause, on the
first request that uses it.

Avoid using configuration straight from the environment:

<!-- doctest:fragment context=entrypoint -->
```gb
let secret = sys.getenv("CSRF_SECRET") ?? "";
gebweb.cli(build(secret), {"name": "myapp"});
```

Prefer checking every value at startup and refusing to start with a message
that names the problem:

<!-- doctest:fragment context=entrypoint -->
```gb
cli.checkArgs(args, {"name": "myapp"});
string secret = (sys.getenv("CSRF_SECRET") ?? "").trim();
if (secret.length() < 32) {
    io.stderrPrintln("myapp: CSRF_SECRET must be at least 32 characters");
    return 2;
}
string poolSize = sys.getenv("DB_POOL_SIZE") ?? "8";
if (!poolSize.isInt() || poolSize.toInt() < 1) {
    io.stderrPrintln("myapp: DB_POOL_SIZE must be a positive whole number");
    return 2;
}
gebweb.cli(build(secret), {"name": "myapp"});
```

`dotenv.loadAndApply` overwrites existing environment variables with the
file's values, and `gebweb.cli` then applies flags over the environment over
its `opts` defaults; decide which source wins before relying on it. Keep
`.env` files, databases, session directories and other runtime data out of
source control and out of public asset directories. Every instance that must
read the same sessions needs a session store they share; separate local
directories do not.

## Close request resources and observe background work

A handle opened while handling a request belongs to that request. Release it
where it was opened, on every path:

<!-- doctest:fragment context=script -->
```gb
class ReportController extends gebweb.Controller {
    @Get("/reports/latest")
    func latest(): string {
        with (f = file.open("reports/latest.txt", "r")) {
            return f.readAll();
        }
    }
}
```

`with` closes the file even when the read throws. When a handle is handed to a
streaming response, the stream's producer owns it from then on and closes it
when the stream ends. For resources recorded on the request, register
`gebweb.onRequestEnd`, which runs after every request, including failed ones;
see [Middleware](09-middleware.md#guaranteed-request-cleanup).

Work that must finish after the response, such as sending email or calling a
slow API, needs an owner that sees its result. A background task started from
a handler and never checked fails silently, and is lost if the process
restarts. Use the job queue: jobs are stored, retried on failure, and run by a
worker that drains at shutdown. Jobs can run more than once, so make their
effects idempotent and record what has been done. A job timeout asks the job
to stop; long jobs should check for cancellation. See
[Background jobs](18-background-jobs.md).

## Test observable behavior in the right environment

Tests that call `build` with an in-memory database exercise the same wiring as
production, through the same routes a client uses. Assert on responses and on
stored data, not on which internal methods were called; such tests survive
refactoring and catch the failures users would see.

| Change | Useful verification |
| --- | --- |
| Typed body or validation rule | Valid and invalid input; HTML redirect and JSON 422. |
| Private resource route | Owner, other user and anonymous requests. |
| Browser form or session behavior | Missing or wrong CSRF token, redirects, latest cookies, one-time state, escaped output. |
| Shared mutable state | Concurrent requests that exercise the invariant. |
| Resource ownership | Success, expected error and early-exit paths. |
| Deployment layout | A built binary run from a directory without the source tree. |

`TestClient` dispatches in process: send the real `Content-Type`, `Accept`
and cookie headers for the flow under test, and keep CSRF enabled. Run tests
from the app directory when templates and fixtures use relative paths. Run
both runtimes and strict checking before a release:

```sh
geblang check --strict src/
geblang test src/
geblang test --runtime=evaluator src/
```

Add real HTTP checks for what an in-process client cannot see: wire headers,
listeners, streamed files and shutdown. The bookmarks example's `verify.sh`
shows a server started on temporary data, probed until ready, and checked as
a built binary. See [Testing](14-testing.md).

## Deploy the artifact you verified

Commit the dependency lockfile and build with the same Geblang and Gebweb
versions you tested. Test the built binary, not only the source: templates and
assets are embedded in it, but databases, session files and uploads are not,
so give it explicit paths for them when it runs from another directory.

Serve with `gebweb.cli` for the standard flags and graceful shutdown, and add
`gebweb.useOps` so readiness reports draining while in-flight requests finish.
Configure trusted proxies for the load balancers that actually forward your
traffic, so client addresses, rate limits and logs are right. Set request-body
and concurrency limits from the workload you expect, and measure latency and
memory on the built binary before tuning the runtime. See
[Deployment](37-deployment.md).

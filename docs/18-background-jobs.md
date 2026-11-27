# Background jobs

<!-- doctest:strict -->

Background jobs move work out of an HTTP request into a durable database queue.
A producer enqueues a JSON payload, then a worker calls the registered
`@Job("name")` handler. SQLite, PostgreSQL and MySQL queues use the language's
versioned job engine; no separate message broker is required.

Delivery is at least once. The queue fences stale claims and records outcomes,
but an external effect can succeed before the worker records completion. Make
handlers idempotent and keep request latency independent of that work.

## A runnable application

This SQLite application accepts a welcome request and records a receipt in a
worker. The receipt's primary key makes repeating that effect harmless.
Controllers and handlers share a repository holding the database connection.

Put this in `src/welcome_app.gb`:

<!-- doctest:file welcome_app.gb -->
```gb
import db;
import gebweb;
import http;

class ReceiptRepository {
    db.Connection conn;

    func ReceiptRepository(db.Connection conn) {
        this.conn = conn;
    }

    func record(string userId, string name): void {
        this.conn.exec(
            "INSERT OR IGNORE INTO receipts (user_id, message) VALUES (?, ?)",
            [userId, "Welcome, " + name]
        );
    }
}

@Tag("job.handler")
class WelcomeHandler {
    ReceiptRepository repository;

    func WelcomeHandler(ReceiptRepository repository) {
        this.repository = repository;
    }

    @Job("welcome", unique: "$payload.userId")
    func send(gebweb.Job job): void {
        this.repository.record(job.payload["userId"] as string, job.payload["name"] as string);
    }
}

class WelcomeInput {
    @Assert.notBlank
    string userId;
    @Assert.notBlank
    string name;
}

class QueuedJob {
    string id;

    func QueuedJob(string id) {
        this.id = id;
    }
}

@Controller("/welcome")
class WelcomeController {
    gebweb.GebwebApp app;

    func WelcomeController(gebweb.GebwebApp app) {
        this.app = app;
    }

    @Post("")
    func create(WelcomeInput body): http.Response {
        string id = gebweb.enqueue(this.app, "welcome", {"userId": body.userId, "name": body.name});
        return http.jsonResponse(QueuedJob(id), 202);
    }
}

export func build(db.Connection conn): gebweb.GebwebApp {
    conn.migrate([
        {
            "id": "001_create_receipts",
            "sql": "CREATE TABLE receipts (user_id TEXT PRIMARY KEY, message TEXT NOT NULL)",
        },
    ]);
    let app = gebweb.app();
    gebweb.useJobs(app, conn, {"pollIntervalMs": 1});
    gebweb.registerInstance(app, ReceiptRepository, ReceiptRepository(conn));
    gebweb.discover(app);
    return app;
}
```

Discovery constructs the `@Tag("job.handler")` service through DI and registers
its decorated methods. It also mounts the `@Controller` routes. Configure jobs
and dependencies before discovery; a worker does not automatically discover
handlers when `runWorker` is called.

The test below sends the same request twice before draining. Both responses
refer to the same active job, and the request itself has not written a receipt:

<!-- doctest:run -->
```gb
import db;
import gebweb;
import gebweb.jobs as jobsmod;
import io;
import welcome_app;

let conn = db.connect("sqlite", ":memory:");
let app = welcome_app.build(conn);
let client = gebweb.TestClient(app);
let first = client.post("/welcome", {"userId": "u-1", "name": "Ada"});
let second = client.post("/welcome", {"userId": "u-1", "name": "Ada"});
io.println(first.status);
io.println(first.json()["id"] == second.json()["id"]);
io.println(conn.queryOne("SELECT COUNT(*) AS total FROM receipts")["total"]);

gebweb.runWorker(app, {"drainOnce": true});
io.println(conn.queryOne("SELECT message FROM receipts WHERE user_id = ?", "u-1")["message"]);
let config = app.jobConfig as jobsmod.JobConfig;
io.println(jobsmod.stats(config)["completed"]);
conn.close();
```

```text
202
true
0
Welcome, Ada
1
```

A successful enqueue commits independently of surrounding application writes.
Do not treat writing an application row and calling `gebweb.enqueue` as a single
transaction. If they must commit together, design an outbox or use a queue
transaction API deliberately rather than assuming the facade joins an open one.

## Queue configuration

Call `gebweb.useJobs(app, conn, opts = {})` once during construction. New
queues initialize versioned `geblang_jobs_*` tables. Producers and workers must
use the same database, namespace and table prefix.

| Option | Default | Effect |
|--------|---------|--------|
| `pollIntervalMs` | `1000` | Positive interval between empty queue polls. |
| `maxAttempts` | `5` | Total attempt ceiling, including the initial attempt. |
| `backoffMs` | `[1000, 5000, 30000, 120000, 600000]` | Retry delays in milliseconds; the last value is reused. |
| `reclaimAfterMs` | `900000` | Renewable claim lease in milliseconds; `0` disables crash recovery with a warning. |
| `namespace` | `"gebweb"` | Logical queue namespace within the tables. |
| `tablePrefix` | `"geblang_jobs"` | SQL table prefix; 1..48 lowercase ASCII letters, digits or underscores. |
| `maxPayloadBytes` | `0` | JSON payload size limit; `0` preserves the unlimited framework default. |
| `autoCreate` | `true` | Initialize missing versioned tables; `false` requires an existing valid schema. |
| `inProcessWorker` | Disabled | `true` or a worker-options dictionary starts an embedded worker when the app listens. |

Use a non-empty list of non-negative retry delays. A populated legacy
`gebweb_jobs` table requires the explicit [maintenance migration](#maintenance-migration)
before `useJobs`; an empty legacy table is ignored.

## Enqueuing and handlers

`gebweb.enqueue(app, name, payload, opts = {})` returns a durable string job ID.
The payload must be a JSON-serializable `dict<string, any>`; validate untrusted
input before passing it into typed application work.

| Enqueue option | Default | Effect |
|----------------|---------|--------|
| `runAt` | Immediately eligible | Unix timestamp in **seconds**, from `0` through `253402300799`; delayed work stays pending until due. |
| `priority` | Handler's priority, or `"default"` | Queue name; ensure a worker selects it. |
| `unique` | Handler's unique template, or none | Active-job deduplication key; `null` explicitly disables the handler default. |

For example, schedule work an hour from now:

<!-- doctest:fragment context=app -->
```gb
import datetime;

gebweb.useJobs(app, conn);
gebweb.enqueue(app, "report", {"reportId": "r-1"}, {
    "runAt": datetime.nowUnix() + 3600,
    "priority": "reports",
});
```

Register the `report` handler before starting its worker. A missing handler
causes a claimed job to fail; enqueueing alone does not validate its existence.

`@Job` handlers receive one `gebweb.Job` argument and return `void`. Normal
return completes the job; an exception triggers retry or terminal failure.
Inherited decorated methods participate in registration. Duplicate job names
are rejected, and handler registration closes when the worker is prepared.

| Job field / method | Meaning |
|--------------------|---------|
| `id: string` | Stable job ID across attempts. |
| `name: string` | Enqueued handler name. |
| `payload: dict<string, any>` | JSON-decoded payload. |
| `attempts: int` | Current attempt number, starting at 1. |
| `queue: string` | Selected priority queue. |
| `app: any` | Owning app; cast when a typed app reference is required. |
| `isCancellationRequested(): bool` | Whether cooperative cancellation was requested. |
| `throwIfCancelled(): void` | Throw when the underlying attempt has been cancelled. |

Constructor injection keeps required dependencies explicit, as in
`WelcomeHandler`. For manual wiring, `gebweb.registerJobHandlers(app,
[instance])` registers an already-constructed instance; do not also discover
that same handler through a job-handler tag.

### Unique jobs

A key is scoped to the namespace and job name. While a matching job is pending
or running, enqueue returns its existing ID without replacing its payload.
After that job reaches a terminal state, a fresh enqueue may create another ID.
Changing priority does not give the same name/key pair a separate reservation.

A literal `unique` string is used verbatim. `"$payload.userId"` takes the
string representation of the top-level `userId` field. A missing field disables
deduplication for that enqueue; it is not nested-path interpolation. Validate
required key fields, and keep the database idempotency guard even when using
deduplication: these mechanisms protect different failure windows.

### Retries and deadlines

| `@Job` argument | Default | Effect |
|-----------------|---------|--------|
| `priority` | `"default"` | Default queue for this handler's enqueues. |
| `unique` | None | Default active-job deduplication template. |
| `retry` | Queue policy | Retry dictionary or callable delay function. |
| `timeoutMs` | `0` | Cooperative execution deadline; `0` disables it. |

A retry dictionary accepts `maxAttempts` (queue default), `baseMs` (default
`1000`) and `backoff`: `"fixed"` returns `baseMs`, `"linear"` returns
`baseMs * attempt`, and `"exponential"` returns `baseMs * 2 ** (attempt - 1)`.
Without a curve it uses the queue's `backoffMs` list. Unknown curves fail
registration. A callable such as `(int attempt) => attempt * 2000` supplies a
delay in milliseconds and retains the queue-wide attempt ceiling. It is
resolved when attempts are claimed, including the final attempt; keep it fast
and free of side effects.

This deterministic example fails once, retries immediately and completes on
attempt 2:

<!-- doctest:run -->
```gb
import db;
import gebweb;
import gebweb.jobs as jobsmod;
import io;

class RetryHandler {
    @Job("refresh", retry: {"maxAttempts": 2, "backoff": "fixed", "baseMs": 0})
    func refresh(gebweb.Job job): void {
        if (job.attempts == 1) {
            throw RuntimeError("temporary failure");
        }
    }
}

let conn = db.connect("sqlite", ":memory:");
let app = gebweb.app();
gebweb.useJobs(app, conn, {"pollIntervalMs": 1});
gebweb.registerJobHandlers(app, [RetryHandler()]);
string id = gebweb.enqueue(app, "refresh", {});
gebweb.runWorker(app, {"drainOnce": true});
let row = jobsmod.get(app.jobConfig as jobsmod.JobConfig, id) as dict<string, any>;
io.println(row["status"]);
io.println(row["attempts"]);
conn.close();
```

```text
completed
2
```

Timeout does not kill a handler. The worker requests cancellation, retains its
slot and renews the lease until the handler stops. Long loops should poll
`isCancellationRequested()` or call `throwIfCancelled()`. Use a supervised
process when hard termination is required. A normal return after a shutdown
cancellation request can still be recorded as completed.

## Running a worker

Use a module entry that constructs the same app and returns
`gebweb.workerCli(app, args)`. It passes worker arguments to the job CLI,
not the HTTP `cli.checkArgs` parser. For the SQLite application above, put this
in `src/welcome_worker.gb`:

<!-- doctest:file welcome_worker.gb -->
```gb
module welcome_worker;
import db;
import gebweb;
import log;
import sys;
import welcome_app;

export func main(list<string> args): int {
    let conn = db.Connection({
        "driver": "sqlite",
        "dsn": sys.getenv("DATABASE_URL") ?? "welcome.db",
        "txLock": "immediate",
    });
    let app = welcome_app.build(conn);
    gebweb.useLogger(app, log.stderr());
    gebweb.onShutdown(app, "database", func(): void {
        conn.close();
    });
    try {
        return gebweb.workerCli(app, args);
    } finally {
        gebweb.shutdown(app);
    }
}
```

The worker sends framework diagnostics to stderr, keeping stdout available for
operator-command results. A finite smoke run on an empty queue returns without
starting a listener:

<!-- doctest:run -->
```gb
import io;
import welcome_worker;

io.println(welcome_worker.main(["--once"]));
```

```text
0
```

The `DATABASE_URL` value above is the app's SQLite connection string, not the
migration CLI's scheme-prefixed URL. Choose and document one convention for
your app; the framework does not automatically translate environment values.

```sh
gebweb worker --entry welcome_worker
gebweb worker --entry welcome_worker --once
gebweb worker --entry welcome_worker --concurrency 4 --queue high --queue default
gebweb worker --entry welcome_worker --max-jobs 100
gebweb worker --entry welcome_worker stats --format json
gebweb worker --entry welcome_worker dlq list --limit 100
```

| Module worker option | Default | Effect |
|----------------------|---------|--------|
| `--queue NAME` | `high`, `default`, `low` | Repeatable queue selection, in priority order. |
| `--job NAME` | All names | Repeatable name filter. |
| `--concurrency N` | `1` | Maximum simultaneous attempts in this process. |
| `--once` | Continuous polling | Drain currently eligible selected work, then return. |
| `--max-jobs N` | Unlimited | Finite drain capped at `N` claimed attempts, including retries. |
| `--scheduler-only` | Worker enabled | Run only the configured scheduler; conflicts with finite limits and job/queue selection. |

Module execution delegates to `geblang worker`. Exit codes are `0` for a clean
run, `2` for invalid arguments/configuration and `1` for infrastructure failure,
incomplete shutdown or terminal failures encountered during a finite run.
`--once` does not wait for future-due jobs or delayed retries. A finite success
is not proof that no delayed or unselected work remains.

File bootstraps (`--entry PATH.gb`, default `src/main.gb`) remain compatible
with `GEBWEB_RUN=worker`. They must select a worker loop explicitly; see
[The CLI](15-cli.md#legacy-file-bootstrap). Prefer module entries for new apps.

### HTTP entry point

Keep the HTTP process separate from worker argument parsing:

<!-- doctest:run file=main.gb -->
```gb
module main;
import db;
import gebweb;
import gebweb.cli as cli;
import sys;
import welcome_app;

export func main(list<string> args): int {
    cli.checkArgs(args, {"name": "welcome"});
    let conn = db.Connection({
        "driver": "sqlite",
        "dsn": sys.getenv("DATABASE_URL") ?? "welcome.db",
        "txLock": "immediate",
    });
    let app = welcome_app.build(conn);
    gebweb.onShutdown(app, "database", func(): void {
        conn.close();
    });
    gebweb.cli(app, {"name": "welcome", "port": 8080});
    return 0;
}
```

`gebweb routes` prints:

```text
METHOD  PATH                          HANDLER
POST    /welcome                      WelcomeController.create
```

Both entry points load the same handlers and queue settings. In production,
configure a suitable poll interval, bound payload sizes and give workers the
same persistent database as the HTTP producers.

### Worker options and priorities

`gebweb.runWorker(app, opts = {})` is useful for tests and explicit in-process
control. Its main options are:

| Option | Default | Effect |
|--------|---------|--------|
| `queues` | `["high", "default", "low"]` | Selected queues in priority order. Unlisted queues are not claimed. |
| `names` | `[]` | Job names to claim; an empty list means all. Overrides `GEBWEB_WORKER_JOBS`. |
| `maxConcurrency` | `1` | Positive overall attempt limit. |
| `queueConcurrency` | `{}` | Positive per-queue limits within the overall limit; keys must be selected queues. |
| `drainOnce` | `true` when `maxJobs` is supplied, otherwise `false` | Return after currently eligible selected work is exhausted. |
| `maxJobs` | No cap | Maximum claimed attempts; waits for admitted work before returning. |
| `workerId` | `<hostname>:<pid>` | Claim owner label. |
| `shutdownTimeoutMs` | `30000` | Worker shutdown budget. |
| `maxStoreFailures` | `10` | Consecutive admission-store failures before the worker fails. |

`pollIntervalMs` and lease/heartbeat settings come from `useJobs`, rather than
worker-option overrides. `GEBWEB_WORKER_JOBS` supplies a comma-separated name
filter only when `names` is absent. The module CLI's `--job` selections override
its environment-derived default.

Queue order is admission priority, not preemption: a newly arrived high-priority
job does not interrupt running low-priority work. This example selects a custom
queue and leaves an unselected queue pending:

<!-- doctest:run -->
```gb
import db;
import gebweb;
import gebweb.jobs as jobsmod;
import io;

class RenderHandler {
    @Job("render")
    func render(gebweb.Job job): void {
        io.println(job.queue);
    }
}

let conn = db.connect("sqlite", ":memory:");
let app = gebweb.app();
gebweb.useJobs(app, conn, {"pollIntervalMs": 1});
gebweb.registerJobHandlers(app, [RenderHandler()]);
gebweb.enqueue(app, "render", {}, {"priority": "reports"});
gebweb.enqueue(app, "render", {}, {"priority": "high"});
gebweb.enqueue(app, "render", {}, {"priority": "other"});
gebweb.runWorker(app, {"drainOnce": true, "queues": ["high", "reports"]});
io.println(jobsmod.stats(app.jobConfig as jobsmod.JobConfig)["pending"]);
conn.close();
```

```text
high
reports
1
```

Concurrent attempts share handler instances. Use local values, the job context,
`store.Store` or a concurrency-safe database service for writes; a plain mutable
handler field is shared state. A per-queue cap applies within one worker, not
across every process. Database pooling, external rate limits and CPU/memory
still bound useful concurrency.

### In-process worker

For a single-process deployment, `useJobs` can configure an embedded worker:

<!-- doctest:fragment context=app -->
```gb
gebweb.useJobs(app, conn, {
    "inProcessWorker": {
        "queues": ["default", "reports"],
        "maxConcurrency": 4,
        "queueConcurrency": {"reports": 1},
    },
});
```

Register handlers before listening. The worker starts when the app first
listens, alongside HTTP handling; constructing a `TestClient` is not a substitute
for starting it. It ticks a configured scheduler and stops admitting work when
`gebweb.shutdown` requests drain. Unexpected loop failures cause a restart
attempt after the queue's `pollIntervalMs`.

With SQLite, use `txLock: "immediate"` for read-then-write transactions that
share the file, or put jobs in a separate database. Give the worker and server
adequate connection capacity. An embedded worker couples their resource use and
shutdown; separate worker processes are easier to scale independently.

### Graceful shutdown and crash recovery

Module workers stop admission on SIGINT/SIGTERM and share a shutdown budget
between scheduler and jobs. `runWorker` installs signal handlers only when
neither `drainOnce` nor `maxJobs` is supplied. Finite test runs leave process
signal handling alone. The CLI forwards termination signals to its child.

A claim carries a fresh token and a renewable lease. Heartbeats extend it while
the handler is alive, including while cancellation is pending. After a crash,
expired claims are recovered using their persisted attempt ceiling and delay;
a stale owner cannot finalize a replacement claim. Size the lease comfortably
above database latency and heartbeat intervals. `reclaimAfterMs: 0` disables
recovery and logs a warning; it does not make external effects exactly once.

## Inspection and dead-letter recovery

Import `gebweb.jobs as jobsmod` for the compatibility inspection helpers:

| Helper | Result |
|--------|--------|
| `jobsmod.get(app.jobConfig as jobsmod.JobConfig, id)` | Row dictionary, or `null` when absent. Includes `status`, `attempts`, `payload_json`, `last_error` and priority. |
| `jobsmod.stats(app.jobConfig as jobsmod.JobConfig)` | Counts keyed by `pending`, `running`, `completed`, `failed` and `cancelled`. |

`payload_json` is encoded JSON, not a decoded handler payload. The
`locked_at` compatibility field reflects the lease-expiry value. For bounded
attempt history, failed-job pagination and retention, use the configured
`JobConfig.engine` API. Avoid unbounded dashboard scans and retain schedule
occurrence records so maintenance does not cause replay.

The module CLI uses that app's configured database and namespace:

```sh
gebweb worker --entry welcome_worker dlq list --limit 100
gebweb worker --entry welcome_worker dlq retry JOB_ID
gebweb worker --entry welcome_worker dlq retry --all
gebweb worker --entry welcome_worker dlq purge JOB_ID
gebweb worker --entry welcome_worker dlq purge --all
```

Retry resets a failed job for another attempt cycle. Purge deletes terminal
failed jobs; it is not a cancellation command for pending or running work.
The legacy `gebweb worker dlq` form without a module entry reads `DATABASE_URL`
directly instead of loading the app, and warns about that mode on stderr.

This script demonstrates a terminal failure and explicit retry without sleeps:

<!-- doctest:run -->
```gb
import db;
import gebweb;
import gebweb.jobs as jobsmod;
import io;

class FailingHandler {
    @Job("fail", retry: {"maxAttempts": 1})
    func fail(gebweb.Job job): void {
        throw RuntimeError("delivery unavailable");
    }
}

let conn = db.connect("sqlite", ":memory:");
let app = gebweb.app();
gebweb.useJobs(app, conn, {"pollIntervalMs": 1});
gebweb.registerJobHandlers(app, [FailingHandler()]);
string id = gebweb.enqueue(app, "fail", {});
gebweb.runWorker(app, {"drainOnce": true});
let config = app.jobConfig as jobsmod.JobConfig;
io.println((jobsmod.get(config, id) as dict<string, any>)["status"]);
io.println(gebweb.workerCli(app, ["dlq", "retry", id]));
io.println((jobsmod.get(config, id) as dict<string, any>)["status"]);
conn.close();
```

```text
failed
{"changed":1}
0
pending
```

## Scheduled tasks

Configure jobs before `gebweb.useScheduler`. A scheduled method takes no
arguments; use constructor injection for its dependencies. Cron expressions
have five numeric fields and run in UTC:

| Field | Range |
|-------|-------|
| Minute | `0..59` |
| Hour | `0..23` |
| Day of month | `1..31` |
| Month | `1..12` |
| Day of week | `0..6`, Sunday `0`; `7` also maps to Sunday. |

Fields accept `*`, an integer, comma-separated values, a range, `*/N` or a
stepped range such as `1-10/2`. Gebweb retains **AND** matching when both day
of month and day of week are restricted: `0 0 1 * 1` means midnight on a
Monday that is also the first of the month.

### Direct scheduling

The default `execution: "direct"` invokes the method in the scheduler. It
coordinates through database leader and occurrence records, logs callback
exceptions and continues, but does not retry or recover a callback lost after
dispatch. This one-tick example uses an always-matching expression:

<!-- doctest:run -->
```gb
import db;
import gebweb;
import io;

class Maintenance {
    @Scheduled("* * * * *")
    func sweep(): void {
        io.println("maintenance");
    }
}

let conn = db.connect("sqlite", ":memory:");
let app = gebweb.app();
gebweb.useJobs(app, conn);
gebweb.useScheduler(app);
gebweb.registerScheduledTasks(app, [Maintenance()]);
gebweb.tickScheduler(app);
conn.close();
```

```text
maintenance
```

### Queued scheduled handlers

Use `execution: "queued"` when scheduled work should run through durable job
claims, retries and worker concurrency. Publication and cursor advancement
commit together; the worker executes the generated scheduled job later.

<!-- doctest:fragment context=app -->
```gb
class Reports {
    @Scheduled("0 * * * *", id: "reports.hourly", priority: "reports", missed: "latest", overlap: "skip")
    func generate(): void {
        io.println("generate hourly report");
    }
}

gebweb.useJobs(app, conn);
gebweb.useScheduler(app, {"execution": "queued"});
gebweb.registerScheduledTasks(app, [Reports()]);
```

Select the `reports` queue in the worker that runs this handler. Alternatively,
mark a service with `@Tag("scheduled")` and let discovery construct/register it
once. Queued scheduling requires the **same connection instance** as the job
queue. Register scheduled handlers before starting either scheduler or worker.

| Scheduler option | Default | Effect |
|------------------|---------|--------|
| `execution` | `"direct"` | Direct callbacks or durable queued publication. |
| `conn` | Job queue's connection | Connection used by scheduling; queued mode must use that exact connection. |
| `tickIntervalMs` | `30000` | Positive interval between checks. |
| `leaseSec` | `60` | Direct scheduler's leader lease in seconds. |
| `workerId` | `<hostname>:<pid>` | Direct scheduler's leader identity. |

| Queued `@Scheduled` argument | Default | Effect |
|------------------------------|---------|--------|
| `id` | Bare `Class.method` | Stable persisted schedule identity; specify an explicit unique ID across modules and class renames. |
| `priority` | `"default"` | Generated job's queue. |
| `retry`, `timeoutMs` | Job defaults | Execution policy for the generated job. |
| `missed` | `"skip"` | `skip`, `latest` or `all` missed-run policy. |
| `overlap` | `"allow"` | `skip` avoids publication while an earlier occurrence remains pending/running. |
| `batchSize` | `100` | Catch-up publication bound per schedule tick. |
| `maxAgeMs` | `86400000` | Catch-up age bound in milliseconds. |

Default schedule IDs do not include the declaring module. Use explicit stable
IDs when classes in different modules can share a name, and keep the ID when
renaming or moving a task.

Changing a queued definition updates its stored schedule at startup, re-enables
it and logs the change. An old instance stops publishing a schedule whose stored
definition no longer matches its own. Finish rolling deployments rather than
leaving old and new definitions active indefinitely. Use a dedicated publisher
when saturated workers could delay schedule checks:

```sh
gebweb worker --entry welcome_worker --scheduler-only
```

That command requires the entry module to configure a scheduler; add
`useScheduler` and scheduled-handler registration to its factory first.

## Maintenance migration

Migration from `gebweb_jobs` is an operator step, not normal request startup.
Stop every old producer, worker and scheduler, and back up the database. Before
calling `useJobs`, inspect a dry-run report:

<!-- doctest:fragment context=app -->
```gb
let report = gebweb.migrateJobs(conn);
io.println(report.total);
io.println(report.invalid);
io.println(report.errors);
```

The source table must exist. Dry run is read-only by default. Resolve invalid
rows and active dedupe conflicts before applying:

<!-- doctest:fragment context=app -->
```gb
let report = gebweb.migrateJobs(conn, {"dryRun": false, "stopped": true});
io.println(report.migrated);
```

| Migration option | Default | Effect |
|------------------|---------|--------|
| `dryRun` | `true` | Validate without copying data. |
| `stopped` | `false` | Must be `true` to acknowledge the stopped-system prerequisite when applying. |
| `namespace` | `"gebweb"` | Target namespace. |
| `tablePrefix` | `"geblang_jobs"` | Target versioned table prefix. |
| `maxAttempts` | `5` | Default recovery ceiling for legacy rows. |
| `policies` | `{}` | Job-name map of engine policies such as `maxAttempts`, `backoffMs` and `timeoutMs`; use concrete delay lists rather than framework retry curves. |

The report exposes `total`, `migrated`, `invalid`, `errors` and `dryRun`.
Migration retains legacy IDs, names, payloads, priorities, attempts and due times,
keeps the source table and resumes interrupted batches. Interrupted running jobs
become pending or failed according to their retry ceiling; historical attempt
records are not invented. Incomplete migrations block queue startup. Never run
old and new workers together. Keep the backup until counts and terminal states
have been checked.

## Reference

| API | Purpose |
|-----|---------|
| `gebweb.useJobs(app, conn, opts = {})` | Configure a durable queue. |
| `gebweb.registerJobHandlers(app, instances)` | Register decorated methods on supplied instances. |
| `gebweb.enqueue(app, name, payload, opts = {})` | Enqueue work or return an active deduplicated ID. |
| `gebweb.runWorker(app, opts = {})` | Run a worker; finite options are useful in tests. |
| `gebweb.workerCli(app, args)` | Worker/operator command surface; returns an exit code. |
| `gebweb.Job` | Handler context, attempt number and cancellation access. |
| `jobsmod.get(config, id)`, `jobsmod.stats(config)` | Compatibility inspection dictionaries. |
| `gebweb.useScheduler(app, opts = {})` | Configure direct or queued scheduling. |
| `gebweb.registerScheduledTasks(app, instances)` | Register decorated no-argument scheduled methods. |
| `gebweb.tickScheduler(app)` | Execute one scheduler check. |
| `gebweb.scheduler.parseCron(text)` | Parse a supported numeric five-field expression. |
| `gebweb.migrateJobs(conn, opts = {})` | Validate or migrate the legacy job table during maintenance. |

Related chapters: [Dependency injection](06a-dependency-injection.md),
[Testing](14-testing.md), [The CLI](15-cli.md),
[Database migrations](17-migrations.md), [Events](19-events.md),
[Message brokers](22-messaging.md), [Concurrency](35-concurrency.md), and
[Deployment](37-deployment.md).

# Background jobs

Gebweb ships a DB-backed background-job runner: producers enqueue
work from request handlers; one or more `gebweb worker` processes
poll versioned queue tables and run the registered
`@Job("name")` handler.

The queue lives in the same database as your application data
(SQLite, Postgres, or MySQL); there is no separate broker to
operate. Failed jobs retry with exponential backoff up to
`maxAttempts`, then move to the `failed` status with `last_error`
captured.

## Wiring

```gb
import gebweb;
import db;

let conn = db.connect("sqlite", "./app.db");
let app = gebweb.app([HomeController()]);
gebweb.useJobs(app, conn);
gebweb.registerJobHandlers(app, [WelcomeMailer()]);
```

`useJobs` delegates persistence and fenced execution to the language `jobs`
engine. Fresh databases initialize versioned `geblang_jobs_*` tables using the
stable namespace `gebweb`. A `gebweb_jobs` table that still holds rows requires
the explicit maintenance migration below; an empty one is ignored. Options:

| Option | Default | Meaning |
|--------|---------|---------|
| `pollIntervalMs` | `1000` | Worker sleep between polls when the queue is empty. |
| `maxAttempts` | `5` | After this many failed attempts a job moves to `failed`. |
| `backoffMs` | `[1000, 5000, 30000, 120000, 600000]` | Delay between retries; falls back to the last entry once exhausted. |
| `reclaimAfterMs` | `900000` | Reclaim a job left `running` by a crashed worker once its lock is this old (see [Crash recovery](#crash-recovery)). `0` disables. |

Pass overrides as a dict: `gebweb.useJobs(app, conn, {"maxAttempts": 10})`.

## Enqueuing

Any code holding the `app` reference can enqueue a job:

```gb
class SignupController {
    @Post("/signup")
    func signup(SignupForm form): dict<string, any> {
        let user = userRepo.create(form);
        gebweb.enqueue(app, "welcome-email", {"to": user.email, "name": user.name});
        return {"id": user.id};
    }
}
```

`gebweb.enqueue(app, name, payload, opts?)` returns the new job
id. `payload` is any JSON-stringifiable dict. Options:

- `runAt`: unix-seconds timestamp for delayed execution.
- `priority`: `"high"`, `"default"`, `"low"`, or any string drain
  order you configure (see [Priorities](#priorities)). Overrides the
  handler's `@Job(priority: ...)` default.
- `unique`: a dedupe key (see [Unique jobs](#unique-jobs)). Overrides
  the handler's `@Job(unique: ...)` default.

## Handlers

Declare a class with one or more methods decorated with
`@Job("name")`. Each method receives a `gebweb.Job` context:

```gb
import gebweb;

class WelcomeMailer {
    @Job("welcome-email")
    func send(gebweb.Job job): void {
        let to = job.payload["to"] as string;
        let name = job.payload["name"] as string;
        mailer.send(to, "Welcome, " + name + "!", "<p>Glad you're here.</p>");
    }
}
```

`Job` fields:

| Field | Type | Description |
|-------|------|-------------|
| `id` | `string` | Stable durable job ID. |
| `name` | `string` | The job name passed to `enqueue`. |
| `payload` | `dict<string, any>` | The enqueued payload, JSON-decoded. |
| `attempts` | `int` | 1-based attempt counter for the current execution. |
| `app` | `any` | The GebwebApp; useful for resolving DI dependencies via `gebweb.resolve(job.app, SomeService)`. |

Throwing from a handler triggers a retry (or final failure once
`maxAttempts` is reached); returning normally marks the job
`completed`.

## Job options

`@Job` takes named arguments that configure a handler's scheduling
and failure behaviour:

```gb
class Mailer {
    @Job("welcome-email", priority: "high", retry: {"maxAttempts": 3, "backoff": "exponential", "baseMs": 1000}, timeoutMs: 30000)
    func send(gebweb.Job job): void { ... }
}
```

| Argument | Meaning |
|----------|---------|
| `priority` | Drain priority for this job (see [Priorities](#priorities)). |
| `unique` | Dedupe key template (see [Unique jobs](#unique-jobs)). |
| `retry` | Per-handler retry policy, overriding the queue defaults. |
| `timeoutMs` | Per-job deadline; cancellation is requested and the attempt is retried once the handler returns. |

Any of these can be set per-enqueue too via the `enqueue` opts
(`priority`, `unique`), which override the handler default.

### Priorities

Each job carries a priority string (`"default"` if unset). The
worker drains all jobs of the highest priority before moving to the
next, so a flood of low-priority work never starves urgent jobs.
Configure the order with `queues` (highest first):

```gb
gebweb.runWorker(app, {"queues": ["high", "default", "low"]});
```

The default order is `["high", "default", "low"]`. A job whose
priority is not in the list is treated as lowest. The CLI worker
takes the same default; pass a custom order from `runWorker` in
`main.gb`.

### Unique jobs

A `unique` key dedupes a job while an equivalent one is still
`pending` or `running`: enqueuing the same key returns the existing
job id instead of inserting a duplicate. Once that job `completed`
or `failed`, a fresh enqueue is allowed again.

The key is a template. A literal string is used verbatim; a
`$payload.<field>` template resolves against the enqueued payload:

```gb
class Reindex {
    /* one reindex per user at a time */
    @Job("reindex", unique: "$payload.userId")
    func run(gebweb.Job job): void { ... }
}

gebweb.enqueue(app, "reindex", {"userId": "42"});
gebweb.enqueue(app, "reindex", {"userId": "42"});   /* same id: deduped */
gebweb.enqueue(app, "reindex", {"userId": "99"});   /* distinct */
```

The guard is enforced at enqueue time; a partial unique index backs
it where the database supports one.

### Per-handler retry

`retry` overrides the queue-wide `maxAttempts` / `backoffMs` for one
handler. As a dict it takes `maxAttempts` plus a `backoff` curve
(`"fixed"`, `"linear"`, or `"exponential"`) over `baseMs`:

```gb
@Job("charge", retry: {"maxAttempts": 5, "backoff": "exponential", "baseMs": 1000})
```

For full control, pass a callable that maps the (1-based) attempt
number to a delay in milliseconds:

```gb
@Job("charge", retry: func(attempt: int): int { return attempt * 2000; })
```

### Per-job timeout

`timeoutMs` requests cooperative cancellation after the execution deadline.
The worker retains its slot and renews the lease until the handler stops; it
does not admit replacement work over a still-running timed-out handler. Poll
`job.isCancellationRequested()` or call `job.throwIfCancelled()` in long loops.
Run a supervised worker process when hard termination is required.

## Running a worker

The `gebweb worker` CLI subcommand re-runs your project's
`src/main.gb` with `GEBWEB_RUN=worker` in the environment. Branch
on it so the same binary serves HTTP and background work:

```gb
import gebweb;
import db;
import sys;

let conn = db.connect("sqlite", "./app.db");
let app = gebweb.app([HomeController()]);
gebweb.useJobs(app, conn);
gebweb.registerJobHandlers(app, [WelcomeMailer()]);

if (sys.getenv("GEBWEB_RUN") == "worker") {
    gebweb.runWorker(app);
} else {
    gebweb.serve(app, "0.0.0.0:3000");
}
```

Run multiple workers in parallel by starting `gebweb worker` in
multiple processes - each row is claimed atomically via a
conditional `UPDATE`, so the same job can't be picked up twice.

### Graceful shutdown

On `SIGTERM` or `SIGINT`, a worker finishes the job it is currently
running and then stops, rather than being killed mid-job. In the
long-running mode `gebweb.runWorker(app)` installs the signal
handlers for you (they call `gebweb.shutdown(app)`, which flips the
drain flag the loop checks between jobs). The `gebweb worker` CLI
forwards termination signals to the worker process, so a container
stop (Docker / Kubernetes `SIGTERM`) drains in-flight work before
exit. A job interrupted before its worker could finish is recovered
by [crash recovery](#crash-recovery).

The signal handlers are installed only in the blocking production
mode; the test helpers (`drainOnce` / `maxJobs`) leave the host
process's signal handling untouched.

### Concurrency within a worker

By default a worker runs one job at a time. Set `maxConcurrency` to
run several at once in a single process:

```gb
gebweb.runWorker(app, {"maxConcurrency": 4});
```

The worker keeps up to that many jobs in flight via a bounded async
pool, claiming more as each finishes. This raises throughput for
IO-bound work (a job waiting on the network or database lets others
run) without starting more processes.

Concurrent jobs share the same handler instance, so handlers run
under `maxConcurrency > 1` **must be concurrency-safe**: they may use
`job.payload`, local variables, and the database freely, but must not
mutate shared in-process state (a handler field holding a dict/list, a
module-level collection). This is the same contract as request
handlers under load. If you are unsure, leave `maxConcurrency` at 1
and scale out with multiple worker processes instead, which keeps each
worker fully isolated.

`maxConcurrency` composes with `maxJobs` and `drainOnce` (both wait
for all in-flight jobs before returning) and with graceful shutdown
(in-flight jobs finish before the worker stops).

### Per-queue concurrency

`queueConcurrency` caps how many jobs from one queue (priority) run at
the same time, within the overall `maxConcurrency`. Use it to keep a
CPU-heavy queue from taking every slot:

```gb
gebweb.runWorker(app, {
    "queues": ["high", "default", "reports"],
    "maxConcurrency": 4,
    "queueConcurrency": {"reports": 1},
});
```

A queue only drains when it is listed in `queues`, so add any custom
priority there. Jobs reach a queue through `@Job(priority: "reports")`
or `gebweb.enqueue(app, name, payload, {"priority": "reports"})`.

### In-process worker

A single-binary deployment can run the worker inside the server process
instead of as a separate `gebweb worker`. Pass `inProcessWorker` to
`useJobs`: `true`, or a dict of the `runWorker` options above (`queues`,
`maxConcurrency`, `queueConcurrency`, `names`, `workerId`):

```gb
gebweb.useJobs(app, conn, {
    "inProcessWorker": {"queues": ["default", "reports"], "maxConcurrency": 4, "queueConcurrency": {"reports": 1}},
});
gebweb.registerJobHandlers(app, [Reports(), Mailers()]);
gebweb.cli(app);
```

The worker starts the first time the app listens (`gebweb.cli`,
`gebweb.serve` or `gebweb.listen`) and runs as a background task
alongside request handling, so its handlers follow the same
concurrency-safe contract as `maxConcurrency > 1`. It ticks a registered
scheduler like a standalone worker. `gebweb.shutdown(app)` (called by
`gebweb.cli` on SIGINT/SIGTERM) stops it claiming new jobs and waits up
to the shutdown timeout for in-flight jobs; a job cut off there is
recovered by [crash recovery](#crash-recovery). A handler that returns
normally after shutdown requested cancellation is recorded as completed. If the worker loop
itself fails (for example the database is briefly unavailable) it logs
the error and restarts after `pollIntervalMs`.

With SQLite, the server's own writes and the worker's writes come from
different connections; open the job connection with
`"txLock": "immediate"` (or keep the queue in its own database file) so
read-then-write transactions wait for each other instead of failing with
`database is locked`.

### Filtering by job name

By default a worker drains every job name. To pin a worker process
to a subset of names (so one server handles email, another handles
image-resize), pass `--job <name>` to the CLI:

    gebweb worker --job email --job sms

The flag is repeatable; the CLI populates `GEBWEB_WORKER_JOBS`
which `gebweb.runWorker(app)` reads automatically. Equivalently
in code:

    gebweb.runWorker(app, {"names": ["email", "sms"]});

A worker pinned to a name list only ever claims rows whose `name`
column appears in the list, leaving other jobs in `pending` for a
differently-scoped worker (or the same worker on a later run) to
process.

## Crash recovery

A claim has a fresh token and a renewable lease. `reclaimAfterMs` (default
900000 ms) sets the lease lifetime. Heartbeats extend it while the handler is
alive, including during cancellation and drain. Choose a lease that comfortably
exceeds database latency and the heartbeat interval. Expired claims are recovered
using their persisted retry ceiling and delay; a stale owner cannot finish or
release the replacement claim. Set `reclaimAfterMs: 0` only to preserve disabled
legacy recovery; the worker emits a warning. At-least-once delivery still needs
idempotent effects outside the queue transaction.

## Dead-letter queue

Jobs that exhaust their retries land in the `failed` status with
`last_error` captured. The `gebweb worker dlq` subcommand inspects
and recovers them. Like `gebweb migrate`, it connects directly via
`$DATABASE_URL` rather than loading your app:

    gebweb worker dlq list                 # show failed jobs
    gebweb worker dlq retry <id> [<id>...] # re-queue specific jobs
    gebweb worker dlq retry --all          # re-queue every failed job
    gebweb worker dlq purge <id> [<id>...] # delete specific jobs
    gebweb worker dlq purge --all          # delete every failed job

`retry` resets a job to `pending` with its attempt counter cleared
and `run_at` in the past, so the next worker picks it up
immediately. `purge` deletes the rows outright. Both report how many
rows they touched; neither affects jobs in any other status.

## Inspection

For ad-hoc dashboards or tests:

- `gebweb.jobs.get(app.jobConfig, id): ?dict<string, any>` -
  fetch a row by id.
- `gebweb.jobs.stats(app.jobConfig): dict<string, int>` - counts
  keyed by status (`pending`, `running`, `completed`, `failed`).

## Test pattern

Use the in-memory SQLite driver and `runWorker` in drain mode:

```gb
let app = gebweb.app([]);
let conn = db.connect("sqlite", ":memory:");
gebweb.useJobs(app, conn);
gebweb.registerJobHandlers(app, [MyHandler()]);
gebweb.enqueue(app, "my-job", {"x": 1});
gebweb.runWorker(app, {"drainOnce": true});
```

`drainOnce: true` processes every pending job - including
follow-ons enqueued by handlers during the drain - and returns
when the queue is empty. `maxJobs: N` is the safety-capped
variant: same drain semantics, but stop after `N` jobs total to
guard against runaway re-enqueueing in a buggy handler.

## Scheduled tasks

The same worker process can run cron-style scheduled tasks.
Multiple worker processes coordinate via leader election on a
`gebweb_scheduler_leader` row, so scaling out doesn't double-fire.

```gb
class Cleanup {
    @Scheduled("0 * * * *")             /* every hour on the hour */
    func sweepStaleSessions(): void {
        db.exec("DELETE FROM sessions WHERE expires_at < ?", datetime.nowUnix());
    }

    @Scheduled("*/15 * * * *")          /* every 15 minutes */
    func refreshLeaderboard(): void {
        leaderboardService.rebuild();
    }
}

gebweb.useJobs(app, conn);
gebweb.useScheduler(app);
gebweb.registerScheduledTasks(app, [Cleanup()]);
```

Cron grammar (POSIX five-field subset): `minute (0-59)`,
`hour (0-23)`, `day-of-month (1-31)`, `month (1-12)`,
`day-of-week (0-6, Sun=0; 7 also accepted as Sunday)`.

Each field accepts: `*`, `N`, `N,M,O`, `N-M`, `*/N`, `M-N/S`.

When both day-of-month and day-of-week are restricted they are
AND-ed (so `0 0 1 * 1` means "midnight on the 1st only if that
day is Monday"). This differs from Vixie cron's OR semantics; the
AND behaviour matches what users expect more often.

`gebweb.useScheduler(app, opts = {})` options:

| Option | Default | Meaning |
|--------|---------|---------|
| `tickIntervalMs` | `30000` | Maximum delay between scheduler checks. The worker also ticks the scheduler between job polls. |
| `leaseSec` | `60` | How long a leader's claim remains valid. A leader refreshes its lease on every tick; if a leader dies, another worker takes over `leaseSec` later. |
| `workerId` | `<hostname>:<pid>` | Tag for the `gebweb_scheduler_leader` row; usually default. |
| `conn` | the job queue's connection | Override the DB connection used for leader election. |

Scheduler exceptions are logged to stderr and swallowed - one
broken task does not stop the rest. For at-most-once delivery
under leader-election semantics, prefer enqueueing a regular
`@Job` from the scheduled task body (the job retry machinery
takes over from there).

## Reference

| Helper | Purpose |
|--------|---------|
| `gebweb.useJobs(app, conn, opts = {})` | Attach a job queue to the app. |
| `gebweb.registerJobHandlers(app, instances)` | Discover `@Job` handlers on each instance. |
| `gebweb.enqueue(app, name, payload, opts = {})` | Insert a job row. Returns the new id. |
| `gebweb.runWorker(app, opts = {})` | Block running jobs (and ticking the scheduler when configured). `opts.maxJobs` for tests. |
| `gebweb.Job` | Handler context (id, name, payload, attempts, app). |
| `gebweb.jobs.get(config, id)` | Fetch a row by id (mostly for tests / dashboards). |
| `gebweb.jobs.stats(config)` | Count rows by status. |
| `gebweb.useScheduler(app, opts = {})` | Attach the scheduler. |
| `gebweb.registerScheduledTasks(app, instances)` | Discover `@Scheduled` handlers. |
| `gebweb.tickScheduler(app)` | Run one scheduler tick directly (tests). |
| `gebweb.scheduler.parseCron(text)` | Parse a cron expression to a `CronExpr`. |

## Versioned engine and module workers

The framework preserves `@Job`, `enqueue`, retry decorators, priority queues and
legacy result dictionaries. It delegates claim, renewal, recovery and history to
the language queue engine. New framework queues preserve the legacy unlimited
payload setting; set a payload limit explicitly when constructing a core queue.
A worker renews its lease throughout long handlers. `reclaimAfterMs` now sets the
lease duration; 0 preserves the legacy disabled-recovery setting and emits a
warning. The default remains 900000 ms for compatibility.

Handlers are registered before startup. Inherited decorated methods participate
in discovery, duplicate job names fail startup, and controllers/services remain
shared instances. Use a Store or database for shared mutable state.

Use a module that exports `main(list<string> args): int`, constructs the app,
registers its handlers and returns `gebweb.workerCli(app, args)`. This uses the
application's configured connection and namespace:

```sh
gebweb worker --entry app.worker --once
gebweb worker --entry app.worker --concurrency 4 --queue high --queue default
gebweb worker --entry app.worker --max-jobs 100
gebweb worker --entry app.worker --scheduler-only
gebweb worker --entry app.worker stats --format json
gebweb worker --entry app.worker dlq list --limit 100
gebweb worker --entry app.worker dlq retry JOB_ID
gebweb worker --entry app.worker dlq purge --all
```

The CLI delegates module execution to `geblang worker`. File entries and the
`GEBWEB_RUN=worker` bootstrap remain available for existing applications. Bare
`gebweb worker dlq` uses the legacy DATABASE_URL mode and says so on stderr; use the
module entry point above for the configured queue. Exit codes are 0 clean,
2 invalid arguments/configuration and 1 infrastructure failure, incomplete drain
or terminal failures in finite mode.

SIGINT/SIGTERM stop admission. Scheduler and worker share one shutdown budget.
An embedded handler calling app shutdown requests drain without joining itself.
Timeout is cooperative: cancellation keeps the slot and lease until the handler
returns. `gebweb.Job.isCancellationRequested()` and `throwIfCancelled()` expose
the context. Use supervised processes when a handler needs a hard stop.
External effects must be idempotent because execution is at least once.

## Queued scheduled handlers

Configure jobs before enabling queued scheduling:

```gb
gebweb.useJobs(app, conn);
gebweb.useScheduler(app, {"execution": "queued"});
gebweb.discover(app);
```

Queued `@Scheduled` handlers become durable named jobs. Publication and cursor
advancement commit together, and workers execute/retry them. Legacy direct
scheduling remains available for compatibility; it is best effort and does not
promise recovery of callbacks lost after dispatch. Both paths retain the legacy
AND behavior when cron day-of-month and day-of-week are restricted. New language
schedules default to OR. Queued jobs and scheduler must use the same connection.
Run a dedicated `--scheduler-only` publisher when worker saturation matters.

Changing a queued handler's cron expression or options updates the stored
schedule at the next startup, re-enables it, and logs the change. An instance
still running the old definition stops publishing that schedule, so finish a
rolling deploy rather than leaving both versions running. An unknown `retry`
backoff strategy is rejected when handlers are registered.

## Maintenance migration

Stop every old producer, worker and scheduler and back up the database. Before
calling `useJobs`, run:

```gb
let report = gebweb.migrateJobs(conn);
```

The default is a read-only dry run. Inspect `total`, `invalid` and `errors`.
After fixing validation problems, apply with
`gebweb.migrateJobs(conn, {"dryRun": false, "stopped": true})`. The migration
copies legacy IDs, names, JSON, priorities, retry counts and due times, retains
the source table, and resumes interrupted batches. Interrupted running jobs are
made pending or failed using their retry ceiling; historical attempts are not
invented. Supply per-handler `policies` if ceilings differ from the default.
Incomplete migrations block startup. Do not run old and new workers together.
Keep the backup until counts, payloads and terminal states have been checked.

The underlying `JobConfig.engine` provides bounded failed-job and attempt
pagination, explicit retry, terminal purge and age/count pruning. Retention is
operator managed. Preserve schedule occurrence records to prevent replay.

# Background jobs

Gebweb ships a DB-backed background-job runner: producers enqueue
work from request handlers; one or more `gebweb worker` processes
poll a shared `gebweb_jobs` table and run the registered
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

`useJobs` auto-creates the `gebweb_jobs` table if it doesn't
exist. Options:

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
| `id` | `string` | Row id from `gebweb_jobs.id`. |
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
| `timeoutMs` | Per-job deadline; a slower run is released for retry. |

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

`timeoutMs` bounds how long a single execution may run. Past the
deadline the worker stops waiting and releases the job's claim so it
retries (or fails once attempts are exhausted). The timeout is
cooperative: the handler's own in-flight work is not forcibly
interrupted, it finishes in the background while the worker moves on.

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

When a worker claims a job it sets the row to `running` and stamps a lock. If
that worker crashes mid-job, the row would otherwise stay `running` forever. The
worker periodically reclaims any job whose lock is older than `reclaimAfterMs`
(default 15 minutes), returning it to `pending` so a healthy worker picks it up.

Set `reclaimAfterMs` longer than your slowest job (and any per-job `timeoutMs`);
otherwise a job that is still legitimately running could be reclaimed and run a
second time. Set it to `0` to disable reclaiming. A reclaimed job whose attempts
have already reached `maxAttempts` is sent straight to the dead-letter queue
rather than retried, so a job that keeps crashing its worker cannot loop forever.

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

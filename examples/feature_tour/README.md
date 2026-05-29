# Feature tour

A small task-manager app that exercises the async, data, and
integration features in one place: API-key auth, per-permission
gating, cursor pagination, query DSL, background jobs, scheduled
tasks, in-process event bus, mailer, file storage, server-side
views, and OpenAPI 3.1.

## Run

HTTP server on port 8080:

```sh
cd examples/feature_tour
geblang main.gb
```

Background worker (separate process; runs scheduled cleanup and
any enqueued jobs):

```sh
cd examples/feature_tour
gebweb worker
```

Tests:

```sh
cd examples/feature_tour
geblang test tests/
```

## What this exercises

| Feature                       | Where                                                                |
|-------------------------------|----------------------------------------------------------------------|
| `gebweb.useApiKeyAuth`        | `main.gb`, `tests/tasks_test.gb` (X-API-Key header)                  |
| `@RequiresPermission`         | `TaskController.delete` requires `tasks.delete`                      |
| Cursor pagination             | `TaskController.list` returns `nextCursor`                           |
| Query DSL                     | `TaskRepo.listOwned`, `TaskRepo.purgeOldCompleted`                   |
| Background jobs + worker      | `TaskNotificationJob` enqueued from the event subscriber             |
| Scheduled tasks               | `TaskCleanup.nightlyPurge` at `0 3 * * *`                            |
| Event bus                     | `gebweb.publish(app, "task.completed", ...)` + `@On`                 |
| Mailer                        | `TaskCompletedMail` rendered and delivered                           |
| Storage                       | `AttachmentController.upload` calls `gebweb.put` via `saveToStorage` |
| Views templating              | `DashboardController.index` renders `dashboard.html`                 |
| OpenAPI 3.1                   | auto-generated; visit `/docs`                                        |

## Layout

```
feature_tour/
    geblang.yaml
    README.md
    main.gb                       # wiring (HTTP + worker branch)
    src/
        app.gb                    # controllers, repo, models
        handlers.gb               # event subscriber, job, scheduled task
    templates/
        dashboard.html            # views.gb template
        emails/task-completed.html
    tests/
        tasks_test.gb             # TestClient-driven tests
```

## Try it manually

```sh
# Create a task
curl -X POST localhost:8080/tasks \
     -H 'X-API-Key: ada-key' \
     -H 'Content-Type: application/json' \
     -d '{"title": "Read the manual"}'

# List
curl localhost:8080/tasks -H 'X-API-Key: ada-key'

# Mark complete (fires task.completed event)
curl -X POST localhost:8080/tasks/<id>/complete -H 'X-API-Key: ada-key'

# Dashboard (HTML)
curl localhost:8080/dashboard -H 'X-API-Key: ada-key'

# Attach a file
echo "notes" > /tmp/notes.txt
curl -X POST localhost:8080/tasks/<id>/attachment \
     -H 'X-API-Key: ada-key' \
     -F file=@/tmp/notes.txt
```

## Notes

- The app uses an in-memory SQLite database; restart wipes state.
  Swap to a file-backed `db.connect("sqlite", "./app.db")` for
  persistence.
- Storage uses `gebweb.memoryStorage()`; swap to
  `gebweb.localStorage("/srv/uploads").withUrlPrefix("/uploads")`
  for disk persistence.
- The mailer uses `gebweb.logMailer()`; swap to
  `gebweb.smtpMailer({...})` for real delivery (see the mailer
  chapter in `docs/`).

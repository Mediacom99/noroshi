# Noroshi — Design Document

Technical reference for architecture decisions and implementation patterns. For user-facing docs see README.md.

## Project Structure

```
noroshi/
├── cmd/
│   └── monitor/
│       └── main.go                      # Entrypoint: wiring, signal handling, health server
├── internal/
│   ├── apperror/
│   │   ├── apperror.go                  # AppError type, sentinels, Wrap helper
│   │   └── apperror_test.go
│   ├── config/
│   │   ├── config.go                    # Config struct, Load() from env vars
│   │   └── config_test.go
│   ├── bot/
│   │   ├── bot.go                       # Bot struct, Store/Scheduler/Checker interfaces, TelegramNotifier
│   │   ├── handlers.go                  # Command handlers (/add, /delete, /list, /status, /interval, /help)
│   │   ├── callbacks.go                 # Inline keyboard callbacks (detail, delete, interval, refresh)
│   │   ├── format.go                    # Message formatting + callback unique IDs
│   │   ├── validate.go                  # Name/URL validation
│   │   └── *_test.go                    # Handler, callback, format, validate tests + mocks
│   ├── monitor/
│   │   ├── checker.go                   # retryablehttp-based health checker (returns code + latency)
│   │   ├── scheduler.go                 # gocron scheduler, checkAndNotify, CheckNow
│   │   └── *_test.go
│   └── storage/
│       ├── migrations/                  # goose migrations 001–006
│       ├── models.go                    # Endpoint struct
│       ├── store.go                     # OpenDB, RunMigrations, SQLiteStore
│       └── store_test.go
├── .github/workflows/                   # ci.yml (lint, build, vet, test), release.yml (GHCR)
├── Dockerfile                           # Multi-stage, non-root, HEALTHCHECK on ${HEALTH_PORT:-8080}
├── docker-compose.yml
├── entrypoint.sh                        # Volume permission fix + privilege drop
└── .golangci.yml                        # bodyclose, contextcheck, errorlint, sloglint, sqlclosecheck
```

## Startup Flow

1. `config.Load()` — read env vars, validate, return `Config`.
2. `storage.OpenDB(path)` — SQLite with `_journal_mode=WAL&_busy_timeout=5000`.
3. `storage.RunMigrations(db)` — `goose.Up` over embedded migrations.
4. `storage.NewSQLiteStore(db)`.
5. `monitor.NewHTTPChecker(cfg.HTTPTimeout)`.
6. `bot.NewBot(token, chatID, store, checker, ctx)` — no scheduler yet (circular dependency, see below).
7. `bot.NewTelegramNotifier(bot, maxFailureNotifications)` — implements `monitor.Notifier`.
8. `monitor.NewScheduler(ctx, store, checker, notifier, maxFailureNotifications)`.
9. `bot.SetScheduler(scheduler)` — closes the circular dependency.
10. Load all endpoints from DB → `scheduler.Add(ctx, ep)` for each.
11. `scheduler.Start()`, `bot.Start()` (goroutine), health server on `HEALTH_PORT` (goroutine).
12. Block on `<-ctx.Done()`; graceful shutdown: `bot.Stop()` → `scheduler.Shutdown()` (waits for running jobs) → health server `Shutdown()` → `db.Close()`.

## Circular Dependency Resolution

The bot needs the scheduler (add/remove jobs, `CheckNow`), and the scheduler needs the notifier (which wraps the bot). Resolution: create bot first, then notifier from bot, then scheduler with notifier, finally `bot.SetScheduler(scheduler)`. Bot handlers nil-check the scheduler before use.

## Context & Shutdown

- `signal.NotifyContext(context.Background(), SIGINT, SIGTERM)` in `main.go` is the ONLY `context.Background()` outside tests and the health-server shutdown.
- The root context is stored on `Bot` (`rootCtx`) and `Scheduler` (`ctx`) and flows into all store/HTTP calls.

## gocron Scheduler Pattern

```go
s, _ := gocron.NewScheduler()
s.Start() // non-blocking

job, _ := s.NewJob(
    gocron.DurationJob(time.Duration(ep.IntervalSeconds) * time.Second),
    gocron.NewTask(s.checkAndNotify, ep.ID),
    gocron.WithTags(fmt.Sprintf("endpoint-%d", ep.ID)),
    gocron.WithStartAt(gocron.WithStartImmediately()),   // first check right away
    gocron.WithSingletonMode(gocron.LimitModeReschedule), // never overlap checks for one endpoint
)

s.RemoveByTags("endpoint-1") // remove by tag
s.Shutdown()                 // blocks until running jobs finish
```

- **Singleton mode is mandatory**: a check slower than the interval must never overlap the next run — concurrent runs would race on the failure counters.
- **DO NOT** hand-roll scheduling with goroutines/`time.Ticker`/`sync.Mutex`.

## Checker

retryablehttp with `RetryMax=2`, `RetryWaitMin=500ms`, `RetryWaitMax=2s`, per-request timeout from config, default retry policy (connection errors + 5xx, never 4xx), and `PassthroughErrorHandler` so the last response is returned after retries are exhausted. `Check` returns `(statusCode, latency, error)` and drains the response body for keep-alive reuse.

**Success rule: any HTTP 2xx is UP.** Everything else — 3xx, 4xx, 5xx, connection error — is DOWN.

## Database Schema (after migrations 001–006)

```sql
CREATE TABLE endpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    url TEXT NOT NULL UNIQUE,
    interval_seconds INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'unknown',   -- 'unknown' | 'ok' | 'not_ok'
    last_checked_at DATETIME,
    last_failure_at DATETIME,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    failure_notifications_sent INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_status_code INTEGER NOT NULL DEFAULT 0,   -- 003
    last_latency_ms INTEGER NOT NULL DEFAULT 0,    -- 004
    paused INTEGER NOT NULL DEFAULT 0,             -- 005
    last_notified_at DATETIME,                     -- 006: drives REMINDER_INTERVAL re-alerts
    paused_until DATETIME,                         -- 006: auto-resume time for timed pauses
    alert_message_id INTEGER NOT NULL DEFAULT 0    -- 006: Telegram alert to thread recovery to
);
```

- `name` and `url` are both UNIQUE.
- `paused` endpoints keep their row and config but have no gocron job; they are skipped at startup, in `checkAndNotify`, and in `CheckNow`. A timed pause sets `paused_until`; a per-minute gocron housekeeping job (`resumeExpiredPauses`) resumes expired pauses.
- `failure_notifications_sent` only counts failures at or beyond `FAILURE_THRESHOLD` and is capped at `MAX_FAILURE_NOTIFICATIONS` by `RecordFailure` — it always reflects notifications actually sent.
- `last_failure_at` is set on the first failure of an outage and cleared on recovery; `RecordRecovery` returns the endpoint with the pre-reset value so downtime can be computed.
- Connection string: `file:<path>?_journal_mode=WAL&_busy_timeout=5000`. Migrations embedded via `//go:embed migrations/*.sql` — never inline `CREATE TABLE` in Go.

## Store Interface

Defined at the point of use. The scheduler consumes the narrowest set:

```go
type Store interface { // internal/monitor/scheduler.go
    GetEndpoint(ctx context.Context, id int64) (storage.Endpoint, error)
    UpdateEndpointStatus(ctx context.Context, id int64, status string, statusCode int, latencyMs int64) error
    RecordFailure(ctx context.Context, id int64, statusCode int, latencyMs int64, maxNotifications int) (storage.Endpoint, error)
    RecordRecovery(ctx context.Context, id int64, statusCode int, latencyMs int64) (storage.Endpoint, error)
}
```

The bot consumes Add/Get/GetByURL/GetByName/Delete/List/UpdateEndpointInterval (`internal/bot/bot.go`). `storage.SQLiteStore` implements both implicitly.

## Notification Behavior (scheduled checks — `checkAndNotify`)

1. Check: `statusCode, latency, err := checker.Check(ctx, url)`.
2. **DOWN** (error or non-2xx):
   - `RecordFailure` → increments `consecutive_failures`; increments `failure_notifications_sent` only once `consecutive_failures >= FAILURE_THRESHOLD`, capped at `MAX_FAILURE_NOTIFICATIONS`; sets `last_failure_at` on first failure.
   - Notify only when the counter actually increased (threshold reached, cap not yet hit). Alerting failures log at Info; sub-threshold/capped failures log at Debug.
   - The alert's Telegram message ID is stored (`alert_message_id`); the recovery notification is sent as a reply to it (threaded alerts).
   - If `REMINDER_INTERVAL` > 0 and the cap is reached but the outage continues, a reminder alert is re-sent once per interval (tracked via `last_notified_at`).
3. **UP, previously down** (`status` not `ok`/`unknown`):
   - `RecordRecovery` → resets counters, returns old `last_failure_at`.
   - Send recovery notification with downtime **only if `last_failure_at` was set** — a `not_ok` status without it comes from an ad-hoc probe, not a tracked outage.
4. **UP, already up**: `UpdateEndpointStatus` (status, code, latency, `last_checked_at`).

## Ad-hoc Checks (`CheckNow`, used by `/status`)

Performs a check and updates status/code/latency, but deliberately does NOT touch failure counters and never notifies. Exception: a DOWN→UP transition calls `RecordRecovery` to clear any tracked outage state. The scheduled jobs own the failure/recovery state machine. Paused endpoints are never checked, not even on demand.

`/add` also runs one immediate check (via the bot's `Checker`) purely to include the result in the confirmation reply; the job's `WithStartImmediately` run persists the authoritative state. A "🔍 Check now" inline button in the detail view exposes `CheckNow` per endpoint.

## Telegram Commands

| Command | Behavior |
|---------|----------|
| `/add <name> <url> [interval]` | Validate name (1–50 chars, `[A-Za-z0-9_-]`, not all-numeric, no leading/trailing `-`) and URL (http/https, dotted host). Interval ≥ 10s, default `1m`. `ErrDuplicate` → friendly message. On scheduler failure → warning reply (monitored after restart). Reply includes immediate first-check result. |
| `/delete <name or id>` | Lookup by ID → name → URL. Remove job, then delete row. |
| `/interval <name or id> <interval>` | `updateInterval` helper: update DB, then remove+re-add job; on job failure roll back DB and restore the old job. |
| `/pause <name or id> [duration]` / `/resume <name or id>` | `setPaused` helper: persist flag (+ optional `paused_until`), remove/add job; resume failure rolls the flag back. Also available as an inline button in the detail view. |
| `/list` | Summary + per-endpoint lines, inline buttons (detail → interval presets / confirmed delete, refresh). |
| `/status` | Concurrent `CheckNow` for all endpoints; reply with HTTP code + latency per endpoint. |
| `/help` | Static help text. |

All user-provided content is HTML-escaped; messages use `ParseMode: HTML`. The `guarded` middleware drops updates from any chat other than `TELEGRAM_CHAT_ID`.

## Error Handling

`internal/apperror`: `AppError{Code, Message, Cause}` implementing `Error`, `Unwrap`, `Is` (compares `Code`). Sentinels: `ErrNotFound`, `ErrDuplicate`, `ErrInvalidInput`, `ErrDatabase`. `Wrap(sentinel, cause)` clones. Always `errors.Is`/`errors.As` — never string matching (the single exception: `isUniqueViolation` detecting SQLite UNIQUE errors by message, wrapped into `ErrDuplicate` at the storage boundary).

## Logging

`slog` text handler to stderr, level from `LOG_LEVEL`. Conventions: short lowercase action phrases (`"scheduler: record failure"`), entity context via `"id"`/`"name"`/`"url"`, errors under `"error"`. State transitions log at Info with `status_code`, `duration_ms`, and `downtime`.

## Health Endpoint

`GET /healthz` on `HEALTH_PORT` → `200 {"status":"ok"}`. Server has `ReadHeaderTimeout: 5s`. Docker `HEALTHCHECK` curls `http://localhost:${HEALTH_PORT:-8080}/healthz`.

## Docker & Release

- Dockerfile: `golang:1.26.1-alpine` builder (`CGO_ENABLED=0`, `-ldflags="-s -w"`), `alpine:3.21` runtime with `ca-certificates`, `tzdata`, `curl`, `su-exec`, non-root `appuser`, `/app/data` volume.
- `entrypoint.sh`: chowns `/app/data` and drops to `appuser` via `su-exec` when running as root; runs the binary directly otherwise (supports `docker run --user`).
- Release workflow: pushing a `v*` tag builds `linux/amd64` + `linux/arm64` images and pushes them to `ghcr.io/mediacom99/noroshi` with semver and `latest` tags.

## Edge Cases

- **Duplicate name or URL on /add** → `ErrDuplicate` → friendly message.
- **All-numeric names rejected** — they'd be ambiguous with IDs in command arguments.
- **Interval < 10s rejected** (handlers, not the scheduler).
- **Container restart** → all endpoints reloaded from DB and re-scheduled; failure state persists.
- **Context cancellation mid-check** → retryablehttp respects context, checks abort.
- **Scheduler re-add failure on interval change** → DB rolled back, old job restored.
- **Scheduler add failure on /add** → endpoint persisted, user warned, monitored after restart.

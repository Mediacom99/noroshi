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
│   ├── api/
│   │   ├── api.go                       # Dashboard JSON API: Server, auth + CORS middleware
│   │   ├── handlers.go                  # /api/endpoints CRUD, pause/resume, check-now, incidents, checks
│   │   ├── types.go                     # JSON DTOs, error mapping
│   │   └── api_test.go                  # Handler tests with mock Store/Scheduler
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
│   │   ├── checker.go                   # Scheme-dispatching checker: retryablehttp for HTTP, net probes for tcp/dns/ping
│   │   ├── digest.go                    # Uptime digest: DigestConfig, BuildDigest, FormatDigest
│   │   ├── webhook_notifier.go          # WebhookNotifier (generic JSON alert channel) + MultiNotifier fan-out
│   │   ├── scheduler.go                 # gocron scheduler, checkAndNotify, CheckNow, digest job
│   │   └── *_test.go
│   └── storage/
│       ├── migrations/                  # goose migrations 001–008
│       ├── models.go                    # Endpoint struct
│       ├── store.go                     # OpenDB, RunMigrations, SQLiteStore
│       └── store_test.go
├── .github/workflows/                   # ci.yml (lint, build, vet, test), release.yml (GHCR)
├── web/                                 # Optional React dashboard (Vite + TS + Tailwind + TanStack)
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
6. `bot.NewBot(token, chatID, store, checker, slowThresholdMs, webhookCfg, ctx, logger)` — no scheduler yet (circular dependency, see below). The update poller is chosen by `choosePoller`: `tele.LongPoller` by default, `tele.Webhook` (listening on `TELEGRAM_WEBHOOK_PORT`, registered against `TELEGRAM_WEBHOOK_URL`, optional secret-token verification) when `TELEGRAM_WEBHOOK_URL` is set. `Bot.Stop` deregisters the webhook in webhook mode.
7. `bot.NewTelegramNotifier(bot, maxFailureNotifications)` — implements `monitor.Notifier`. When `ALERT_WEBHOOK_URL` is set, a `monitor.WebhookNotifier` is appended and both are wrapped in `monitor.NewMultiNotifier` (Telegram always first).
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

The checker dispatches on the URL scheme, so one `Check(ctx, url, opts)` serves every check type and no schema column is needed for the type:

- `http`/`https` — retryablehttp with `RetryMax=2`, `RetryWaitMin=500ms`, `RetryWaitMax=2s`, per-request timeout from config, default retry policy (connection errors + 5xx, never 4xx), and `PassthroughErrorHandler` so the last response is returned after retries are exhausted.
- `tcp://host:port` — `net.Dialer` with the configured timeout; a successful dial is up.
- `dns://host` — `net.DefaultResolver.LookupHost`; at least one record is up.
- `ping://host` — unprivileged ICMP echo via `golang.org/x/net/icmp` (`udp4` datagram socket). Requires `net.ipv4.ping_group_range` to permit it; otherwise the check reports "ping not permitted".

`Check` returns `(statusCode, latency, error)` and drains the response body for keep-alive reuse. Non-HTTP checks leave `StatusCode` and `CertExpiry` zero; formatters only render the HTTP code when it is > 0.

**Success rule (HTTP):** by default any HTTP 2xx is UP. Per endpoint, `/expect` can require an exact status and `/keyword` a body check (first 1 MiB). Keyword specs carry their mode as a prefix: plain text = must contain, `!` = must not contain, `re:` = regexp must match, `!re:` = regexp must not match. Regexes are validated at `/keyword` time (`ValidateKeywordSpec`) and compiled per check; an invalid stored regex reports DOWN with reason "invalid regex". Everything else — 3xx, 4xx, 5xx, wrong status, failed body check, connection error — is DOWN, with the reason persisted in `last_check_error`. `/expect` and `/keyword` apply to HTTP checks only; TCP/DNS/ping checks have their own fixed success rules.

`Check(ctx, url, CheckOptions) CheckResult` returns up/down, status, latency, failure reason, and TLS certificate expiry. When a cert expires in < 14 days, the scheduler sends a warning at most once per 24h (`last_cert_warning_at`).

## Database Schema (after migrations 001–010)

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
    alert_message_id INTEGER NOT NULL DEFAULT 0,   -- 006: Telegram alert to thread recovery to
    expected_status INTEGER NOT NULL DEFAULT 0,    -- 007: exact status required; 0 = any 2xx
    expected_keyword TEXT NOT NULL DEFAULT '',     -- 007: required response substring
    last_check_error TEXT NOT NULL DEFAULT '',     -- 007: human-readable failure reason
    cert_expires_at DATETIME,                      -- 007: TLS cert expiry (https only)
    last_cert_warning_at DATETIME                  -- 007: cert-warning throttle
);

-- 008: per-check history powering /uptime, /incidents, and badges
CREATE TABLE checks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    endpoint_id INTEGER NOT NULL REFERENCES endpoints(id) ON DELETE CASCADE,
    up INTEGER NOT NULL,
    status_code INTEGER NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_checks_endpoint_time ON checks (endpoint_id, checked_at);

-- 010: recurring maintenance windows (checks skipped while active)
CREATE TABLE maintenance_windows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    endpoint_id INTEGER REFERENCES endpoints(id) ON DELETE CASCADE,  -- NULL = all endpoints
    days TEXT NOT NULL,           -- 'all' or comma day codes: mon,tue,...
    start_minutes INTEGER NOT NULL,  -- minutes since midnight UTC
    end_minutes INTEGER NOT NULL     -- end < start = overnight window
);
```

- `name` and `url` are both UNIQUE.
- Every check (scheduled or ad-hoc) appends one `checks` row. An hourly housekeeping job prunes rows older than 30 days. SQLite compares DATETIMEs as strings — all stored times and query parameters are normalized to UTC.
- `paused` endpoints keep their row and config but have no gocron job; they are skipped at startup, in `checkAndNotify`, and in `CheckNow`. A timed pause sets `paused_until`; a per-minute gocron housekeeping job (`resumeExpiredPauses`) resumes expired pauses.
- **Maintenance windows**: `checkAndNotify` calls `IsInMaintenance(endpointID, now)` right after the paused check and skips the check entirely inside a window (no request, no history row, no alert). Window matching lives in `MaintenanceWindow.Applies` (UTC, end-exclusive, overnight windows belong to their start day). A failed window lookup logs an error and the check proceeds — a broken lookup must not silence monitoring. Foreign keys are not enforced in SQLite here (no `foreign_keys` pragma), so `DeleteEndpoint` removes the endpoint's maintenance windows explicitly.
- `failure_notifications_sent` only counts failures at or beyond `FAILURE_THRESHOLD` and is capped at `MAX_FAILURE_NOTIFICATIONS` by `RecordFailure` — it always reflects notifications actually sent.
- `last_failure_at` is set on the first failure of an outage and cleared on recovery; `RecordRecovery` returns the endpoint with the pre-reset value so downtime can be computed.
- Connection string: `file:<path>?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)` — modernc.org/sqlite only honors `_pragma` params (the mattn-style `_journal_mode`/`_busy_timeout` are silently ignored). `OpenDB` sets `SetMaxOpenConns(1)` to serialize in-process writers. Migrations embedded via `//go:embed migrations/*.sql` — never inline `CREATE TABLE` in Go.

## Digest

When `DIGEST` is `daily` or `weekly`, `NewScheduler` registers a gocron `CronJob` (`M H * * *`, or `M H * * 1` for weekly on Mondays, UTC from `DIGEST_TIME`). The job calls `BuildDigest(store, window)` — paused endpoints are excluded, an empty active list sends nothing — and sends the result via `Notifier.NotifyDigest` (silent message). The same `BuildDigest`/`FormatDigest` pair backs the on-demand `/digest` command (always 24h), so both render identically. The formatter lives in `internal/monitor/digest.go` (bot imports monitor, not vice versa) and HTML-escapes names with `html.EscapeString`.

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

## Notifiers

`monitor.Notifier` has four methods: `NotifyFailure` (returns a message ID for threading), `NotifyRecovery`, `NotifyCertExpiry`, `NotifyDigest`. Implementations:

- `bot.TelegramNotifier` — the primary channel; returns the real Telegram message ID.
- `monitor.WebhookNotifier` (`webhook_notifier.go`) — POSTs JSON events to `ALERT_WEBHOOK_URL` (one POST, 10s timeout, no retries, optional Bearer token). Returns message ID 0.
- `monitor.MultiNotifier` (same file) — fans out to all configured notifiers. Individual failures are logged and tolerated; an error is returned only when EVERY notifier fails. `NotifyFailure` returns the first non-zero message ID, so a failing webhook never breaks recovery threading on Telegram. Telegram is always wired first in `main.go`.

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
| `/add <name> <url> [interval]` | Validate name (1–50 chars, `[A-Za-z0-9_-]`, not all-numeric, no leading/trailing `-`) and URL (`http`/`https` need a dotted host; `tcp` needs host:port; `dns`/`ping` need a host). Interval ≥ 10s, default `1m`. `ErrDuplicate` → friendly message. On scheduler failure → warning reply (monitored after restart). Reply includes immediate first-check result. |
| `/delete <name or id>` | Lookup by ID → name → URL. Remove job, then delete row. |
| `/interval <name or id> <interval>` | `updateInterval` helper: update DB, then remove+re-add job; on job failure roll back DB and restore the old job. |
| `/pause <name or id> [duration]` / `/resume <name or id>` | `setPaused` helper: persist flag (+ optional `paused_until`), remove/add job; resume failure rolls the flag back. Also available as an inline button in the detail view. `all` applies to every endpoint. |
| `/maint add <name\|all> <days> <HH:MM-HH:MM>` / `/maint list` / `/maint del <id>` | Recurring maintenance windows (UTC). `ParseMaintDays`/`ParseMaintTimeRange` validate input; window matching is `MaintenanceWindow.Applies`. |
| `/expect <name or id> <status\|any>` | Require an exact HTTP status (0 = any 2xx). |
| `/keyword <name or id> <spec\|off>` | Body check spec: substring, `!` absence, `re:`/`!re:` regexp. Regexps validated before persisting. |
| `/rename <name or id> <new-name>` | Rename; `ErrDuplicate` on name clash. |
| `/uptime <name or id>` | `GetCheckStats` over 24h/7d/30d: uptime %, avg + p95 latency (SQL offset trick), incident count (up→down transitions via `LAG`). Also a detail-view button. |
| `/incidents <name or id>` | `GetRecentTransitions` composed into outages (start, duration or ongoing, HTTP code), newest first, max 5. Also a detail-view button. |
| `/digest` | On-demand 24h digest via `monitor.BuildDigest`. |
| `/export` | Sends a `noroshi-export-<date>.json` Telegram document with all endpoints and maintenance windows (`buildExport` in `internal/bot/export.go`). Windows reference endpoints by name; check history is excluded. |
| `/list` | Summary + per-endpoint lines, inline buttons (detail → interval presets / confirmed delete, refresh). |
| `/status` | Concurrent `CheckNow` for all endpoints; reply with HTTP code + latency per endpoint. |
| `/help` | Static help text. |

All user-provided content is HTML-escaped; messages use `ParseMode: HTML`. The `guarded` middleware drops updates from any chat other than `TELEGRAM_CHAT_ID`.

## Error Handling

`internal/apperror`: `AppError{Code, Message, Cause}` implementing `Error`, `Unwrap`, `Is` (compares `Code`). Sentinels: `ErrNotFound`, `ErrDuplicate`, `ErrInvalidInput`, `ErrDatabase`. `Wrap(sentinel, cause)` clones. Always `errors.Is`/`errors.As` — never string matching (the single exception: `isUniqueViolation` detecting SQLite UNIQUE errors by message, wrapped into `ErrDuplicate` at the storage boundary).

## Logging

`slog` text handler to stderr, level from `LOG_LEVEL`. Scoped loggers are dependency-injected: `main` derives `base.With("component", ...)` (`main`, `bot`, `scheduler`) and passes them to constructors (nil falls back to `slog.Default()` for tests). Operations derive entity context once via `log.With("id"/"name"/"url")`. Conventions: short lowercase action phrases, errors under `"error"`. State transitions and operator actions (endpoint added/deleted/renamed, interval/expect/keyword changes, pause/resume) log at Info with entity context. Telebot handler errors route through `Settings.OnError` into slog. Callback message-edit failures log at Debug (benign "message not modified" cases included). Startup logs a config summary (no secrets).

## Health Endpoint

`GET /healthz` on `HEALTH_PORT` → `200 {"status":"ok"}`. Server has `ReadHeaderTimeout: 5s`. Docker `HEALTHCHECK` curls `http://localhost:${HEALTH_PORT:-8080}/healthz`.

`GET /badge/<name>.svg` → shields-style SVG status badge (up/down/paused/unknown colors), `Cache-Control: no-cache`. Public by design — meant for READMEs and dashboards.

`GET /metrics` → Prometheus exposition from a dedicated registry (`internal/monitor/metrics.go`): `noroshi_checks_total{endpoint,up}`, `noroshi_check_latency_seconds{endpoint}` histogram, `noroshi_endpoint_up{endpoint}` gauge, `noroshi_endpoint_info{endpoint,url,type}` gauge. Recorded in `scheduler.recordCheck` (the choke point for scheduled and ad-hoc checks); `endpoint_info` is set on `Scheduler.Add` and series are deleted on `Scheduler.Remove` (name looked up before the bot deletes the row). The scheduler receives metrics via `SetMetrics` (nil disables) to keep the constructor stable. Public by design, like badges.

## Dashboard API

When `DASHBOARD_TOKEN` is set, `internal/api` is mounted at `/api/` on the health server: a JSON API (stdlib `net/http` mux, no framework) backing the optional React dashboard in `web/`. Every request requires `Authorization: Bearer <DASHBOARD_TOKEN>` (constant-time compare); when unset the API is not mounted at all. CORS allows only the origins listed in `DASHBOARD_ORIGIN` (comma-separated); preflights are answered by middleware before auth.

Management actions mirror the bot's semantics exactly: ad-hoc checks go through `Scheduler.CheckNow` (no failure counters, no notifications), interval changes roll the DB back when re-adding the job fails, resume rolls the pause flag back on job failure, and a scheduler-add failure on create leaves the endpoint persisted for pickup after restart. Validation reuses the bot's exported `ValidateName`/`ValidateURL`/`ValidateKeywordSpec` — the same rules apply to both interfaces. The API's `Store`/`Scheduler` interfaces are defined at the point of use like the bot's.

The frontend is a static Vite build (`web/`, React + TanStack Query/Router + Tailwind) hosted separately; it stores the token in localStorage and sends it as a Bearer header.

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

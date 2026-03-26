# Architecture

**Analysis Date:** 2026-03-26

## System Overview

Noroshi is a single-binary uptime monitor that periodically checks HTTP endpoints and sends status notifications via a Telegram bot. It uses SQLite for persistence, gocron for scheduling, and the telebot library for user interaction. The system runs as a long-lived process with graceful shutdown via OS signals.

**Key Characteristics:**
- Single-process Go binary with no external runtime dependencies (pure Go SQLite driver, no CGO)
- Interface-based dependency injection enabling mock-based testing
- Telegram bot as the sole user interface (long polling, not webhooks)
- Periodic health checks via gocron scheduler, not hand-rolled goroutines

## Layers

**Entry Point (`cmd/monitor/`):**
- Purpose: Wire dependencies, start subsystems, manage lifecycle
- Location: `cmd/monitor/main.go`
- Contains: `main()`, `setupLogging()`, `startHealthServer()`
- Depends on: `internal/config`, `internal/storage`, `internal/monitor`, `internal/bot`
- Used by: Nothing (top of dependency tree)
- Creates the root `context.Context` via `signal.NotifyContext` — all other packages receive it via injection

**Configuration (`internal/config/`):**
- Purpose: Load and validate environment variables
- Location: `internal/config/config.go`
- Contains: `Config` struct, `Load()` function
- Depends on: stdlib only (`os`, `strconv`, `time`)
- Used by: `cmd/monitor/main.go`
- No config libraries — uses `os.Getenv` directly per project rules

**Error Handling (`internal/apperror/`):**
- Purpose: Structured application errors with sentinel matching
- Location: `internal/apperror/apperror.go`
- Contains: `AppError` type, sentinel errors (`ErrNotFound`, `ErrDuplicate`, `ErrInvalidInput`, `ErrDatabase`), `Wrap()` function
- Depends on: stdlib only (`fmt`)
- Used by: `internal/storage`, `internal/bot`

**Storage (`internal/storage/`):**
- Purpose: SQLite persistence and schema migrations
- Location: `internal/storage/store.go`, `internal/storage/models.go`
- Contains: `SQLiteStore` (concrete), `Endpoint` model, `OpenDB()`, `RunMigrations()`
- Depends on: `internal/apperror`, `database/sql`, `modernc.org/sqlite`, `github.com/pressly/goose/v3`
- Used by: `internal/monitor` (via `Store` interface), `internal/bot` (via `Store` interface)
- Migrations embedded via `//go:embed` in `internal/storage/migrations/*.sql`

**Monitor (`internal/monitor/`):**
- Purpose: HTTP health checking and scheduled execution
- Location: `internal/monitor/checker.go`, `internal/monitor/scheduler.go`
- Contains: `HTTPChecker` (performs HTTP checks), `Scheduler` (manages gocron jobs)
- Depends on: `internal/storage` (for `Endpoint` type), `github.com/hashicorp/go-retryablehttp`, `github.com/go-co-op/gocron/v2`
- Used by: `cmd/monitor/main.go`, `internal/bot` (via `Scheduler` interface)
- Defines its own `Store` and `Notifier` interfaces at point of use

**Bot (`internal/bot/`):**
- Purpose: Telegram user interface — command handling, inline keyboards, notifications
- Location: `internal/bot/bot.go`, `internal/bot/handlers.go`, `internal/bot/callbacks.go`, `internal/bot/format.go`, `internal/bot/validate.go`
- Contains: `Bot` struct, `TelegramNotifier`, command handlers, callback handlers, message formatting, URL validation
- Depends on: `internal/storage` (for `Endpoint` type), `internal/apperror`, `gopkg.in/telebot.v4`
- Used by: `cmd/monitor/main.go`
- Defines its own `Store` and `Scheduler` interfaces at point of use

## Data Flow

**Startup Sequence:**

1. `main()` creates root context via `signal.NotifyContext(context.Background(), SIGINT, SIGTERM)` — `cmd/monitor/main.go:20`
2. `config.Load()` reads environment variables
3. `storage.OpenDB()` opens SQLite with WAL mode, `storage.RunMigrations()` runs goose migrations
4. `storage.NewSQLiteStore(db)` creates the concrete store
5. `monitor.NewHTTPChecker(timeout)` creates the HTTP checker
6. `bot.NewBot(token, chatID, store, ctx)` creates the Telegram bot (registers handlers)
7. `bot.NewTelegramNotifier(teleBot, maxFail)` wraps the bot as a `monitor.Notifier`
8. `monitor.NewScheduler(ctx, store, checker, notifier, maxFail)` creates the scheduler
9. `teleBot.SetScheduler(scheduler)` closes the circular dependency (bot needs scheduler, scheduler needs notifier which wraps bot)
10. All existing endpoints loaded from DB and added to scheduler
11. `scheduler.Start()`, `teleBot.Start()`, `startHealthServer()` begin operation
12. `<-ctx.Done()` blocks until OS signal, then graceful shutdown in reverse order

**Health Check Cycle (per endpoint):**

1. gocron fires `Scheduler.checkAndNotify(endpointID)` at the configured interval — `internal/monitor/scheduler.go:86`
2. Scheduler fetches current endpoint from store: `store.GetEndpoint(ctx, id)` — `scheduler.go:89`
3. `HTTPChecker.Check(ctx, url)` performs GET with retryablehttp (2 retries, 500ms-2s backoff) — `checker.go:30`
4. On failure (non-200 or connection error): `store.RecordFailure()` increments counters, then `notifier.NotifyFailure()` if under notification cap
5. On recovery (was not_ok, now 200): `store.RecordRecovery()` resets counters, then `notifier.NotifyRecovery()` with downtime duration
6. On steady-ok: `store.UpdateEndpointStatus()` updates timestamp only

**User Command Flow (e.g., /add):**

1. Telegram long poller receives message — `internal/bot/bot.go:44`
2. `guarded()` middleware rejects messages from non-configured chat IDs — `bot.go:96`
3. Handler parses arguments, validates input — `internal/bot/handlers.go:27`
4. Handler calls `store.AddEndpoint()` for persistence — `handlers.go:51`
5. Handler calls `scheduler.Add()` to start monitoring immediately — `handlers.go:61`
6. Handler sends formatted response with HTML markup — `handlers.go:66`

**Inline Keyboard Flow (e.g., detail view):**

1. User taps inline button in `/list` response
2. Callback handler identified by unique ID (e.g., `cbDetail` = `"dtl"`) — `internal/bot/callbacks.go:14`
3. Handler parses endpoint ID from callback data — `callbacks.go:25`
4. `FormatEndpointDetail()` builds HTML text + inline keyboard markup — `internal/bot/format.go:152`
5. Message edited in-place via `c.Edit()` — `callbacks.go:36`

**State Management:**
- All state lives in SQLite (single `endpoints` table)
- No in-memory caches — every check/command hits the database
- gocron manages job scheduling state internally; jobs are identified by tags (`endpoint-{id}`)

## Key Abstractions

**Store Interface (defined twice, at point of use):**
- `internal/monitor/scheduler.go:15-20` — subset needed by scheduler (Get, Update, RecordFailure, RecordRecovery)
- `internal/bot/bot.go:15-23` — larger subset needed by bot (Add, Get, GetByURL, GetByName, Delete, List, UpdateInterval)
- Concrete implementation: `storage.SQLiteStore` in `internal/storage/store.go:47`

**Notifier Interface:**
- Defined in `internal/monitor/scheduler.go:23-26`
- Methods: `NotifyFailure(ctx, endpoint)`, `NotifyRecovery(ctx, endpoint, downtime)`
- Concrete implementation: `bot.TelegramNotifier` in `internal/bot/bot.go:120`

**Scheduler Interface:**
- Defined in `internal/bot/bot.go:26-29`
- Methods: `Add(ctx, endpoint)`, `Remove(endpointID)`
- Concrete implementation: `monitor.Scheduler` in `internal/monitor/scheduler.go:29`

**Circular Dependency Resolution:**
- Bot needs Scheduler (to add/remove jobs when user adds/deletes endpoints)
- Scheduler needs Notifier (to send alerts), and Notifier wraps Bot
- Resolution: `Bot.SetScheduler()` is called after both are constructed — `cmd/monitor/main.go:67`

## Entry Points

**`cmd/monitor/main.go`:**
- Location: `cmd/monitor/main.go`
- Triggers: Direct execution or Docker container start
- Responsibilities: Dependency wiring, lifecycle management, health endpoint (`GET /healthz` on configurable port)

## Error Handling

**Strategy:** Sentinel errors with cause wrapping via `internal/apperror`

**Patterns:**
- Storage layer wraps all DB errors: `apperror.Wrap(apperror.ErrDatabase, err)` — `internal/storage/store.go`
- Not-found returns: `apperror.Wrap(apperror.ErrNotFound, err)` — checked with `errors.Is(err, apperror.ErrNotFound)`
- Unique constraint violations: `apperror.Wrap(apperror.ErrDuplicate, err)` — string-based detection in `isUniqueViolation()`
- Bot handlers check error type and send user-friendly messages: `"Endpoint not found."`, `"This name or URL is already being monitored."`
- Scheduler logs errors via `slog.Error()` and continues (non-fatal for individual check failures)

## Cross-Cutting Concerns

**Logging:**
- Framework: `log/slog` (stdlib)
- Level configurable via `LOG_LEVEL` env var (debug, info, warn, error)
- Text handler writing to stderr: `slog.NewTextHandler(os.Stderr, ...)` — `cmd/monitor/main.go:144`
- Used throughout with structured fields: `slog.Error("context", "key", value, "error", err)`

**Validation:**
- URL validation in `internal/bot/validate.go`: must be http/https, must have domain with dot
- Interval validation in handlers: minimum 10 seconds
- Config validation in `internal/config/config.go`: required fields checked at startup

**Authentication:**
- Single-chat guard: `Bot.guarded()` middleware drops messages from unauthorized chat IDs — `internal/bot/bot.go:96`
- No multi-user support — one Telegram chat ID configured via env var

**Context Propagation:**
- Root context created in `main()` via `signal.NotifyContext` — `cmd/monitor/main.go:20`
- Passed to `bot.NewBot()` and stored as `Bot.rootCtx` — used in all store/scheduler calls
- Passed to `monitor.NewScheduler()` and stored as `Scheduler.ctx` — used in `checkAndNotify()`
- `context.Background()` used only in `main.go` (for `signal.NotifyContext` and `healthSrv.Shutdown`)

---

*Architecture analysis: 2026-03-26*

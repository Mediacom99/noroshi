# Noroshi — Uptime Monitor

## Module

- Go module path: `noroshi`
- Build: `CGO_ENABLED=0 go build ./cmd/monitor/`
- Vet: `go vet ./...`
- Test: `go test ./...`

## Mandatory Libraries

These are the ONLY external dependencies. NEVER add others without explicit approval.

| Purpose | Library | Notes |
|---------|---------|-------|
| Scheduling | `github.com/go-co-op/gocron/v2` | NEVER use time.Ticker + goroutine + sync.Mutex |
| HTTP checks | `github.com/hashicorp/go-retryablehttp` | NEVER use raw net/http for health checks |
| DB migrations | `github.com/pressly/goose/v3` | NEVER use inline CREATE TABLE statements |
| Telegram bot | `gopkg.in/telebot.v4` | Long polling, not webhooks |
| SQLite driver | `modernc.org/sqlite` | Pure Go, no CGO |
| SQL interface | `database/sql` | stdlib |
| Logging | `log/slog` | stdlib |
| Config | `os.Getenv` | NEVER use config libraries (viper, envconfig, etc.) |

## Error Handling

- Define a custom `AppError` type in `internal/apperror/` with `Code`, `Message`, and `Cause` fields.
- `AppError` MUST implement `Error()`, `Unwrap()`, and `Is(target error) bool` (comparing `Code` for equality).
- Define sentinel errors: `ErrNotFound`, `ErrDuplicate`, `ErrInvalidInput`, `ErrDatabase`.
- Use `Wrap(sentinel, cause)` to clone a sentinel and attach a cause.
- Always use `errors.Is` / `errors.As` for error checking — never compare error strings.

## Testing Requirements

- Every non-main package MUST have `_test.go` files, including `internal/bot/` (handlers and callbacks are tested with mock `tele.Context`).
- Use stdlib `testing` only — no testify, no gomock.
- Use table-driven tests where applicable.
- `go test ./...` MUST pass before every commit.

## Monitoring Rules

- Any HTTP 2xx status counts as UP; everything else (3xx, 4xx, 5xx, connection errors) is DOWN.
- gocron jobs MUST use `gocron.WithSingletonMode(gocron.LimitModeReschedule)` — checks for the same endpoint must never overlap.
- Alerts start only after `FAILURE_THRESHOLD` consecutive failures (default 1) and stop after `MAX_FAILURE_NOTIFICATIONS` (default 3). `failure_notifications_sent` only counts failures at/beyond the threshold and is capped in `RecordFailure`; notify only when the counter actually increments.
- Paused endpoints (`paused` column, migration 005) have no gocron job and are never checked — not at startup, not by `checkAndNotify`, not by `CheckNow`.
- Ad-hoc checks (`/status`, "Check now" button → `Scheduler.CheckNow`) never touch failure counters and never send notifications.

## Code Style

- Define interfaces at the point of use, not in the implementing package.
- `Store` is an interface — implementations are concrete structs. This enables mock-based testing.
- Context propagation: the root context from `signal.NotifyContext` flows everywhere. NEVER call `context.Background()` outside of `main.go`.
- All function signatures that do I/O MUST take `context.Context` as the first parameter.

## What NOT To Do

- NEVER hand-roll a scheduler with time.Ticker + goroutine + sync.Mutex. Use gocron.
- NEVER skip writing tests. Every step that creates a package must include tests.
- NEVER use `context.Background()` outside `main.go`. Propagate the root context.
- NEVER write inline SQL schemas (CREATE TABLE in Go code). Use goose migration files.
- NEVER use raw `net/http` for health checks. Use retryablehttp.
- NEVER use global variables for state. Use dependency injection.
- NEVER add a dependency not listed in the Mandatory Libraries table.

## Commit Rules

- `CGO_ENABLED=0 go build ./cmd/monitor/`, `go vet ./...`, and `go test ./...` MUST all pass before committing.
- One logical change per commit. Concise, descriptive commit messages.

<!-- GSD:project-start source:PROJECT.md -->
## Project

**Noroshi — Uptime Monitor**

A self-contained uptime monitor built in Go that uses a Telegram bot as its sole interface. Users add HTTP endpoints to monitor via chat commands, and the bot sends alerts on failures and recoveries. Runs as a single Docker container with SQLite for persistence. Designed to be simple, useful, and easy to self-host.

**Core Value:** Reliable uptime monitoring with zero-friction setup — one Docker container, one Telegram bot, no dashboards to maintain.

### Constraints

- **Dependencies:** Only the libraries listed in CLAUDE.md — no new deps without explicit approval
- **CGO:** Must build with `CGO_ENABLED=0` (pure Go SQLite driver)
- **Testing:** Every non-main package must have `_test.go` files (exception: `internal/bot/` historically, but this milestone adds bot tests)
- **Context:** No `context.Background()` outside `main.go`
- **Interfaces:** Defined at point of use, not in implementing package
<!-- GSD:project-end -->

<!-- GSD:stack-start source:codebase/STACK.md -->
## Technology Stack

## Languages
- Go 1.26.1 - All application code
- SQL - Database migrations (`internal/storage/migrations/*.sql`)
- Shell (sh) - Docker entrypoint (`entrypoint.sh`)
## Runtime
- Go 1.26.1 (compiled binary, no runtime needed beyond OS)
- Build constraint: `CGO_ENABLED=0` (pure Go, no C dependencies)
- Go modules
- Module path: `noroshi`
- Lockfile: `go.sum` present
## Frameworks
- No web framework - single-purpose CLI binary at `cmd/monitor/main.go`
- `gopkg.in/telebot.v4` v4.0.0-beta.7 - Telegram Bot API (long polling, not webhooks)
- `github.com/go-co-op/gocron/v2` v2.19.1 - Cron-style job scheduler for periodic health checks
- stdlib `testing` only - no external test frameworks, no testify, no gomock
- `go build` with `-ldflags="-s -w"` for stripped production binaries
- Docker multi-stage build (`Dockerfile`)
## Key Dependencies
| Package | Version | Purpose | Usage Locations |
|---------|---------|---------|-----------------|
| `github.com/go-co-op/gocron/v2` | v2.19.1 | Job scheduling for periodic health checks | `internal/monitor/scheduler.go` |
| `github.com/hashicorp/go-retryablehttp` | v0.7.8 | HTTP health checks with automatic retry | `internal/monitor/checker.go` |
| `github.com/pressly/goose/v3` | v3.27.0 | Database schema migrations (embedded SQL) | `internal/storage/store.go` |
| `gopkg.in/telebot.v4` | v4.0.0-beta.7 | Telegram bot interface (long polling) | `internal/bot/*.go` |
| `modernc.org/sqlite` | v1.46.1 | Pure-Go SQLite driver (no CGO) | `internal/storage/store.go` |
- `database/sql` - SQL interface wrapping the SQLite driver
- `log/slog` - Structured logging (text handler to stderr)
- `os` / `os/signal` - Configuration via `os.Getenv`, graceful shutdown via signals
- `net/http` - Health check HTTP server only (`/healthz` endpoint)
- `embed` - Embedding migration SQL files into the binary
- `context` - Propagated from root `signal.NotifyContext` throughout the app
| Package | Version | Pulled By |
|---------|---------|-----------|
| `github.com/hashicorp/go-cleanhttp` | v0.5.2 | go-retryablehttp |
| `github.com/google/uuid` | v1.6.0 | gocron |
| `github.com/jonboulle/clockwork` | v0.5.0 | gocron |
| `github.com/robfig/cron/v3` | v3.0.1 | gocron |
| `github.com/dustin/go-humanize` | v1.0.1 | modernc.org/sqlite |
| `github.com/mattn/go-isatty` | v0.0.20 | modernc.org/sqlite |
| `github.com/ncruces/go-strftime` | v1.0.0 | modernc.org/sqlite |
| `github.com/remyoudompheng/bigfft` | v0.0.0 | modernc.org/sqlite |
| `github.com/mfridman/interpolate` | v0.0.2 | goose |
| `github.com/sethvargo/go-retry` | v0.3.0 | goose |
| `go.uber.org/multierr` | v1.11.0 | goose |
| `golang.org/x/exp` | v0.0.0 | gocron |
| `golang.org/x/sync` | v0.19.0 | goose |
| `golang.org/x/sys` | v0.41.0 | modernc.org/sqlite |
| `modernc.org/libc` | v1.68.0 | modernc.org/sqlite |
| `modernc.org/mathutil` | v1.7.1 | modernc.org/sqlite |
| `modernc.org/memory` | v1.11.0 | modernc.org/sqlite |
## Configuration
- All configuration via `os.Getenv` - no config libraries allowed
- Implementation: `internal/config/config.go` (`config.Load()`)
- `.env.example` documents all variables
- `.env` exists locally (gitignored, never read by the app directly - must be injected)
- `TELEGRAM_TOKEN` - Telegram Bot API token
- `TELEGRAM_CHAT_ID` - Target chat ID (int64, e.g., `-100123456789`)
- `DATABASE_PATH` - SQLite file path (default: `./data/uptime.db`)
- `HTTP_TIMEOUT` - Health check timeout as Go duration (default: `10s`)
- `MAX_FAILURE_NOTIFICATIONS` - Max consecutive failure alerts (default: `3`)
- `LOG_LEVEL` - Logging level: debug, info, warn, error (default: `info`)
- `HEALTH_PORT` - Health check HTTP server port (default: `8080`)
- `Dockerfile` - Multi-stage: `golang:1.26.1-alpine` builder, `alpine:3.21` runtime
- `docker-compose.yml` - Single service with named volume for SQLite persistence
- Build command: `CGO_ENABLED=0 go build -ldflags="-s -w" -o /monitor ./cmd/monitor/`
## Build & Deploy
- Multi-stage build: Go 1.26.1 Alpine builder -> Alpine 3.21 runtime
- Runtime installs: `ca-certificates`, `tzdata`, `curl`, `su-exec`
- Runs as non-root `appuser` via `entrypoint.sh` with `su-exec`
- Volume: `/app/data` for SQLite database persistence
- Health check: `curl -f http://localhost:8080/healthz` every 30s
- `entrypoint.sh` fixes volume permissions before starting the binary
- Coolify (based on recent commit: "Add Coolify deployment setup with volume permission fix")
- Docker Compose compatible hosting
## Platform Requirements
- Go 1.26.1+
- No CGO required (pure Go SQLite driver)
- Telegram Bot API token for full integration testing
- Linux (Alpine-based Docker container)
- Writable filesystem for SQLite database at `DATABASE_PATH`
- Outbound HTTPS to `api.telegram.org` (Telegram Bot API)
- Outbound HTTP/HTTPS to monitored endpoint URLs
- Port 8080 (configurable) for health check endpoint
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

## Naming Patterns
- Short, lowercase, single-word names: `bot`, `monitor`, `storage`, `config`, `apperror`
- No underscores or camelCase in package names
- Lowercase with underscores for multi-word: `apperror.go`, `apperror_test.go`
- Functional grouping within packages: `bot.go` (core struct), `handlers.go` (command handlers), `callbacks.go` (inline keyboard callbacks), `format.go` (message formatting), `validate.go` (input validation)
- Models in a separate file: `internal/storage/models.go`
- Exported: PascalCase (`NewHTTPChecker`, `FormatDuration`, `ValidateURL`)
- Unexported: camelCase (`checkAndNotify`, `htmlEscape`, `statusEmoji`, `isUniqueViolation`)
- Constructors: `New` prefix returning the struct pointer (`NewBot`, `NewSQLiteStore`, `NewHTTPChecker`, `NewScheduler`)
- Struct fields: PascalCase for exported (`ID`, `URL`, `Name`), camelCase for unexported (`bot`, `store`, `rootCtx`)
- Local variables: short camelCase (`ep`, `cfg`, `srv`, `tb`)
- Abbreviations stay uppercase: `ID`, `URL`, `HTTP`
- Structs: PascalCase nouns (`Bot`, `Scheduler`, `HTTPChecker`, `SQLiteStore`, `Endpoint`)
- Interfaces: named by method set or purpose (`Store`, `Scheduler`, `Notifier`)
- Error type: `AppError` with pointer receiver
- Unexported callback identifiers use short abbreviations: `cbDetail = "dtl"`, `cbDelete = "del"`, `cbConfirmDelete = "cdel"`
## Error Handling
- Defined in `internal/apperror/apperror.go`
- Struct with `Code` (string), `Message` (string), `Cause` (error)
- Implements `Error()`, `Unwrap()`, and `Is(target error) bool` (compares `Code` field)
- `apperror.ErrNotFound` (Code: `"NOT_FOUND"`)
- `apperror.ErrDuplicate` (Code: `"DUPLICATE"`)
- `apperror.ErrInvalidInput` (Code: `"INVALID_INPUT"`)
- `apperror.ErrDatabase` (Code: `"DATABASE"`)
- Use `apperror.Wrap(sentinel, cause)` to clone a sentinel and attach a root cause
- Always check errors with `errors.Is()` -- never compare error strings
- For non-sentinel errors, use `fmt.Errorf("context: %w", err)` (seen in `cmd/monitor/main.go`, `internal/monitor/scheduler.go`)
- Use `slog.Error("action description", "error", err)` for operational errors
- Use `slog.Error("action description", "id", id, "error", err)` when entity context is available
## Code Organization
- Define interfaces at the point of use (consumer side), not in the implementing package
- `internal/bot/bot.go` defines `Store` interface (consumed by bot, implemented by `storage.SQLiteStore`)
- `internal/bot/bot.go` defines `Scheduler` interface (consumed by bot, implemented by `monitor.Scheduler`)
- `internal/monitor/scheduler.go` defines `Store` interface (a different, narrower set of methods than bot's `Store`)
- `internal/monitor/scheduler.go` defines `Notifier` interface (consumed by scheduler, implemented by `bot.TelegramNotifier`)
- Root context created in `cmd/monitor/main.go` via `signal.NotifyContext(context.Background(), ...)`
- `context.Background()` is used ONLY in `main.go`
- The root context is passed to `Bot` via constructor and stored as `rootCtx`
- All I/O functions take `context.Context` as the first parameter
- In tests, `context.Background()` is acceptable
- All dependencies injected via constructors (`NewBot`, `NewScheduler`, `NewSQLiteStore`)
- No global mutable state
- Circular dependency between `Bot` and `Scheduler` resolved by `SetScheduler()` method called after both are constructed
- `Scheduler` field checked for nil before use: `if b.scheduler != nil { ... }`
- `cmd/monitor/` -- entry point, wiring, config loading
- `internal/config/` -- env var reading, validation, defaults
- `internal/apperror/` -- error types and sentinels
- `internal/storage/` -- data models, SQLite store, migrations
- `internal/monitor/` -- HTTP checker, scheduler (gocron), check-and-notify logic
- `internal/bot/` -- Telegram bot, command handlers, callbacks, formatting, validation
## Formatting & Style
- Standard `gofmt` (no custom formatter configuration detected)
- No `.golangci.yml` or other linter configs present
- `go vet ./...` required before every commit (per `CLAUDE.md`)
- No golangci-lint or other third-party linters configured
- `tele "gopkg.in/telebot.v4"` -- short alias for the Telegram bot library
- Blank import for SQLite driver: `_ "modernc.org/sqlite"`
- Exported types and constructors first
- Exported methods second
- Unexported helper functions last
- In `bot.go`: type definition -> `NewBot` -> `SetScheduler` -> `Start`/`Stop` -> `registerCommands` -> `guarded` -> `SendMessage`/`SendSilentMessage` -> `TelegramNotifier`
- Godoc-style comments on all exported types and functions
- Brief, single-line comments for most exported items
- No comments on unexported functions unless the logic is non-obvious
- Inline comments used sparingly to explain "why" not "what"
## Logging
- Use structured key-value pairs: `slog.Error("action", "key", value, "error", err)`
- Action descriptions are short lowercase phrases: `"load config"`, `"add endpoint"`, `"scheduler: get endpoint"`
- Prefix with component name for scheduler logs: `"scheduler: record failure"`
- Use `slog.Info` for lifecycle events: `"telegram bot started"`, `"loaded endpoints"`
## Configuration
- Required vars fail fast with descriptive error messages
- Optional vars have sensible defaults
- All config loaded in one place: `internal/config/config.go`
- Config struct is plain data -- no methods besides the `Load()` constructor function
## Database Migrations
- Migration files in `internal/storage/migrations/`
- Naming: `NNN_description.sql` (e.g., `001_create_endpoints.sql`)
- Each file has `-- +goose Up` and `-- +goose Down` sections
- Migrations embedded via `//go:embed migrations/*.sql`
- Never use inline `CREATE TABLE` statements in Go code
## Telegram Message Formatting
- Always HTML-escape user-provided content with `html.EscapeString()`
- Use `<b>`, `<code>` tags for formatting
- Use emoji prefixes for visual status indicators
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

## System Overview
- Single-process Go binary with no external runtime dependencies (pure Go SQLite driver, no CGO)
- Interface-based dependency injection enabling mock-based testing
- Telegram bot as the sole user interface (long polling, not webhooks)
- Periodic health checks via gocron scheduler, not hand-rolled goroutines
## Layers
- Purpose: Wire dependencies, start subsystems, manage lifecycle
- Location: `cmd/monitor/main.go`
- Contains: `main()`, `setupLogging()`, `startHealthServer()`
- Depends on: `internal/config`, `internal/storage`, `internal/monitor`, `internal/bot`
- Used by: Nothing (top of dependency tree)
- Creates the root `context.Context` via `signal.NotifyContext` — all other packages receive it via injection
- Purpose: Load and validate environment variables
- Location: `internal/config/config.go`
- Contains: `Config` struct, `Load()` function
- Depends on: stdlib only (`os`, `strconv`, `time`)
- Used by: `cmd/monitor/main.go`
- No config libraries — uses `os.Getenv` directly per project rules
- Purpose: Structured application errors with sentinel matching
- Location: `internal/apperror/apperror.go`
- Contains: `AppError` type, sentinel errors (`ErrNotFound`, `ErrDuplicate`, `ErrInvalidInput`, `ErrDatabase`), `Wrap()` function
- Depends on: stdlib only (`fmt`)
- Used by: `internal/storage`, `internal/bot`
- Purpose: SQLite persistence and schema migrations
- Location: `internal/storage/store.go`, `internal/storage/models.go`
- Contains: `SQLiteStore` (concrete), `Endpoint` model, `OpenDB()`, `RunMigrations()`
- Depends on: `internal/apperror`, `database/sql`, `modernc.org/sqlite`, `github.com/pressly/goose/v3`
- Used by: `internal/monitor` (via `Store` interface), `internal/bot` (via `Store` interface)
- Migrations embedded via `//go:embed` in `internal/storage/migrations/*.sql`
- Purpose: HTTP health checking and scheduled execution
- Location: `internal/monitor/checker.go`, `internal/monitor/scheduler.go`
- Contains: `HTTPChecker` (performs HTTP checks), `Scheduler` (manages gocron jobs)
- Depends on: `internal/storage` (for `Endpoint` type), `github.com/hashicorp/go-retryablehttp`, `github.com/go-co-op/gocron/v2`
- Used by: `cmd/monitor/main.go`, `internal/bot` (via `Scheduler` interface)
- Defines its own `Store` and `Notifier` interfaces at point of use
- Purpose: Telegram user interface — command handling, inline keyboards, notifications
- Location: `internal/bot/bot.go`, `internal/bot/handlers.go`, `internal/bot/callbacks.go`, `internal/bot/format.go`, `internal/bot/validate.go`
- Contains: `Bot` struct, `TelegramNotifier`, command handlers, callback handlers, message formatting, URL validation
- Depends on: `internal/storage` (for `Endpoint` type), `internal/apperror`, `gopkg.in/telebot.v4`
- Used by: `cmd/monitor/main.go`
- Defines its own `Store` and `Scheduler` interfaces at point of use
## Data Flow
- All state lives in SQLite (single `endpoints` table)
- No in-memory caches — every check/command hits the database
- gocron manages job scheduling state internally; jobs are identified by tags (`endpoint-{id}`)
## Key Abstractions
- `internal/monitor/scheduler.go:15-20` — subset needed by scheduler (Get, Update, RecordFailure, RecordRecovery)
- `internal/bot/bot.go:15-23` — larger subset needed by bot (Add, Get, GetByURL, GetByName, Delete, List, UpdateInterval)
- Concrete implementation: `storage.SQLiteStore` in `internal/storage/store.go:47`
- Defined in `internal/monitor/scheduler.go:23-26`
- Methods: `NotifyFailure(ctx, endpoint)`, `NotifyRecovery(ctx, endpoint, downtime)`
- Concrete implementation: `bot.TelegramNotifier` in `internal/bot/bot.go:120`
- Defined in `internal/bot/bot.go:26-29`
- Methods: `Add(ctx, endpoint)`, `Remove(endpointID)`
- Concrete implementation: `monitor.Scheduler` in `internal/monitor/scheduler.go:29`
- Bot needs Scheduler (to add/remove jobs when user adds/deletes endpoints)
- Scheduler needs Notifier (to send alerts), and Notifier wraps Bot
- Resolution: `Bot.SetScheduler()` is called after both are constructed — `cmd/monitor/main.go:67`
## Entry Points
- Location: `cmd/monitor/main.go`
- Triggers: Direct execution or Docker container start
- Responsibilities: Dependency wiring, lifecycle management, health endpoint (`GET /healthz` on configurable port)
## Error Handling
- Storage layer wraps all DB errors: `apperror.Wrap(apperror.ErrDatabase, err)` — `internal/storage/store.go`
- Not-found returns: `apperror.Wrap(apperror.ErrNotFound, err)` — checked with `errors.Is(err, apperror.ErrNotFound)`
- Unique constraint violations: `apperror.Wrap(apperror.ErrDuplicate, err)` — string-based detection in `isUniqueViolation()`
- Bot handlers check error type and send user-friendly messages: `"Endpoint not found."`, `"This name or URL is already being monitored."`
- Scheduler logs errors via `slog.Error()` and continues (non-fatal for individual check failures)
## Cross-Cutting Concerns
- Framework: `log/slog` (stdlib)
- Level configurable via `LOG_LEVEL` env var (debug, info, warn, error)
- Text handler writing to stderr: `slog.NewTextHandler(os.Stderr, ...)` — `cmd/monitor/main.go:144`
- Used throughout with structured fields: `slog.Error("context", "key", value, "error", err)`
- URL validation in `internal/bot/validate.go`: must be http/https, must have domain with dot
- Interval validation in handlers: minimum 10 seconds
- Config validation in `internal/config/config.go`: required fields checked at startup
- Single-chat guard: `Bot.guarded()` middleware drops messages from unauthorized chat IDs — `internal/bot/bot.go:96`
- No multi-user support — one Telegram chat ID configured via env var
- Root context created in `main()` via `signal.NotifyContext` — `cmd/monitor/main.go:20`
- Passed to `bot.NewBot()` and stored as `Bot.rootCtx` — used in all store/scheduler calls
- Passed to `monitor.NewScheduler()` and stored as `Scheduler.ctx` — used in `checkAndNotify()`
- `context.Background()` used only in `main.go` (for `signal.NotifyContext` and `healthSrv.Shutdown`)
<!-- GSD:architecture-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd:quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd:debug` for investigation and bug fixing
- `/gsd:execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->

<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd:profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->

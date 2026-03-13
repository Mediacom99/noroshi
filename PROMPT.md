# Noroshi — Build Prompt

## Before You Start

1. Read `CLAUDE.md` for mandatory rules and constraints.
2. Read `DESIGN.md` for architecture, patterns, and API usage.
3. Check existing files, `go.mod`, and git history. **Never redo completed work** — pick up from where the last step left off.
4. If a step is already done (files exist, tests pass), skip it.

## Quality Gates

These apply to EVERY step. Before committing, ALL must pass:

```sh
CGO_ENABLED=0 go build ./cmd/monitor/
go vet ./...
go test ./...
```

If any gate fails, fix before committing. One logical change per commit.

**Note:** In early steps before `cmd/monitor/main.go` exists, the build gate won't apply — but vet and test must still pass.

## Steps

Complete in order. Commit after each step.

---

### Step 1: Initialize module and dependencies

- `go mod init noroshi`
- Add all 5 external dependencies to `go.mod`:
  - `github.com/go-co-op/gocron/v2`
  - `github.com/hashicorp/go-retryablehttp`
  - `github.com/pressly/goose/v3`
  - `gopkg.in/telebot.v4`
  - `modernc.org/sqlite`
- Run `go mod tidy`. The `go.mod` must list all 5 deps and `go.sum` must be populated.

**Acceptance**: `go.mod` lists all 5 external deps. `go mod tidy` produces no changes.

---

### Step 2: Custom error types — `internal/apperror/`

Create `internal/apperror/apperror.go`:
- `AppError` struct with `Code`, `Message`, `Cause` fields.
- `Error()` method: returns `Message: Cause` if cause exists, else `Message`.
- `Unwrap()` method: returns `Cause`.
- `Is(target error) bool` method: type-assert target to `*AppError`, compare `Code` for equality.
- `Wrap(sentinel *AppError, cause error) *AppError` function: clones sentinel and sets Cause.
- Sentinels: `ErrNotFound`, `ErrDuplicate`, `ErrInvalidInput`, `ErrDatabase`.

Create `internal/apperror/apperror_test.go`:
- Test `errors.Is(Wrap(ErrNotFound, someErr), ErrNotFound)` returns true.
- Test `errors.Is(Wrap(ErrNotFound, someErr), ErrDuplicate)` returns false.
- Test `Unwrap` returns the original cause.
- Test `Error()` output with and without cause.

**Acceptance**: `go test ./internal/apperror/` passes. `errors.Is` works correctly through `Wrap`.

---

### Step 3: Config — `internal/config/`

Create `internal/config/config.go`:
- `Config` struct with 7 fields matching DESIGN.md environment variables.
- `Load() (*Config, error)` reads from `os.Getenv`, applies defaults, validates.
- Validation: `TELEGRAM_TOKEN` and `TELEGRAM_CHAT_ID` required. `TELEGRAM_CHAT_ID` must parse as int64. `HTTP_TIMEOUT` must parse as `time.Duration`. `MAX_FAILURE_NOTIFICATIONS` must parse as int.

Create `internal/config/config_test.go`:
- Test valid config with all env vars set.
- Test defaults are applied when optional vars are missing.
- Test error when required vars are missing.
- Test error for invalid `TELEGRAM_CHAT_ID` format.

**Acceptance**: `go test ./internal/config/` passes. All 7 fields populate correctly.

---

### Step 4: Database and migrations — `internal/storage/`

Create `internal/storage/migrations/001_create_endpoints.sql`:
- goose migration with `-- +goose Up` and `-- +goose Down` markers.
- Schema exactly as specified in DESIGN.md.

Create `internal/storage/store.go` (just the migration runner for now):
- `//go:embed migrations/*.sql` directive.
- `RunMigrations(db *sql.DB) error` using goose with embedded FS.
- `OpenDB(path string) (*sql.DB, error)` that opens SQLite with WAL and busy timeout.

Create `internal/storage/store_test.go`:
- Test migrations run on in-memory SQLite (`:memory:`).
- Test migrations are idempotent (run twice, no error).
- Test endpoints table exists after migration.

**Acceptance**: `go test ./internal/storage/` passes. In-memory DB has endpoints table after migration.

---

### Step 5: Storage layer — `internal/storage/`

Create `internal/storage/models.go`:
- `Endpoint` struct with fields matching the DB schema. Use `sql.NullTime` for nullable datetime fields.

Expand `internal/storage/store.go`:
- `SQLiteStore` struct holding `*sql.DB`.
- Implement all methods from the Store interface in DESIGN.md.
- All errors MUST use apperror sentinels (`ErrNotFound`, `ErrDuplicate`, `ErrDatabase`).
- Detect UNIQUE constraint violations → return `ErrDuplicate`.
- Detect `sql.ErrNoRows` → return `ErrNotFound`.

Expand `internal/storage/store_test.go`:
- Test `AddEndpoint` + `GetEndpoint` round-trip.
- Test `AddEndpoint` duplicate URL → `errors.Is(err, apperror.ErrDuplicate)`.
- Test `GetEndpoint` non-existent ID → `errors.Is(err, apperror.ErrNotFound)`.
- Test `DeleteEndpoint` + verify gone.
- Test `ListEndpoints` returns all added endpoints.
- Test `RecordFailure` increments counters correctly.
- Test `RecordRecovery` resets counters and returns old `last_failure_at`.
- Test `UpdateEndpointInterval`.
- Test `GetEndpointByURL` with existing and non-existent URL.
- Test `UpdateEndpointStatus` updates status and last_checked_at.

**Acceptance**: `go test ./internal/storage/` passes. All CRUD operations work. Error sentinels are correct.

---

### Step 6: Health checker — `internal/monitor/`

Create `internal/monitor/checker.go`:
- `Checker` struct holding a `*retryablehttp.Client`.
- `NewChecker(timeout time.Duration) *Checker` — configure retryablehttp as specified in DESIGN.md.
- `Check(ctx context.Context, url string) (statusCode int, err error)` — returns status code and error.
  - On success: returns `resp.StatusCode, nil`.
  - On connection error: returns `0, err`.

Create `internal/monitor/checker_test.go`:
- Use `httptest.NewServer` for test targets.
- Test 200 response → returns 200, nil.
- Test 503 response → returns 503, nil (retries exhausted, still returns the status code).
- Test unreachable server → returns 0, error.
- Test cancelled context → returns quickly with error.

**Acceptance**: `go test ./internal/monitor/` passes. Checker uses retryablehttp, not raw net/http.

---

### Step 7: Scheduler — `internal/monitor/`

Create `internal/monitor/scheduler.go`:
- `Notifier` interface: `NotifyFailure(ctx, Endpoint) error`, `NotifyRecovery(ctx, Endpoint, time.Duration) error`.
- Define a `Store` interface locally (at point of use) with the methods the scheduler needs.
- `Scheduler` struct holding gocron scheduler, store, checker, notifier, and config values.
- `NewScheduler(store, checker, notifier, maxFailureNotifications) *Scheduler`.
- `Start()` — calls `s.Start()` on the gocron scheduler.
- `Add(ctx context.Context, endpoint Endpoint) error` — creates a gocron job tagged `endpoint-{id}`.
- `Remove(endpointID int64) error` — removes by tag.
- `Shutdown() error` — calls gocron `Shutdown()`.
- Internal `checkAndNotify(ctx, endpointID)` method — implements the notification logic from DESIGN.md.

Create `internal/monitor/scheduler_test.go`:
- Create mock store and mock notifier (simple structs implementing the interfaces).
- Test: add endpoint → checkAndNotify runs → store is updated.
- Test: failure triggers `NotifyFailure` call.
- Test: failure notifications stop after cap is reached.
- Test: recovery after failure triggers `NotifyRecovery` with correct downtime.
- Test: no notification when endpoint stays OK.

**Acceptance**: `go test ./internal/monitor/` passes. Scheduler uses gocron, not manual goroutines.

---

### Step 8: Message formatting — `internal/bot/`

Create `internal/bot/format.go`:
- `FormatDuration(d time.Duration) string` — human-readable: "2h 15m 30s", "12m 34s", "45s".
- `FormatFailure(endpoint, maxFailures int) string` — failure notification message.
- `FormatRecovery(endpoint, downtime time.Duration) string` — recovery notification message.
- `FormatEndpointList(endpoints []Endpoint) string` — list/status display.
- `FormatHelp() string` — help text.

Create `internal/bot/format_test.go`:
- Test `FormatDuration` with various durations (seconds only, minutes+seconds, hours+minutes+seconds, zero).
- Test `FormatFailure` output matches expected format.
- Test `FormatRecovery` output includes downtime.
- Test `FormatEndpointList` with 0, 1, and multiple endpoints.

**Acceptance**: `go test ./internal/bot/` passes. All format functions produce correct output.

---

### Step 9: Telegram bot — `internal/bot/`

Create `internal/bot/bot.go`:
- `Bot` struct holding telebot.Bot, store, scheduler (set later), chatID, rootCtx.
- `NewBot(token string, chatID int64, store Store, rootCtx context.Context) (*Bot, error)`.
- `SetScheduler(scheduler *Scheduler)` — called after scheduler is created.
- `Start()` — starts the bot poller in a goroutine.
- `Stop()` — stops the bot poller.
- `TelegramNotifier` struct implementing `Notifier` interface. Sends formatted messages to chatID.

Create `internal/bot/handlers.go`:
- Register handlers for `/add`, `/delete`, `/status`, `/list`, `/interval`, `/help`.
- Each handler: parse args, validate, call store/scheduler, format response, reply.
- Use `errors.Is` for friendly error messages (e.g., `ErrDuplicate` → "This URL is already being monitored").

**No unit tests** for this step (requires Telegram API).

**Acceptance**: Code compiles. Bot registers all 6 command handlers.

---

### Step 10: Main entrypoint and health server — `cmd/monitor/`

Create `cmd/monitor/main.go`:
- `signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)` — this is the ONLY `context.Background()` call.
- Wire everything following the startup flow in DESIGN.md.
- Start `/healthz` HTTP server on `HEALTH_PORT` → responds with `200 {"status":"ok"}`.
- Graceful shutdown: stop bot → shutdown scheduler → shutdown health server → close DB.

**No unit tests** for this step.

**Acceptance**: `CGO_ENABLED=0 go build ./cmd/monitor/` succeeds. All quality gates pass.

---

### Step 11: Dockerfile and docker-compose

Create `Dockerfile`:
- Builder: `golang:1.26.1-alpine`, `CGO_ENABLED=0`, `-ldflags="-s -w"`.
- Runtime: `alpine:latest`, `ca-certificates`, `tzdata`, `curl`, non-root user, `/app/data` volume.
- `HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -f http://localhost:8080/healthz || exit 1`

Create `docker-compose.yml` as specified in DESIGN.md.

Update `.env.example` with all 7 environment variables.

**Acceptance**: `docker build .` succeeds (if Docker is available). Files match DESIGN.md specs.

---

### Step 12: README.md

Create `README.md` covering:
- Project description (one paragraph).
- Setup: environment variables, Docker usage.
- Bot commands reference.
- Architecture overview (brief).

**Acceptance**: README exists and covers setup, commands, and config.

---

## Completion

When ALL 12 steps are done, all quality gates pass, and every non-main/non-bot package has tests:

<promise>NOROSHI COMPLETE</promise>

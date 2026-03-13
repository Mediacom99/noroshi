# Noroshi — Design Document

Complete technical reference for architecture decisions and implementation patterns.

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
│   │   ├── bot.go                       # Bot struct, init, SetScheduler, TelegramNotifier
│   │   ├── handlers.go                  # Command handlers (/add, /delete, /status, /list, /interval, /help)
│   │   ├── format.go                    # Message formatting (failure, recovery, list, status, help)
│   │   └── format_test.go
│   ├── monitor/
│   │   ├── checker.go                   # retryablehttp-based health checker
│   │   ├── checker_test.go
│   │   ├── scheduler.go                 # gocron-based scheduler with check-and-notify logic
│   │   └── scheduler_test.go
│   └── storage/
│       ├── migrations/
│       │   └── 001_create_endpoints.sql # goose migration
│       ├── models.go                    # Endpoint struct
│       ├── store.go                     # SQLiteStore implementation
│       └── store_test.go
├── Dockerfile
├── docker-compose.yml
├── .env.example
├── go.mod
└── go.sum
```

## Startup Flow

1. `config.Load()` — read env vars, validate, return `Config` struct.
2. Open SQLite with `_journal_mode=WAL&_busy_timeout=5000`.
3. Run goose migrations (`goose.Up`).
4. Create `SQLiteStore` with the `*sql.DB`.
5. Create Telegram `Bot` (without scheduler — see Circular Dependency Resolution below).
6. Create `TelegramNotifier` from the bot (implements `Notifier` interface).
7. Create gocron `Scheduler` with store, checker, and notifier.
8. Call `bot.SetScheduler(scheduler)` to close the circular dependency.
9. Load all endpoints from DB → call `scheduler.Add` for each.
10. Start gocron scheduler (`s.Start()`).
11. Start Telegram bot poller (in a goroutine).
12. Start health HTTP server on `HEALTH_PORT` (in a goroutine).
13. Block on `<-ctx.Done()` (from `signal.NotifyContext`).
14. Graceful shutdown: stop bot, `scheduler.Shutdown()`, close DB.

## Circular Dependency Resolution

The bot needs the scheduler (to add/delete endpoints), and the scheduler needs the notifier (which wraps the bot for sending messages). Resolve this:

1. Create bot first (no scheduler yet).
2. Create notifier from bot (it only needs bot's `Send` capability).
3. Create scheduler with notifier.
4. Call `bot.SetScheduler(scheduler)` — bot stores scheduler reference for handlers.

## Graceful Shutdown

- `signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)` in `main.go` creates the root context. This is the ONLY place `context.Background()` is used.
- The root context flows to: gocron scheduler, bot handlers, checker HTTP requests.
- Shutdown sequence: cancel root ctx → stop bot poller → `scheduler.Shutdown()` (waits for running jobs) → close DB.

## gocron Scheduler Pattern

```go
import "github.com/go-co-op/gocron/v2"

// Create
s, err := gocron.NewScheduler()

// Start (non-blocking)
s.Start()

// Add a job for an endpoint
job, err := s.NewJob(
    gocron.DurationJob(time.Duration(endpoint.IntervalSeconds) * time.Second),
    gocron.NewTask(checkAndNotify, ctx, endpoint.ID),
    gocron.WithTags(fmt.Sprintf("endpoint-%d", endpoint.ID)),
)

// Remove a job by endpoint ID
s.RemoveByTags(fmt.Sprintf("endpoint-%d", endpoint.ID))

// Update interval: remove old job, add new one
s.RemoveByTags(fmt.Sprintf("endpoint-%d", endpoint.ID))
s.NewJob(gocron.DurationJob(newInterval), ...)

// Shutdown (blocks until running jobs finish)
s.Shutdown()
```

**DO NOT** use manual goroutines, `sync.Mutex`, cancel func maps, or `time.Ticker`. gocron handles all of this internally.

The `checkAndNotify` function (or method) is passed to `gocron.NewTask`. It:
1. Loads the endpoint from the store (to get current state).
2. Calls the checker to perform the HTTP health check.
3. Updates the endpoint status in the store.
4. Calls the notifier if notification rules apply (see Notification Behavior).

## retryablehttp Checker Pattern

```go
import "github.com/hashicorp/go-retryablehttp"

client := retryablehttp.NewClient()
client.RetryMax = 2
client.RetryWaitMin = 500 * time.Millisecond
client.RetryWaitMax = 2 * time.Second
client.HTTPClient.Timeout = cfg.HTTPTimeout
client.Logger = nil // silence default logger

req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
resp, err := client.Do(req)
```

retryablehttp retries on connection errors and 5xx responses by default. It does NOT retry 4xx. This eliminates false-positive alerts from transient network issues.

## goose Migrations Pattern

Migration file `internal/storage/migrations/001_create_endpoints.sql`:

```sql
-- +goose Up
CREATE TABLE endpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    interval_seconds INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'unknown',
    last_checked_at DATETIME,
    last_failure_at DATETIME,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    failure_notifications_sent INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE endpoints;
```

Running migrations in Go:

```go
import (
    "embed"
    "github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func RunMigrations(db *sql.DB) error {
    goose.SetBaseFS(embedMigrations)
    if err := goose.SetDialect("sqlite3"); err != nil {
        return err
    }
    return goose.Up(db, "migrations")
}
```

## Custom Error Types

```go
// internal/apperror/apperror.go

type AppError struct {
    Code    string
    Message string
    Cause   error
}

func (e *AppError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %v", e.Message, e.Cause)
    }
    return e.Message
}

func (e *AppError) Unwrap() error {
    return e.Cause
}

// Is compares Code for equality — this makes errors.Is work with Wrap'd errors.
func (e *AppError) Is(target error) bool {
    t, ok := target.(*AppError)
    if !ok {
        return false
    }
    return e.Code == t.Code
}

// Wrap clones a sentinel and attaches a cause.
func Wrap(sentinel *AppError, cause error) *AppError {
    return &AppError{
        Code:    sentinel.Code,
        Message: sentinel.Message,
        Cause:   cause,
    }
}

// Sentinels
var (
    ErrNotFound     = &AppError{Code: "NOT_FOUND", Message: "not found"}
    ErrDuplicate    = &AppError{Code: "DUPLICATE", Message: "already exists"}
    ErrInvalidInput = &AppError{Code: "INVALID_INPUT", Message: "invalid input"}
    ErrDatabase     = &AppError{Code: "DATABASE", Message: "database error"}
)
```

## Database Schema

See the goose migration file above. Key points:
- `url` is UNIQUE — enforces no duplicate endpoints.
- `consecutive_failures` and `failure_notifications_sent` track notification state per endpoint.
- `last_failure_at` records when the endpoint first went down (set on first failure, cleared on recovery). Used to calculate downtime duration.
- Connection string: `file:<path>?_journal_mode=WAL&_busy_timeout=5000`

## Store Interface

Defined at the point of use (in the packages that need it), not in `internal/storage/`. The canonical shape:

```go
type Store interface {
    AddEndpoint(ctx context.Context, url string, intervalSeconds int) (Endpoint, error)       // ErrDuplicate, ErrDatabase
    GetEndpoint(ctx context.Context, id int64) (Endpoint, error)                               // ErrNotFound, ErrDatabase
    GetEndpointByURL(ctx context.Context, url string) (Endpoint, error)                        // ErrNotFound, ErrDatabase
    DeleteEndpoint(ctx context.Context, id int64) error                                        // ErrNotFound, ErrDatabase
    ListEndpoints(ctx context.Context) ([]Endpoint, error)                                     // ErrDatabase
    UpdateEndpointStatus(ctx context.Context, id int64, status string, statusCode int) error   // ErrNotFound, ErrDatabase
    UpdateEndpointInterval(ctx context.Context, id int64, intervalSeconds int) error            // ErrNotFound, ErrDatabase
    RecordFailure(ctx context.Context, id int64, statusCode int) (Endpoint, error)             // ErrNotFound, ErrDatabase
    RecordRecovery(ctx context.Context, id int64, statusCode int) (Endpoint, error)            // ErrNotFound, ErrDatabase
}
```

- `RecordFailure` increments `consecutive_failures` and `failure_notifications_sent`, sets `last_failure_at` (on first failure), updates `status` and `last_checked_at`. Returns updated endpoint.
- `RecordRecovery` resets `consecutive_failures` and `failure_notifications_sent` to 0, clears `last_failure_at`, updates `status` and `last_checked_at`. Returns updated endpoint (with `last_failure_at` from BEFORE the reset, so downtime can be calculated).

## Telegram Bot Commands

### /add <url> <interval>
- Parse URL and interval from message text.
- Validate: URL must have http/https scheme. Interval must parse as Go duration and be >= 10s.
- Call `store.AddEndpoint`. On `ErrDuplicate` → friendly message. On `ErrInvalidInput` → show usage.
- Call `scheduler.Add(endpoint)` to start monitoring immediately.
- Reply with confirmation including endpoint ID, URL, and interval.

### /delete <id_or_url>
- Parse argument. Try as int64 (ID) first, then as URL.
- Look up endpoint via `store.GetEndpoint` or `store.GetEndpointByURL`.
- Call `scheduler.Remove(endpoint.ID)` to stop monitoring.
- Call `store.DeleteEndpoint`.
- Reply with confirmation.

### /status
- Call `store.ListEndpoints`.
- For each endpoint, perform an immediate health check via `checker.Check(ctx, url)`.
- Update status in store.
- Reply with formatted status of all endpoints.

### /list
- Call `store.ListEndpoints`.
- Reply with formatted list (no new checks triggered).

### /interval <id_or_url> <new_interval>
- Parse endpoint identifier and new interval.
- Validate interval >= 10s.
- Call `store.UpdateEndpointInterval`.
- Call `scheduler.Remove` then `scheduler.Add` with updated endpoint.
- Reply with confirmation.

### /help
- Reply with static help text showing all commands and usage examples.

## Notification Behavior

The check-and-notify logic runs inside the gocron job:

1. **Check**: Call `checker.Check(ctx, endpoint.URL)` → returns `(statusCode int, err error)`.
2. **Determine outcome**: `statusCode == 200` → OK. Anything else → NOT_OK.
3. **On NOT_OK**:
   - Call `store.RecordFailure(ctx, id, statusCode)` → returns updated endpoint.
   - If `endpoint.FailureNotificationsSent <= maxFailureNotifications`:
     - Call `notifier.NotifyFailure(ctx, endpoint)`.
4. **On OK, if endpoint was previously NOT_OK** (`endpoint.Status != "ok"` before this check):
   - Call `store.RecordRecovery(ctx, id, statusCode)` → returns endpoint with old `last_failure_at`.
   - Calculate downtime: `time.Since(endpoint.LastFailureAt)`.
   - Call `notifier.NotifyRecovery(ctx, endpoint, downtime)`.
5. **On OK, if endpoint was already OK**: Just update `last_checked_at`.

## Notifier Interface

```go
type Notifier interface {
    NotifyFailure(ctx context.Context, endpoint Endpoint) error
    NotifyRecovery(ctx context.Context, endpoint Endpoint, downtime time.Duration) error
}
```

`TelegramNotifier` implements this by formatting messages and sending them to the configured chat ID via telebot.

## Message Formats

### Failure
```
🔴 ENDPOINT DOWN
URL: {url}
Status: NOT_OK (HTTP {status_code})
Time: {timestamp UTC}
Consecutive failures: {failure_notifications_sent}/{max_failure_notifications}
```

### Recovery
```
🟢 ENDPOINT RECOVERED
URL: {url}
Status: OK (HTTP {status_code})
Downtime: {human_readable_duration}
Recovered at: {timestamp UTC}
```

### List / Status
```
📋 Monitored Endpoints

1. {url}
   Status: {status} | Interval: {interval} | Last check: {time}

2. {url}
   ...
```
If no endpoints: "No endpoints are being monitored."

### Help
```
📖 Available Commands

/add <url> <interval> — Add endpoint (e.g., /add https://example.com 30s)
/delete <id_or_url> — Remove endpoint
/status — Check all endpoints now
/list — List all endpoints
/interval <id_or_url> <interval> — Update check interval
/help — Show this message

Intervals: 10s, 30s, 1m, 5m, 1h, etc. (minimum 10s)
```

## Duration Formatting

`FormatDuration(d time.Duration) string` produces human-readable output:
- Under 1 minute: "45s"
- Under 1 hour: "12m 34s"
- 1 hour or more: "2h 15m 30s"
- Zero seconds components are omitted, but at least one component is always shown.

## Environment Variables

| Variable | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `TELEGRAM_TOKEN` | string | Yes | — | Bot token from @BotFather |
| `TELEGRAM_CHAT_ID` | int64 | Yes | — | Group chat ID for notifications |
| `DATABASE_PATH` | string | No | `./data/uptime.db` | SQLite database file path |
| `HTTP_TIMEOUT` | duration | No | `10s` | Health check HTTP timeout |
| `MAX_FAILURE_NOTIFICATIONS` | int | No | `3` | Stop notifying after N consecutive failures |
| `LOG_LEVEL` | string | No | `info` | Log level: debug, info, warn, error |
| `HEALTH_PORT` | int | No | `8080` | Port for /healthz HTTP endpoint |

## Health Endpoint

A minimal `net/http` server on `HEALTH_PORT`:
- `GET /healthz` → `200 OK` with body `{"status":"ok"}`
- Used by Docker HEALTHCHECK and Coolify.
- Runs in its own goroutine; shut down via `server.Shutdown(ctx)` during graceful shutdown.

## Docker

### Dockerfile
- **Builder**: `golang:1.26.1-alpine`, `CGO_ENABLED=0`, `-ldflags="-s -w"`.
- **Runtime**: `alpine:latest`, `ca-certificates`, `tzdata`, non-root user, `/app/data` volume.
- `HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -f http://localhost:8080/healthz || exit 1`
- Install `curl` in runtime stage for HEALTHCHECK.

### docker-compose.yml
```yaml
services:
  uptime-monitor:
    build: .
    restart: unless-stopped
    environment:
      - TELEGRAM_TOKEN=${TELEGRAM_TOKEN}
      - TELEGRAM_CHAT_ID=${TELEGRAM_CHAT_ID}
      - DATABASE_PATH=/app/data/uptime.db
      - MAX_FAILURE_NOTIFICATIONS=3
    volumes:
      - uptime-data:/app/data

volumes:
  uptime-data:
```

## Edge Cases

- **Duplicate URL on /add**: Return `ErrDuplicate` from store → handler replies with friendly message.
- **Invalid URL**: Validate scheme (http/https) before calling store. Return `ErrInvalidInput`.
- **Interval < 10s**: Reject with `ErrInvalidInput`.
- **Endpoint not found on /delete**: `ErrNotFound` → friendly message.
- **Container restart**: On startup, load all endpoints from DB and call `scheduler.Add` for each. gocron picks up where it left off.
- **Context cancellation mid-check**: retryablehttp respects context — cancelled checks return immediately.
- **DB errors**: Wrap with `ErrDatabase` sentinel. Log with slog. Reply with generic "internal error" to user.

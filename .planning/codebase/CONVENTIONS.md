# Coding Conventions

**Analysis Date:** 2026-03-26

## Naming Patterns

**Packages:**
- Short, lowercase, single-word names: `bot`, `monitor`, `storage`, `config`, `apperror`
- No underscores or camelCase in package names

**Files:**
- Lowercase with underscores for multi-word: `apperror.go`, `apperror_test.go`
- Functional grouping within packages: `bot.go` (core struct), `handlers.go` (command handlers), `callbacks.go` (inline keyboard callbacks), `format.go` (message formatting), `validate.go` (input validation)
- Models in a separate file: `internal/storage/models.go`

**Functions:**
- Exported: PascalCase (`NewHTTPChecker`, `FormatDuration`, `ValidateURL`)
- Unexported: camelCase (`checkAndNotify`, `htmlEscape`, `statusEmoji`, `isUniqueViolation`)
- Constructors: `New` prefix returning the struct pointer (`NewBot`, `NewSQLiteStore`, `NewHTTPChecker`, `NewScheduler`)

**Variables:**
- Struct fields: PascalCase for exported (`ID`, `URL`, `Name`), camelCase for unexported (`bot`, `store`, `rootCtx`)
- Local variables: short camelCase (`ep`, `cfg`, `srv`, `tb`)
- Abbreviations stay uppercase: `ID`, `URL`, `HTTP`

**Types:**
- Structs: PascalCase nouns (`Bot`, `Scheduler`, `HTTPChecker`, `SQLiteStore`, `Endpoint`)
- Interfaces: named by method set or purpose (`Store`, `Scheduler`, `Notifier`)
- Error type: `AppError` with pointer receiver

**Constants:**
- Unexported callback identifiers use short abbreviations: `cbDetail = "dtl"`, `cbDelete = "del"`, `cbConfirmDelete = "cdel"`

## Error Handling

**Custom Error Type:**
- Defined in `internal/apperror/apperror.go`
- Struct with `Code` (string), `Message` (string), `Cause` (error)
- Implements `Error()`, `Unwrap()`, and `Is(target error) bool` (compares `Code` field)

**Sentinel Errors:**
- `apperror.ErrNotFound` (Code: `"NOT_FOUND"`)
- `apperror.ErrDuplicate` (Code: `"DUPLICATE"`)
- `apperror.ErrInvalidInput` (Code: `"INVALID_INPUT"`)
- `apperror.ErrDatabase` (Code: `"DATABASE"`)

**Wrapping Pattern:**
- Use `apperror.Wrap(sentinel, cause)` to clone a sentinel and attach a root cause
- Always check errors with `errors.Is()` -- never compare error strings

```go
// Wrapping: clone sentinel + attach cause
if err == sql.ErrNoRows {
    return Endpoint{}, apperror.Wrap(apperror.ErrNotFound, err)
}

// Checking: always use errors.Is
if errors.Is(err, apperror.ErrDuplicate) {
    return c.Send("This name or URL is already being monitored.")
}
```

**Standard Library Wrapping:**
- For non-sentinel errors, use `fmt.Errorf("context: %w", err)` (seen in `cmd/monitor/main.go`, `internal/monitor/scheduler.go`)

**Error Logging:**
- Use `slog.Error("action description", "error", err)` for operational errors
- Use `slog.Error("action description", "id", id, "error", err)` when entity context is available

## Code Organization

**Interface Placement:**
- Define interfaces at the point of use (consumer side), not in the implementing package
- `internal/bot/bot.go` defines `Store` interface (consumed by bot, implemented by `storage.SQLiteStore`)
- `internal/bot/bot.go` defines `Scheduler` interface (consumed by bot, implemented by `monitor.Scheduler`)
- `internal/monitor/scheduler.go` defines `Store` interface (a different, narrower set of methods than bot's `Store`)
- `internal/monitor/scheduler.go` defines `Notifier` interface (consumed by scheduler, implemented by `bot.TelegramNotifier`)

```go
// In internal/bot/bot.go -- only the methods the bot needs
type Store interface {
    AddEndpoint(ctx context.Context, name, url string, intervalSeconds int) (storage.Endpoint, error)
    GetEndpoint(ctx context.Context, id int64) (storage.Endpoint, error)
    // ...
}

// In internal/monitor/scheduler.go -- only the methods the scheduler needs
type Store interface {
    GetEndpoint(ctx context.Context, id int64) (storage.Endpoint, error)
    UpdateEndpointStatus(ctx context.Context, id int64, status string, statusCode int) error
    // ...
}
```

**Context Propagation:**
- Root context created in `cmd/monitor/main.go` via `signal.NotifyContext(context.Background(), ...)`
- `context.Background()` is used ONLY in `main.go`
- The root context is passed to `Bot` via constructor and stored as `rootCtx`
- All I/O functions take `context.Context` as the first parameter
- In tests, `context.Background()` is acceptable

**Dependency Injection:**
- All dependencies injected via constructors (`NewBot`, `NewScheduler`, `NewSQLiteStore`)
- No global mutable state
- Circular dependency between `Bot` and `Scheduler` resolved by `SetScheduler()` method called after both are constructed
- `Scheduler` field checked for nil before use: `if b.scheduler != nil { ... }`

**Package Layering:**
- `cmd/monitor/` -- entry point, wiring, config loading
- `internal/config/` -- env var reading, validation, defaults
- `internal/apperror/` -- error types and sentinels
- `internal/storage/` -- data models, SQLite store, migrations
- `internal/monitor/` -- HTTP checker, scheduler (gocron), check-and-notify logic
- `internal/bot/` -- Telegram bot, command handlers, callbacks, formatting, validation

## Formatting & Style

**Formatting:**
- Standard `gofmt` (no custom formatter configuration detected)
- No `.golangci.yml` or other linter configs present

**Linting:**
- `go vet ./...` required before every commit (per `CLAUDE.md`)
- No golangci-lint or other third-party linters configured

**Import Grouping (3 groups, separated by blank lines):**
1. Standard library (`context`, `fmt`, `log/slog`, `database/sql`, etc.)
2. Internal project packages (`noroshi/internal/...`)
3. External dependencies (`github.com/...`, `gopkg.in/...`, `modernc.org/...`)

```go
import (
    "context"
    "fmt"
    "log/slog"
    "time"

    "noroshi/internal/storage"

    "github.com/go-co-op/gocron/v2"
)
```

**Import Aliases:**
- `tele "gopkg.in/telebot.v4"` -- short alias for the Telegram bot library
- Blank import for SQLite driver: `_ "modernc.org/sqlite"`

**Function Ordering within Files:**
- Exported types and constructors first
- Exported methods second
- Unexported helper functions last
- In `bot.go`: type definition -> `NewBot` -> `SetScheduler` -> `Start`/`Stop` -> `registerCommands` -> `guarded` -> `SendMessage`/`SendSilentMessage` -> `TelegramNotifier`

**Comments:**
- Godoc-style comments on all exported types and functions
- Brief, single-line comments for most exported items
- No comments on unexported functions unless the logic is non-obvious
- Inline comments used sparingly to explain "why" not "what"

```go
// HTTPChecker performs HTTP health checks using retryablehttp.
type HTTPChecker struct { ... }

// NewHTTPChecker creates a HTTPChecker with retryablehttp configured per DESIGN.md.
func NewHTTPChecker(timeout time.Duration) *HTTPChecker { ... }

// Return the last response instead of an error after retries exhausted
client.ErrorHandler = retryablehttp.PassthroughErrorHandler
```

## Logging

**Framework:** `log/slog` (stdlib)

**Patterns:**
- Use structured key-value pairs: `slog.Error("action", "key", value, "error", err)`
- Action descriptions are short lowercase phrases: `"load config"`, `"add endpoint"`, `"scheduler: get endpoint"`
- Prefix with component name for scheduler logs: `"scheduler: record failure"`
- Use `slog.Info` for lifecycle events: `"telegram bot started"`, `"loaded endpoints"`

```go
slog.Error("scheduler: get endpoint", "id", endpointID, "error", err)
slog.Info("loaded endpoints", "count", len(endpoints))
slog.Info("shutting down...")
```

## Configuration

**Approach:** `os.Getenv()` exclusively -- no config libraries
- Required vars fail fast with descriptive error messages
- Optional vars have sensible defaults
- All config loaded in one place: `internal/config/config.go`
- Config struct is plain data -- no methods besides the `Load()` constructor function

## Database Migrations

**Tool:** goose with embedded SQL files
- Migration files in `internal/storage/migrations/`
- Naming: `NNN_description.sql` (e.g., `001_create_endpoints.sql`)
- Each file has `-- +goose Up` and `-- +goose Down` sections
- Migrations embedded via `//go:embed migrations/*.sql`
- Never use inline `CREATE TABLE` statements in Go code

## Telegram Message Formatting

**Parse Mode:** HTML (set globally in bot settings)
- Always HTML-escape user-provided content with `html.EscapeString()`
- Use `<b>`, `<code>` tags for formatting
- Use emoji prefixes for visual status indicators

---

*Convention analysis: 2026-03-26*

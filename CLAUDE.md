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

- Every non-main package MUST have `_test.go` files. Exception: `internal/bot/` (requires Telegram API).
- Use stdlib `testing` only — no testify, no gomock.
- Use table-driven tests where applicable.
- `go test ./...` MUST pass before every commit.

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

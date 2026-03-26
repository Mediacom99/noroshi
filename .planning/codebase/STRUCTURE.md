# Project Structure

**Analysis Date:** 2026-03-26

## Directory Layout

```
noroshi/
├── cmd/
│   └── monitor/
│       └── main.go              # Application entry point (145 lines)
├── internal/
│   ├── apperror/
│   │   ├── apperror.go          # AppError type and sentinel errors (47 lines)
│   │   └── apperror_test.go     # Error matching and wrapping tests (63 lines)
│   ├── bot/
│   │   ├── bot.go               # Bot struct, TelegramNotifier, lifecycle (140 lines)
│   │   ├── handlers.go          # Slash command handlers: /add, /delete, /list, /interval, /help (169 lines)
│   │   ├── callbacks.go         # Inline keyboard callback handlers (193 lines)
│   │   ├── format.go            # Message formatting and inline keyboard builders (197 lines)
│   │   ├── format_test.go       # Format function tests (267 lines)
│   │   ├── validate.go          # URL validation (30 lines)
│   │   └── validate_test.go     # URL validation tests (34 lines)
│   ├── config/
│   │   ├── config.go            # Config struct and Load() from env vars (80 lines)
│   │   └── config_test.go       # Config loading and validation tests (127 lines)
│   ├── monitor/
│   │   ├── checker.go           # HTTPChecker using retryablehttp (43 lines)
│   │   ├── checker_test.go      # Checker tests with httptest server (71 lines)
│   │   ├── scheduler.go         # gocron Scheduler with check-and-notify loop (137 lines)
│   │   └── scheduler_test.go    # Scheduler tests with mock store/notifier (265 lines)
│   └── storage/
│       ├── models.go            # Endpoint struct (20 lines)
│       ├── store.go             # SQLiteStore, OpenDB, RunMigrations (265 lines)
│       ├── store_test.go        # Store CRUD tests with in-memory SQLite (364 lines)
│       └── migrations/
│           ├── 001_create_endpoints.sql   # Initial endpoints table (15 lines)
│           └── 002_add_endpoint_name.sql  # Add name column with table rebuild (44 lines)
├── data/
│   └── uptime.db               # SQLite database (runtime, gitignored except dir)
├── .env                        # Environment config (exists, never read)
├── .env.example                # Documented env var template
├── CLAUDE.md                   # Project rules and conventions
├── DESIGN.md                   # Design document
├── PROMPT.md                   # Original project prompt
├── TODO.md                     # Tracked TODOs
├── README.md                   # Project readme
├── Dockerfile                  # Multi-stage build (alpine)
├── docker-compose.yml          # Single-service compose with named volume
├── entrypoint.sh               # Volume permission fix + su-exec
├── go.mod                      # Go module definition
├── go.sum                      # Dependency checksums
├── monitor                     # Compiled binary (gitignored)
└── .gitignore                  # Ignore rules
```

## Package Dependency Graph

```
cmd/monitor/main.go
    ├── internal/config         (no internal deps)
    ├── internal/storage        (imports internal/apperror)
    ├── internal/monitor        (imports internal/storage for Endpoint type)
    └── internal/bot            (imports internal/storage for Endpoint type, internal/apperror)
```

**Detailed import chains:**

- `internal/config` -- standalone, no internal dependencies
- `internal/apperror` -- standalone, no internal dependencies
- `internal/storage` -- depends on `internal/apperror`
- `internal/monitor` -- depends on `internal/storage` (for `storage.Endpoint` type only)
- `internal/bot` -- depends on `internal/storage` (for `storage.Endpoint` type), `internal/apperror`
- `internal/monitor` and `internal/bot` do NOT import each other (connected via interfaces in `main.go`)

## Key Files

| File | Purpose | Lines |
|------|---------|-------|
| `cmd/monitor/main.go` | Entry point: wires deps, manages lifecycle, health endpoint | 145 |
| `internal/config/config.go` | Loads `Config` struct from env vars with defaults | 80 |
| `internal/apperror/apperror.go` | `AppError` type with Code/Message/Cause, sentinels, `Wrap()` | 47 |
| `internal/storage/models.go` | `Endpoint` struct (the single domain model) | 20 |
| `internal/storage/store.go` | `SQLiteStore` with all CRUD operations, `OpenDB`, `RunMigrations` | 265 |
| `internal/monitor/checker.go` | `HTTPChecker` wrapping retryablehttp client | 43 |
| `internal/monitor/scheduler.go` | `Scheduler` managing gocron jobs, `checkAndNotify` core loop | 137 |
| `internal/bot/bot.go` | `Bot` struct, `TelegramNotifier`, `guarded()` middleware | 140 |
| `internal/bot/handlers.go` | Slash commands: `/add`, `/delete`, `/list`, `/interval`, `/help` | 169 |
| `internal/bot/callbacks.go` | Inline keyboard callbacks: detail, delete, interval, refresh | 193 |
| `internal/bot/format.go` | HTML message formatting, inline keyboard builders, constants | 197 |
| `internal/bot/validate.go` | `ValidateURL()` for http/https with domain check | 30 |

## Directory Purposes

**`cmd/monitor/`:**
- Purpose: Single binary entry point
- Contains: `main.go` only
- Key responsibilities: dependency wiring, signal handling, health HTTP server, logging setup

**`internal/apperror/`:**
- Purpose: Shared error types used across packages
- Contains: `AppError` struct, sentinel errors, `Wrap()` helper
- Key files: `apperror.go`

**`internal/config/`:**
- Purpose: Environment-based configuration loading
- Contains: `Config` struct definition and `Load()` function
- Key files: `config.go`

**`internal/storage/`:**
- Purpose: Database access layer (SQLite via modernc.org/sqlite)
- Contains: Domain models, CRUD operations, embedded SQL migrations
- Key files: `store.go` (main logic), `models.go` (Endpoint struct), `migrations/*.sql`

**`internal/monitor/`:**
- Purpose: Health check execution and scheduling
- Contains: HTTP checker (retryablehttp), gocron scheduler, check-and-notify logic
- Key files: `scheduler.go` (core monitoring loop), `checker.go` (HTTP checks)

**`internal/bot/`:**
- Purpose: Telegram bot interface (commands, inline keyboards, notifications)
- Contains: Bot lifecycle, command handlers, callback handlers, message formatting, URL validation
- Key files: `bot.go` (struct + notifier), `handlers.go` (commands), `callbacks.go` (inline buttons), `format.go` (message templates)

**`internal/storage/migrations/`:**
- Purpose: Goose SQL migration files (embedded at compile time)
- Contains: Versioned `.sql` files
- Generated: No (hand-written)
- Committed: Yes

**`data/`:**
- Purpose: Runtime SQLite database storage
- Contains: `uptime.db`
- Generated: Yes (at runtime)
- Committed: No (directory structure only)

## Naming Conventions

**Files:**
- Lowercase, underscore-separated: `store_test.go`, `checker.go`, `format_test.go`
- Test files co-located with source: `{name}_test.go` next to `{name}.go`
- Migration files: `NNN_description.sql` (e.g., `001_create_endpoints.sql`)

**Directories:**
- Lowercase, singular: `monitor`, `storage`, `bot`, `config`, `apperror`
- Standard Go layout: `cmd/` for binaries, `internal/` for private packages

## Where to Add New Code

**New Telegram command:**
- Handler: `internal/bot/handlers.go` (add `handle{Name}` method)
- Register: `internal/bot/handlers.go:17-24` (in `registerHandlers()`)
- Menu entry: `internal/bot/bot.go:83-89` (in `registerCommands()`)
- Formatting: `internal/bot/format.go` (add `Format{Name}()` function)
- Tests: `internal/bot/format_test.go` (for formatting), handler tests are skipped (Telegram API dependency)

**New inline keyboard callback:**
- Callback constant: `internal/bot/format.go:16-24` (add `cb{Name}` constant)
- Handler: `internal/bot/callbacks.go` (add `handle{Name}Callback` method)
- Register: `internal/bot/callbacks.go:13-21` (in `registerCallbacks()`)

**New storage operation:**
- Method: `internal/storage/store.go` (add method on `SQLiteStore`)
- Interface update: Add to the relevant `Store` interface in `internal/monitor/scheduler.go` or `internal/bot/bot.go` (whichever needs it)
- Tests: `internal/storage/store_test.go`

**New database migration:**
- File: `internal/storage/migrations/NNN_description.sql`
- Format: goose-style with `-- +goose Up` and `-- +goose Down` markers
- Auto-embedded via `//go:embed migrations/*.sql` in `internal/storage/store.go:17`

**New monitor capability:**
- Implementation: `internal/monitor/` (new file or extend existing)
- Tests: `internal/monitor/{name}_test.go`

**New domain model:**
- Struct: `internal/storage/models.go`
- Migration: `internal/storage/migrations/NNN_description.sql`

**New utility/helper:**
- If bot-specific: `internal/bot/` (e.g., `validate.go` pattern)
- If storage-specific: `internal/storage/`
- No shared `utils` package exists; place helpers in the package that uses them

## Special Directories

**`.planning/`:**
- Purpose: Planning and analysis documents
- Generated: By tooling
- Committed: Yes

**`data/`:**
- Purpose: Runtime SQLite database
- Generated: At application startup
- Committed: Directory only (`.gitignore` excludes `*.db`)

---

*Structure analysis: 2026-03-26*

# Uptime Monitor — Claude Code Plan Mode Prompt

## Project Overview

Build a **simple, self-contained uptime monitor** in Go that uses a **Telegram bot** as its only interface (no web frontend). The bot is added to a Telegram group and any member of that group can issue commands to manage monitored endpoints. The service runs as a single Docker container deployed on **Coolify** (Hetzner VPS) with **SQLite** for persistence.

## Core Requirements

### Telegram Bot Commands

The bot must handle these commands (sent in the Telegram group):

1. **`/add <url> <interval>`** — Add a new endpoint to monitor. Interval is a human-friendly duration (e.g., `30s`, `5m`, `1h`). The bot confirms with endpoint details and starts monitoring immediately.

2. **`/delete <id_or_url>`** — Remove an endpoint from monitoring. Stops its health check goroutine and deletes it from the database. The bot confirms deletion.

3. **`/status`** — Triggers an immediate health check of ALL monitored endpoints and returns a formatted message showing each endpoint with its current status (OK / NOT_OK), last check time, and configured interval.

4. **`/list`** — Lists all monitored endpoints with their configuration (URL, interval, current status) without triggering new checks.

5. **`/interval <id_or_url> <new_interval>`** — Update the check interval for an existing endpoint. Restarts its monitoring goroutine with the new interval.

6. **`/help`** — Shows all available commands with usage examples.

### Health Check Logic

- Make an HTTP GET request to the endpoint URL.
- **200** response → status is **OK**.
- **Anything else** (non-200, timeout, DNS failure, connection refused, etc.) → status is **NOT_OK**.
- Use a configurable HTTP timeout per request (env var, default 10s).

### Notification Behavior

- **Immediate notification** on first failure of an endpoint.
- **Consecutive failure cap**: After N consecutive failure notifications (N is configurable via env var, e.g., `MAX_FAILURE_NOTIFICATIONS=3`), stop sending failure notifications for that endpoint until it recovers.
- **Recovery notification**: When a previously-down endpoint returns to OK, send a recovery message including how long it was down (e.g., "endpoint X is back online after 12 minutes").
- **Normal behavior resumes** after recovery — next failure triggers notifications again from scratch.

### Notification Message Format

Failure:
```
🔴 ENDPOINT DOWN
URL: https://api.example.com/health
Status: NOT_OK (HTTP 503)
Time: 2026-03-13 14:32:05 UTC
Consecutive failures: 2/3
```

Recovery:
```
🟢 ENDPOINT RECOVERED
URL: https://api.example.com/health
Status: OK (HTTP 200)
Downtime: 12m 34s
Recovered at: 2026-03-13 14:44:39 UTC
```

## Technology Stack

| Component | Choice | Import Path / Notes |
|-----------|--------|-------------------|
| Language | Go 1.26.1 | Latest stable |
| Telegram Bot | telebot v4 | `gopkg.in/telebot.v4` |
| SQLite Driver | modernc.org/sqlite | Pure Go, no CGO required |
| SQL Interface | `database/sql` | stdlib |
| Scheduling | `time.Ticker` + goroutines + `context` | stdlib, zero external deps |
| HTTP Client | `net/http` | stdlib |
| Logging | `log/slog` | stdlib structured logging (Go 1.21+) |
| Docker | Multi-stage build | `golang:1.26-alpine` → `alpine:latest` |
| Deployment | Coolify | Dockerfile build pack |

## Project Structure

```
uptime-monitor/
├── cmd/
│   └── monitor/
│       └── main.go              # Entry point, wiring, graceful shutdown
├── internal/
│   ├── config/
│   │   └── config.go            # Load config from env vars
│   ├── bot/
│   │   ├── bot.go               # Bot initialization and lifecycle
│   │   ├── handlers.go          # Command handlers (/add, /delete, /status, etc.)
│   │   └── format.go            # Message formatting helpers
│   ├── monitor/
│   │   ├── scheduler.go         # Manages all endpoint monitor goroutines
│   │   ├── checker.go           # HTTP health check logic
│   │   └── worker.go            # Single endpoint monitoring loop (ticker + context)
│   └── storage/
│       ├── sqlite.go            # DB initialization, migrations, connection
│       └── endpoint.go          # Endpoint CRUD operations
├── Dockerfile
├── docker-compose.yml           # For local dev and Coolify reference
├── .env.example                 # Document all env vars
├── go.mod
├── go.sum
└── README.md
```

## Architecture & Design

### Startup Flow

1. Load configuration from environment variables.
2. Initialize SQLite database (create tables if not exist, run migrations).
3. Initialize Telegram bot with long polling.
4. Load all existing endpoints from the database.
5. Start a monitoring goroutine (worker) for each endpoint.
6. Start the Telegram bot poller.
7. Block on OS signal (SIGINT/SIGTERM) for graceful shutdown.

### Graceful Shutdown

Use `signal.NotifyContext` to create a root context. All goroutines (bot poller, monitor workers) receive this context and stop cleanly when cancelled. Use `sync.WaitGroup` to wait for all goroutines to finish before exiting.

### Monitor Scheduler

The scheduler manages a map of endpoint ID → cancel function. When an endpoint is added, it spawns a new goroutine with its own `time.Ticker`. When deleted or interval changes, it cancels the existing goroutine (and starts a new one for interval changes). The scheduler must be safe for concurrent access (use `sync.Mutex`).

### Storage Layer

Use `database/sql` with `modernc.org/sqlite` driver. Keep it simple:

**Endpoints table:**
```sql
CREATE TABLE IF NOT EXISTS endpoints (
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
```

No need for a check history table — keep it minimal. The `consecutive_failures` and `failure_notifications_sent` fields track notification state.

### Bot ↔ Monitor Communication

The bot handlers call the scheduler directly (dependency injection). For example, `/add` handler → calls `scheduler.Add(endpoint)` which persists to DB and starts the goroutine. Keep it synchronous and simple — no message queues or channels needed.

### Telegram Bot Configuration

- The bot uses **long polling** (not webhooks) to avoid needing a public URL/TLS.
- The bot accepts commands from **any member** of the group it's added to.
- The bot must be configured with the **group chat ID** via env var (`TELEGRAM_CHAT_ID`) so it knows where to send proactive notifications (failures/recoveries). Commands are processed from any chat that messages the bot, but notifications only go to the configured group.

## Environment Variables

```
TELEGRAM_TOKEN=your-bot-token          # Required. Bot token from @BotFather
TELEGRAM_CHAT_ID=-100123456789         # Required. Group chat ID for notifications
DATABASE_PATH=/app/data/uptime.db      # Optional. Default: ./data/uptime.db
HTTP_TIMEOUT=10s                       # Optional. Health check HTTP timeout. Default: 10s
MAX_FAILURE_NOTIFICATIONS=3            # Optional. Stop notifying after N consecutive failures. Default: 3
LOG_LEVEL=info                         # Optional. debug/info/warn/error. Default: info
```

## Dockerfile

Multi-stage build:
- **Builder**: `golang:1.26-alpine`, `CGO_ENABLED=0`, build with `-ldflags="-s -w"` for small binary.
- **Runtime**: `alpine:latest`, include `ca-certificates` and `tzdata`, create non-root user, set `/app/data` as the volume mount point.
- Add a `HEALTHCHECK` instruction (the app should expose an internal health endpoint or the binary should support a `health` subcommand).

## docker-compose.yml

For local development and as a Coolify reference:
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

## Clean Code Principles to Follow

1. **Single Responsibility**: Each package owns one concern (bot, monitor, storage, config).
2. **Dependency Injection**: Pass dependencies explicitly (no global state). Main wires everything together.
3. **Error Handling**: Always handle errors. Use `fmt.Errorf("context: %w", err)` for wrapping. Log errors with `slog`.
4. **Naming**: Use clear, descriptive Go names. No abbreviations except well-known ones (URL, HTTP, DB, ID).
5. **Small Functions**: Each function does one thing. Handlers delegate to services.
6. **No Magic**: Configuration is explicit via env vars. No hidden defaults without documentation.
7. **Interfaces where needed**: Define interfaces for storage and checker to allow testing (but don't over-abstract — this is a small project).

## Implementation Order

1. `internal/config/config.go` — env var loading with defaults and validation
2. `internal/storage/sqlite.go` — DB init, migrations
3. `internal/storage/endpoint.go` — CRUD operations
4. `internal/monitor/checker.go` — HTTP health check function
5. `internal/monitor/worker.go` — single endpoint monitoring loop
6. `internal/monitor/scheduler.go` — manage all workers
7. `internal/bot/format.go` — message formatting
8. `internal/bot/bot.go` — bot init and lifecycle
9. `internal/bot/handlers.go` — all command handlers
10. `cmd/monitor/main.go` — wire everything, graceful shutdown
11. `Dockerfile` — multi-stage build
12. `docker-compose.yml` — local dev setup
13. `.env.example` — document all env vars
14. `README.md` — setup instructions, Coolify deployment guide

## Edge Cases to Handle

- **Duplicate URL on /add**: Return friendly error, don't add twice.
- **Invalid URL format**: Validate URL before adding.
- **Invalid interval**: Reject intervals < 10s (configurable minimum) to prevent abuse.
- **Bot removed from group**: Handle gracefully, log warning.
- **SQLite locked**: Use `_journal_mode=WAL` and `_busy_timeout=5000` in connection string for concurrent read/write.
- **Endpoint URL unreachable on first add**: Still add it, first check will catch the failure.
- **Container restart**: On startup, load all endpoints from DB and resume monitoring with their configured intervals.
- **Very long downtime**: Downtime duration in recovery message should be human-readable (e.g., "2h 15m 30s" not "8130s").

## Testing Strategy

- Unit tests for: checker logic, message formatting, config parsing, storage CRUD.
- Use interfaces for HTTP client and storage to enable mocking.
- Integration test: full flow with in-memory SQLite (`:memory:`).
- Keep tests simple — this is a small project, not enterprise software.

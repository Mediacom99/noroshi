# External Integrations

**Analysis Date:** 2026-03-26

## APIs & Services

### Telegram Bot API

**Purpose:** Primary user interface. Users interact with the monitor entirely through Telegram commands and inline keyboard buttons. Also used to send failure/recovery alert notifications.

**SDK/Client:** `gopkg.in/telebot.v4` v4.0.0-beta.7

**Auth:** `TELEGRAM_TOKEN` env var (Bot API token from @BotFather)

**Configuration:**
- Token: `TELEGRAM_TOKEN` (required)
- Target chat: `TELEGRAM_CHAT_ID` (required, int64)
- Polling: Long polling with 10-second timeout (not webhooks)
- Parse mode: HTML

**Usage locations:**
- `internal/bot/bot.go` - Bot initialization, `NewBot()`, `Start()`, `Stop()`, message sending (`SendMessage`, `SendSilentMessage`), `TelegramNotifier` for scheduler callbacks
- `internal/bot/handlers.go` - Command handlers: `/add`, `/delete`, `/list`, `/interval`, `/help`
- `internal/bot/callbacks.go` - Inline keyboard callback handlers: detail view, delete confirmation, interval picker, refresh, back navigation
- `internal/bot/format.go` - HTML message formatting for all bot responses, inline keyboard markup construction

**Bot commands registered:**
| Command | Description | Handler |
|---------|-------------|---------|
| `/list` | View all monitored endpoints | `handleList` in `internal/bot/handlers.go` |
| `/add` | Add endpoint: `<name> <url> [interval]` | `handleAdd` in `internal/bot/handlers.go` |
| `/delete` | Remove an endpoint by name or ID | `handleDelete` in `internal/bot/handlers.go` |
| `/interval` | Change check interval by name or ID | `handleInterval` in `internal/bot/handlers.go` |
| `/help` | Show help and usage info | `handleHelp` in `internal/bot/handlers.go` |

**Inline keyboard callbacks:**
| Unique ID | Purpose | Handler |
|-----------|---------|---------|
| `dtl` | Show endpoint detail view | `handleDetailCallback` in `internal/bot/callbacks.go` |
| `del` | Prompt delete confirmation | `handleDeleteCallback` in `internal/bot/callbacks.go` |
| `cdel` | Confirm and execute delete | `handleConfirmDeleteCallback` in `internal/bot/callbacks.go` |
| `back` | Return to endpoint list | `handleBackCallback` in `internal/bot/callbacks.go` |
| `intv` | Show interval picker | `handleIntervalCallback` in `internal/bot/callbacks.go` |
| `sint` | Apply selected interval | `handleSetIntervalCallback` in `internal/bot/callbacks.go` |
| `ref` | Refresh endpoint list | `handleRefreshCallback` in `internal/bot/callbacks.go` |

**Notification behavior:**
- Failure alerts: Sent with notification sound via `SendMessage`, capped at `MAX_FAILURE_NOTIFICATIONS` consecutive alerts
- Recovery alerts: Sent silently (no sound) via `SendSilentMessage`, includes downtime duration
- Chat guard: All handlers wrapped with `guarded()` which ignores messages from chats other than the configured `TELEGRAM_CHAT_ID`

### Monitored Endpoints (Outbound HTTP)

**Purpose:** The application makes outbound HTTP GET requests to user-configured URLs to check their availability.

**Client:** `github.com/hashicorp/go-retryablehttp` v0.7.8

**Configuration:**
- Timeout: `HTTP_TIMEOUT` env var (default: `10s`)
- Retry: Up to 2 retries, 500ms-2s backoff
- Error handler: `PassthroughErrorHandler` (returns last response instead of error after retries exhausted)
- Logger: Disabled (`client.Logger = nil`)

**Usage locations:**
- `internal/monitor/checker.go` - `HTTPChecker` struct, `NewHTTPChecker()`, `Check(ctx, url)` method
- `internal/monitor/scheduler.go` - `checkAndNotify()` calls `checker.Check()` on each scheduled tick

**Health check logic (in `internal/monitor/scheduler.go`):**
1. `gocron` fires the job at the configured interval
2. `checkAndNotify()` fetches the endpoint from the store
3. `HTTPChecker.Check()` sends a GET request with retries
4. Status code 200 = OK; anything else (or connection error) = NOT_OK
5. State transitions trigger notifications via the `Notifier` interface

## Database

### SQLite

**Type:** SQLite (file-based relational database)

**Driver:** `modernc.org/sqlite` v1.46.1 (pure Go, no CGO)

**Connection:** `DATABASE_PATH` env var (default: `./data/uptime.db`)

**DSN format:** `file:<path>?_journal_mode=WAL&_busy_timeout=5000`

**Connection settings:**
- WAL journal mode for concurrent read/write
- 5-second busy timeout for lock contention

**Usage locations:**
- `internal/storage/store.go` - `OpenDB()`, `RunMigrations()`, `SQLiteStore` implementation
- `internal/storage/models.go` - `Endpoint` model struct
- `cmd/monitor/main.go` - Database lifecycle (open, migrate, close)

**Migration strategy:**
- Tool: `github.com/pressly/goose/v3` v3.27.0
- SQL files embedded via `//go:embed migrations/*.sql`
- Migrations run automatically on startup via `storage.RunMigrations(db)`
- Location: `internal/storage/migrations/`

**Schema (current, after migration 002):**

```sql
CREATE TABLE endpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
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

**Migration history:**
| File | Purpose |
|------|---------|
| `internal/storage/migrations/001_create_endpoints.sql` | Initial endpoints table (without name column) |
| `internal/storage/migrations/002_add_endpoint_name.sql` | Adds `name` column (UNIQUE), rebuilds table due to SQLite ALTER TABLE limitations, backfills existing rows with `endpoint-<id>` |

**Store operations (defined in `internal/storage/store.go`):**
- `AddEndpoint(ctx, name, url, intervalSeconds)` - Insert with duplicate detection
- `GetEndpoint(ctx, id)` - Lookup by primary key
- `GetEndpointByURL(ctx, url)` - Lookup by URL
- `GetEndpointByName(ctx, name)` - Lookup by name
- `DeleteEndpoint(ctx, id)` - Delete with not-found detection
- `ListEndpoints(ctx)` - All endpoints ordered by ID
- `UpdateEndpointStatus(ctx, id, status, statusCode)` - Update status and last_checked_at
- `UpdateEndpointInterval(ctx, id, intervalSeconds)` - Change check interval
- `RecordFailure(ctx, id, statusCode)` - Increment failure counters, set last_failure_at on first failure
- `RecordRecovery(ctx, id, statusCode)` - Reset failure counters, clear last_failure_at

## Health Check Endpoint

**Purpose:** Liveness probe for container orchestration (Docker HEALTHCHECK, Coolify).

**Implementation:** stdlib `net/http` server in `cmd/monitor/main.go` (`startHealthServer()`)

**Endpoint:** `GET /healthz` on port `HEALTH_PORT` (default: 8080)

**Response:** `{"status": "ok"}` with HTTP 200

**Docker HEALTHCHECK:** `curl -f http://localhost:8080/healthz` every 30s, 5s timeout, 3 retries

## Environment Configuration

**Required env vars:**

| Variable | Type | Purpose |
|----------|------|---------|
| `TELEGRAM_TOKEN` | string | Telegram Bot API token from @BotFather |
| `TELEGRAM_CHAT_ID` | int64 | Target Telegram chat/group ID for commands and notifications |

**Optional env vars:**

| Variable | Type | Default | Purpose |
|----------|------|---------|---------|
| `DATABASE_PATH` | string | `./data/uptime.db` | Path to SQLite database file |
| `HTTP_TIMEOUT` | duration | `10s` | Timeout for health check HTTP requests |
| `MAX_FAILURE_NOTIFICATIONS` | int | `3` | Max consecutive failure notifications before silencing |
| `LOG_LEVEL` | string | `info` | Log level: debug, info, warn, error |
| `HEALTH_PORT` | int | `8080` | Port for the `/healthz` liveness endpoint |

**Secrets location:**
- `.env` file in project root (gitignored, local dev only)
- `.env.example` documents all variables with placeholder values
- Docker Compose injects vars from host environment / `.env` file
- Production: Environment variables injected by Coolify deployment platform

## Monitoring & Observability

**Error Tracking:** None (no Sentry, Datadog, etc.)

**Logging:**
- Framework: stdlib `log/slog` with `TextHandler` writing to stderr
- Configured in `cmd/monitor/main.go` `setupLogging()` based on `LOG_LEVEL`
- Structured key-value pairs (e.g., `slog.Error("scheduler: record failure", "id", endpointID, "error", err)`)

**Metrics:** None (no Prometheus, StatsD, etc.)

## CI/CD & Deployment

**Hosting:** Coolify (self-hosted PaaS)

**CI Pipeline:** None detected (no GitHub Actions, GitLab CI, or similar config files)

**Deployment artifacts:**
- `Dockerfile` - Multi-stage Docker image build
- `docker-compose.yml` - Service definition with volume mount
- `entrypoint.sh` - Permission fix and process exec

## Webhooks & Callbacks

**Incoming:** None. The Telegram bot uses long polling, not webhooks.

**Outgoing:** None. Notifications are push-based via Telegram Bot API `sendMessage`.

---

*Integration audit: 2026-03-26*

# Technology Stack

**Analysis Date:** 2026-03-26

## Languages

**Primary:**
- Go 1.26.1 - All application code

**Secondary:**
- SQL - Database migrations (`internal/storage/migrations/*.sql`)
- Shell (sh) - Docker entrypoint (`entrypoint.sh`)

## Runtime

**Environment:**
- Go 1.26.1 (compiled binary, no runtime needed beyond OS)
- Build constraint: `CGO_ENABLED=0` (pure Go, no C dependencies)

**Package Manager:**
- Go modules
- Module path: `noroshi`
- Lockfile: `go.sum` present

## Frameworks

**Core:**
- No web framework - single-purpose CLI binary at `cmd/monitor/main.go`
- `gopkg.in/telebot.v4` v4.0.0-beta.7 - Telegram Bot API (long polling, not webhooks)
- `github.com/go-co-op/gocron/v2` v2.19.1 - Cron-style job scheduler for periodic health checks

**Testing:**
- stdlib `testing` only - no external test frameworks, no testify, no gomock

**Build/Dev:**
- `go build` with `-ldflags="-s -w"` for stripped production binaries
- Docker multi-stage build (`Dockerfile`)

## Key Dependencies

**Critical (direct, explicitly mandated in CLAUDE.md):**

| Package | Version | Purpose | Usage Locations |
|---------|---------|---------|-----------------|
| `github.com/go-co-op/gocron/v2` | v2.19.1 | Job scheduling for periodic health checks | `internal/monitor/scheduler.go` |
| `github.com/hashicorp/go-retryablehttp` | v0.7.8 | HTTP health checks with automatic retry | `internal/monitor/checker.go` |
| `github.com/pressly/goose/v3` | v3.27.0 | Database schema migrations (embedded SQL) | `internal/storage/store.go` |
| `gopkg.in/telebot.v4` | v4.0.0-beta.7 | Telegram bot interface (long polling) | `internal/bot/*.go` |
| `modernc.org/sqlite` | v1.46.1 | Pure-Go SQLite driver (no CGO) | `internal/storage/store.go` |

**Stdlib packages of note:**
- `database/sql` - SQL interface wrapping the SQLite driver
- `log/slog` - Structured logging (text handler to stderr)
- `os` / `os/signal` - Configuration via `os.Getenv`, graceful shutdown via signals
- `net/http` - Health check HTTP server only (`/healthz` endpoint)
- `embed` - Embedding migration SQL files into the binary
- `context` - Propagated from root `signal.NotifyContext` throughout the app

**Indirect (transitive, not directly imported):**

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

**Environment:**
- All configuration via `os.Getenv` - no config libraries allowed
- Implementation: `internal/config/config.go` (`config.Load()`)
- `.env.example` documents all variables
- `.env` exists locally (gitignored, never read by the app directly - must be injected)

**Required env vars:**
- `TELEGRAM_TOKEN` - Telegram Bot API token
- `TELEGRAM_CHAT_ID` - Target chat ID (int64, e.g., `-100123456789`)

**Optional env vars (with defaults):**
- `DATABASE_PATH` - SQLite file path (default: `./data/uptime.db`)
- `HTTP_TIMEOUT` - Health check timeout as Go duration (default: `10s`)
- `MAX_FAILURE_NOTIFICATIONS` - Max consecutive failure alerts (default: `3`)
- `LOG_LEVEL` - Logging level: debug, info, warn, error (default: `info`)
- `HEALTH_PORT` - Health check HTTP server port (default: `8080`)

**Build:**
- `Dockerfile` - Multi-stage: `golang:1.26.1-alpine` builder, `alpine:3.21` runtime
- `docker-compose.yml` - Single service with named volume for SQLite persistence
- Build command: `CGO_ENABLED=0 go build -ldflags="-s -w" -o /monitor ./cmd/monitor/`

## Build & Deploy

**Build commands:**
```bash
CGO_ENABLED=0 go build ./cmd/monitor/       # Dev build (produces ./monitor binary)
CGO_ENABLED=0 go build -ldflags="-s -w" -o monitor ./cmd/monitor/  # Production build
go vet ./...                                  # Static analysis
go test ./...                                 # All tests
```

**Docker build:**
```bash
docker compose build                          # Build image
docker compose up -d                          # Run detached
```

**Docker details:**
- Multi-stage build: Go 1.26.1 Alpine builder -> Alpine 3.21 runtime
- Runtime installs: `ca-certificates`, `tzdata`, `curl`, `su-exec`
- Runs as non-root `appuser` via `entrypoint.sh` with `su-exec`
- Volume: `/app/data` for SQLite database persistence
- Health check: `curl -f http://localhost:8080/healthz` every 30s
- `entrypoint.sh` fixes volume permissions before starting the binary

**Deployment target:**
- Coolify (based on recent commit: "Add Coolify deployment setup with volume permission fix")
- Docker Compose compatible hosting

## Platform Requirements

**Development:**
- Go 1.26.1+
- No CGO required (pure Go SQLite driver)
- Telegram Bot API token for full integration testing

**Production:**
- Linux (Alpine-based Docker container)
- Writable filesystem for SQLite database at `DATABASE_PATH`
- Outbound HTTPS to `api.telegram.org` (Telegram Bot API)
- Outbound HTTP/HTTPS to monitored endpoint URLs
- Port 8080 (configurable) for health check endpoint

---

*Stack analysis: 2026-03-26*

# Noroshi — Uptime Monitor

A self-contained uptime monitor in Go that uses a Telegram bot as its interface. Add endpoints to monitor via chat commands, get notified on failures and recoveries. Runs as a single Docker container with SQLite for persistence.

## Setup

1. Create a Telegram bot via [@BotFather](https://t.me/BotFather) and get the token.
2. Get the chat ID of the group where the bot should send notifications.
3. Copy `.env.example` to `.env` and fill in the values.

### Docker (recommended)

```bash
docker compose up -d
```

### Local

```bash
export TELEGRAM_TOKEN=your-token
export TELEGRAM_CHAT_ID=-100123456789
CGO_ENABLED=0 go build ./cmd/monitor/
./monitor
```

## Bot Commands

| Command | Description |
|---------|-------------|
| `/add <url> <interval>` | Add endpoint (e.g., `/add https://example.com 30s`) |
| `/delete <id_or_url>` | Remove endpoint from monitoring |
| `/status` | Check all endpoints now and show results |
| `/list` | List all endpoints without triggering checks |
| `/interval <id_or_url> <interval>` | Update check interval |
| `/help` | Show available commands |

Intervals: `10s`, `30s`, `1m`, `5m`, `1h`, etc. (minimum 10s)

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TELEGRAM_TOKEN` | Yes | — | Bot token from @BotFather |
| `TELEGRAM_CHAT_ID` | Yes | — | Group chat ID for notifications |
| `DATABASE_PATH` | No | `./data/uptime.db` | SQLite database file path |
| `HTTP_TIMEOUT` | No | `10s` | Health check HTTP timeout |
| `MAX_FAILURE_NOTIFICATIONS` | No | `3` | Stop notifying after N consecutive failures |
| `LOG_LEVEL` | No | `info` | Log level: debug, info, warn, error |
| `HEALTH_PORT` | No | `8080` | Port for /healthz HTTP endpoint |

## Deploy on Coolify

1. **Create a new resource** in Coolify and connect your Git repository (GitHub App or deploy key).
2. **Select Dockerfile** as the build pack.
3. **Set the port to `8080`** in the General tab (Coolify defaults to 3000).
4. **Add persistent storage** — go to the Storage tab and add a volume with destination path `/app/data`. Coolify auto-appends a UUID to the volume name.
5. **Set environment variables** in the Environment Variables tab:
   - `TELEGRAM_TOKEN` (required)
   - `TELEGRAM_CHAT_ID` (required)
   - `DATABASE_PATH` = `/app/data/uptime.db` (optional, this is the default)
   - `LOG_LEVEL`, `MAX_FAILURE_NOTIFICATIONS`, `HTTP_TIMEOUT`, `HEALTH_PORT` (optional, see Configuration table above)
6. **Health checks** work automatically — the Dockerfile `HEALTHCHECK` takes precedence over Coolify's UI settings. Traefik routes traffic only to healthy instances.
7. **Domain** is optional — the bot uses Telegram long polling (no incoming web traffic). Only assign a domain if you want external access to the `/healthz` endpoint.
8. **Deploy** — Coolify will rebuild automatically on every push to the configured branch.

## Architecture

- **Scheduler**: gocron-based periodic health checks per endpoint
- **Checker**: retryablehttp with automatic retries on 5xx and connection errors
- **Storage**: SQLite via modernc.org/sqlite (pure Go, no CGO) with goose migrations
- **Bot**: telebot v4 with long polling
- **Health**: GET /healthz endpoint for Docker HEALTHCHECK and orchestrator probes

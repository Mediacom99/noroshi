# Noroshi — Uptime Monitor

[![CI](https://github.com/Mediacom99/noroshi/actions/workflows/ci.yml/badge.svg)](https://github.com/Mediacom99/noroshi/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Mediacom99/noroshi)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A self-contained uptime monitor in Go that uses a Telegram bot as its only interface. Add HTTP endpoints to monitor via chat commands, get alerted on failures and recoveries. Runs as a single Docker container with SQLite for persistence — no dashboard, no database server, nothing else to maintain.

## Features

- **Telegram bot UI** — manage endpoints entirely from a Telegram chat, with inline keyboards for details, interval changes, and confirmed deletes
- **Reliable checks** — automatic retries on 5xx and connection errors (via retryablehttp), so transient network blips don't page you; any 2xx counts as up
- **Sensible alerting** — alerts start after a configurable failure threshold and stop after N consecutive failures; recovery alerts include downtime duration
- **Pause/resume** — silence an endpoint during maintenance, optionally with auto-resume (`/pause prod-api 2h`)
- **Actionable, threaded alerts** — failure alerts carry check-now/pause/detail buttons; the recovery arrives as a reply to the original alert
- **Slow-endpoint detection** — optional 🟡 degraded state when latency exceeds a threshold
- **On-demand checks** — `/status` probes all endpoints immediately and reports HTTP code and latency
- **SQLite persistence** — endpoints survive restarts; pure-Go driver, no CGO
- **Single container** — multi-arch image (`linux/amd64`, `linux/arm64`) published to GHCR, `/healthz` endpoint for orchestrators

## Quickstart

1. Create a Telegram bot via [@BotFather](https://t.me/BotFather) and copy the token.
2. Get the chat ID of the group where the bot should post (add the bot to the group, then check `https://api.telegram.org/bot<TOKEN>/getUpdates`).
3. Run it:

### Docker (from GHCR)

```bash
docker run -d --name noroshi \
  -e TELEGRAM_TOKEN=your-token \
  -e TELEGRAM_CHAT_ID=-100123456789 \
  -v noroshi-data:/app/data \
  ghcr.io/mediacom99/noroshi:latest
```

### Docker Compose

```bash
cp .env.example .env   # fill in TELEGRAM_TOKEN and TELEGRAM_CHAT_ID
docker compose up -d
```

The compose file builds locally by default; comment out `build: .` and uncomment the `image:` line to use the published image instead.

### Local (no Docker)

```bash
export TELEGRAM_TOKEN=your-token
export TELEGRAM_CHAT_ID=-100123456789
CGO_ENABLED=0 go build ./cmd/monitor/
./monitor
```

## Bot Commands

| Command | Description |
|---------|-------------|
| `/add <name> <url> [interval]` | Add endpoint, e.g. `/add prod-api https://example.com 30s` (default interval: `1m`) |
| `/delete <name or id>` | Remove endpoint from monitoring |
| `/interval <name or id> <interval>` | Change check interval |
| `/pause <name or id> [duration]` | Stop checks and alerts, keep the endpoint configured (auto-resumes if a duration is given, e.g. `/pause prod-api 2h`) |
| `/resume <name or id>` | Resume a paused endpoint |
| `/list` | Dashboard of all endpoints with inline action buttons (check now, pause, interval, delete) |
| `/status` | Check all endpoints right now and show HTTP code + latency |
| `/help` | Show available commands |

- Names: 1–50 chars, letters/digits/`-`/`_`, not all-numeric (IDs take precedence in lookups).
- Intervals: `10s`, `30s`, `1m`, `5m`, `1h`, etc. (minimum `10s`).
- An endpoint is **up** when the check returns any HTTP 2xx status; anything else (including connection errors) is down.

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TELEGRAM_TOKEN` | Yes | — | Bot token from @BotFather |
| `TELEGRAM_CHAT_ID` | Yes | — | Chat ID the bot listens to and notifies |
| `DATABASE_PATH` | No | `./data/uptime.db` | SQLite database file path |
| `HTTP_TIMEOUT` | No | `10s` | Health check HTTP timeout |
| `MAX_FAILURE_NOTIFICATIONS` | No | `3` | Stop alerting after N consecutive failures |
| `FAILURE_THRESHOLD` | No | `1` | Consecutive failures before the first alert |
| `REMINDER_INTERVAL` | No | `0` (off) | Re-alert every interval while an outage continues (e.g. `2h`) |
| `SLOW_THRESHOLD_MS` | No | `0` (off) | Mark healthy endpoints as 🟡 slow above this latency |
| `LOG_LEVEL` | No | `info` | `debug`, `info`, `warn`, `error` |
| `HEALTH_PORT` | No | `8080` | Port for the `/healthz` HTTP endpoint |

The bot answers only in the configured chat — messages from any other chat are ignored.

## Deploy on Coolify

1. **Create a new resource** in Coolify and connect your Git repository (GitHub App or deploy key).
2. **Select Dockerfile** as the build pack (or use "Docker Image" with `ghcr.io/mediacom99/noroshi:latest`).
3. **Set the port to `8080`** in the General tab (Coolify defaults to 3000).
4. **Add persistent storage** — Storage tab, volume with destination path `/app/data`.
5. **Set environment variables**: `TELEGRAM_TOKEN` and `TELEGRAM_CHAT_ID` (required), plus any optional ones from the table above.
6. **Health checks** work automatically — the Dockerfile `HEALTHCHECK` takes precedence over Coolify's UI settings.
7. **Domain** is optional — the bot uses Telegram long polling (no inbound traffic). Only assign one if you want external access to `/healthz`.
8. **Deploy** — Coolify rebuilds on every push to the configured branch.

## Development

```bash
CGO_ENABLED=0 go build ./cmd/monitor/   # build
go vet ./...                            # vet
go test ./...                           # test
golangci-lint run                       # lint (v2.11.4, config in .golangci.yml)
```

Project layout:

```
cmd/monitor/        entrypoint: wiring, signal handling, /healthz server
internal/config/    env-var configuration
internal/apperror/  structured errors with sentinels
internal/storage/   SQLite store + goose migrations
internal/monitor/   HTTP checker (retryablehttp) + gocron scheduler
internal/bot/       Telegram bot: handlers, callbacks, formatting, validation
```

Releasing: push a `vX.Y.Z` tag — the release workflow builds and pushes a multi-arch image to `ghcr.io/mediacom99/noroshi` with `latest` and semver tags.

## License

[MIT](LICENSE)

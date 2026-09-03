# Noroshi — Uptime Monitor

[![CI](https://github.com/Mediacom99/noroshi/actions/workflows/ci.yml/badge.svg)](https://github.com/Mediacom99/noroshi/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Mediacom99/noroshi)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A self-contained uptime monitor in Go that uses a Telegram bot as its primary interface, with an optional React web dashboard. Add HTTP endpoints to monitor via chat commands or the dashboard, get alerted on failures and recoveries. Runs as a single Docker container with SQLite for persistence — no database server, nothing else to maintain.

## Features

- **Telegram bot UI** — manage endpoints entirely from a Telegram chat, with inline keyboards for details, interval changes, and confirmed deletes
- **Multiple check types** — HTTP(S) endpoints, TCP ports (`tcp://host:port`), DNS resolution (`dns://host`), and ICMP ping (`ping://host`)
- **Reliable checks** — automatic retries on 5xx and connection errors (via retryablehttp), so transient network blips don't page you; any 2xx counts as up
- **Sensible alerting** — alerts start after a configurable failure threshold and stop after N consecutive failures; recovery alerts include downtime duration
- **Pause/resume** — silence an endpoint during maintenance, optionally with auto-resume (`/pause prod-api 2h`)
- **Maintenance windows** — recurring scheduled silence (`/maint add prod-api sat,sun 02:00-04:00`, UTC); checks are skipped while a window is active
- **Actionable, threaded alerts** — failure alerts carry check-now/pause/detail buttons; the recovery arrives as a reply to the original alert
- **Slow-endpoint detection** — optional 🟡 degraded state when latency exceeds a threshold
- **Content checks** — require an exact HTTP status and/or a keyword in the response body
- **TLS certificate expiry warnings** for HTTPS endpoints
- **Uptime stats & incident history** — every check is recorded (30-day retention) and aggregated into `/uptime` and `/incidents`
- **Uptime digest** — optional daily/weekly summary message (`DIGEST=daily`), plus on-demand via `/digest`
- **Status badges** — embeddable SVG at `http://<host>:8080/badge/<name>.svg` for READMEs and dashboards
- **Web dashboard (optional)** — static React SPA in `web/` backed by a token-protected JSON API: live status, uptime stats, latency charts, incident history, and endpoint management (add/pause/resume/delete)
- **On-demand checks** — `/status` probes all endpoints immediately and reports HTTP code and latency
- **SQLite persistence** — endpoints survive restarts; pure-Go driver, no CGO
- **Single container** — multi-arch image (`linux/amd64`, `linux/arm64`) published to GHCR, `/healthz` endpoint for orchestrators
- **Prometheus metrics** — `/metrics` on the health port: check counts, latency histograms, and up/down gauges per endpoint

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
| `/add <name> <url> [interval]` | Add endpoint, e.g. `/add prod-api https://example.com 30s` (default interval: `1m`). Schemes: `http(s)://`, `tcp://host:port`, `dns://host`, `ping://host` |
| `/delete <name or id>` | Remove endpoint from monitoring |
| `/interval <name or id> <interval>` | Change check interval |
| `/expect <name or id> <status or any>` | Require an exact HTTP status (default: any 2xx) |
| `/keyword <name or id> <spec or off>` | Body check: `text` = must contain, `!text` = must NOT contain, `re:pattern` = must match regex, `!re:pattern` = must NOT match regex |
| `/rename <name or id> <new-name>` | Rename an endpoint |
| `/pause <name or id> [duration]` | Stop checks and alerts, keep the endpoint configured (auto-resumes if a duration is given, e.g. `/pause prod-api 2h`) |
| `/maint add <name\|all> <days> <HH:MM-HH:MM>` | Recurring maintenance window (UTC): scheduled checks are skipped while active, e.g. `/maint add prod-api sat,sun 02:00-04:00` |
| `/maint list` / `/maint del <id>` | Show / delete maintenance windows |
| `/resume <name or id>` | Resume a paused endpoint |
| `/list` | Dashboard of all endpoints with inline action buttons (check now, pause, interval, delete) |
| `/status` | Check all endpoints right now and show HTTP code + latency |
| `/uptime <name or id>` | Uptime %, avg/p95 latency, incident count (24h / 7d / 30d) |
| `/incidents <name or id>` | Recent outages with duration and HTTP code |
| `/digest` | Send the 24h uptime summary on demand |
| `/export` | Download the full config (endpoints + maintenance windows) as a JSON file |
| `/help` | Show available commands |

- Names: 1–50 chars, letters/digits/`-`/`_`, not all-numeric (IDs take precedence in lookups).
- Intervals: `10s`, `30s`, `1m`, `5m`, `1h`, etc. (minimum `10s`).
- Check types are chosen by URL scheme: `http(s)://` for HTTP endpoints, `tcp://host:port` for TCP ports (successful dial = up), `dns://host` for DNS resolution (resolves = up), and `ping://host` for ICMP echo. `/expect` and `/keyword` only apply to HTTP checks.
- Ping uses unprivileged ICMP, which requires the host's `net.ipv4.ping_group_range` to allow it (default on most Linux). In Docker you may need `--sysctl net.ipv4.ping_group_range="0 2147483647"`; without it, ping checks report down with reason "ping not permitted".
- An HTTP endpoint is **up** when the check returns any HTTP 2xx status (or the exact status set via `/expect`) **and** the response satisfies the `/keyword` body check, if any. Anything else (including connection errors) is down.
- HTTPS endpoints get an automatic **certificate expiry warning** when the cert has less than 14 days left (at most one warning per day).

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TELEGRAM_TOKEN` | Yes | — | Bot token from @BotFather |
| `TELEGRAM_CHAT_ID` | Yes | — | Chat ID the bot listens to and notifies |
| `TELEGRAM_WEBHOOK_URL` | No | — | Public https URL for webhook mode (default: long polling). See below |
| `TELEGRAM_WEBHOOK_PORT` | No | `8081` | Local port the webhook listener binds |
| `TELEGRAM_WEBHOOK_SECRET` | No | — | Verified against Telegram's secret-token header (recommended) |
| `DATABASE_PATH` | No | `./data/uptime.db` | SQLite database file path |
| `HTTP_TIMEOUT` | No | `10s` | Timeout for health checks (all check types) |
| `MAX_FAILURE_NOTIFICATIONS` | No | `3` | Stop alerting after N consecutive failures |
| `FAILURE_THRESHOLD` | No | `1` | Consecutive failures before the first alert |
| `REMINDER_INTERVAL` | No | `0` (off) | Re-alert every interval while an outage continues (e.g. `2h`) |
| `SLOW_THRESHOLD_MS` | No | `0` (off) | Mark healthy endpoints as 🟡 slow above this latency |
| `DIGEST` | No | `off` | Periodic uptime summary: `daily`, `weekly` (Mondays), or `off` |
| `DIGEST_TIME` | No | `09:00` | Time to send the digest (HH:MM, UTC) |
| `ALERT_WEBHOOK_URL` | No | — | POST every alert as JSON to this URL (generic second channel) |
| `ALERT_WEBHOOK_TOKEN` | No | — | Sent as `Authorization: Bearer <token>` on webhook calls |
| `LOG_LEVEL` | No | `info` | `debug`, `info`, `warn`, `error` |
| `HEALTH_PORT` | No | `8080` | Port for the `/healthz` HTTP endpoint |
| `DASHBOARD_TOKEN` | No | — | Bearer token for the dashboard JSON API (`/api/` on `HEALTH_PORT`); unset = API disabled |
| `DASHBOARD_ORIGIN` | No | — | Comma-separated allowed CORS origins for the dashboard frontend |

The bot answers only in the configured chat — messages from any other chat are ignored. The `/badge/<name>.svg` and `/metrics` endpoints are intentionally public (read-only), so only expose `HEALTH_PORT` if that's acceptable. The `/api/` dashboard API (when enabled) requires the `DASHBOARD_TOKEN` Bearer credential on every request.

## Web dashboard

An optional static React SPA lives in `web/` (Vite + TypeScript + Tailwind + TanStack Query/Router). It talks to the JSON API the binary serves under `/api/` on `HEALTH_PORT` when `DASHBOARD_TOKEN` is set — the same port as `/healthz`, `/badge` and `/metrics`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/endpoints` | List all endpoints with live status |
| `POST` | `/api/endpoints` | Add an endpoint `{name, url, interval_seconds?}` |
| `GET` | `/api/endpoints/{id}` | Detail + uptime stats (24h / 7d / 30d) |
| `PATCH` | `/api/endpoints/{id}` | Update name, interval, expected status, keyword |
| `DELETE` | `/api/endpoints/{id}` | Remove an endpoint |
| `POST` | `/api/endpoints/{id}/pause` | Pause (`{"duration": "2h"}` for auto-resume) |
| `POST` | `/api/endpoints/{id}/resume` | Resume a paused endpoint |
| `POST` | `/api/endpoints/{id}/check` | Run an ad-hoc check now |
| `GET` | `/api/endpoints/{id}/incidents` | Recent outages |
| `GET` | `/api/endpoints/{id}/checks?window=24h` | Raw check history (`24h`, `7d`, `30d`) |

All requests need `Authorization: Bearer <DASHBOARD_TOKEN>`. When the frontend is hosted on a different origin, list it in `DASHBOARD_ORIGIN` for CORS.

```bash
cd web
npm install
npm run dev        # dev server on :5173, /api proxied to localhost:8080
npm run build      # static build in web/dist/ — host it anywhere
```

The build reads `VITE_API_URL` (see `web/.env.example`) to know where the API lives; in dev the Vite proxy handles it, so no CORS setup is needed locally.

### Alert webhook

When `ALERT_WEBHOOK_URL` is set, every Telegram alert is also POSTed to that URL as JSON — useful as a fallback if Telegram itself is unreachable. Delivery is fire-and-forget: one POST, 10s timeout, no retries; a failing webhook never affects Telegram delivery.

```json
{
  "event": "failure",
  "timestamp": "2026-08-22T10:30:00Z",
  "endpoint": {"id": 1, "name": "prod-api", "url": "https://example.com",
               "status_code": 503, "latency_ms": 42, "reason": "HTTP 503"}
}
```

`event` is one of `failure`, `recovery` (adds `downtime_seconds`), `cert_expiry` (adds `days_left`), or `digest` (carries the digest in `text` instead of `endpoint`).

### Webhook mode (optional)

By default the bot uses long polling — no inbound traffic needed. If you'd rather have Telegram push updates to you, set:

```bash
TELEGRAM_WEBHOOK_URL=https://noroshi.example.com/telegram   # must be https
TELEGRAM_WEBHOOK_PORT=8081                                  # local listener
TELEGRAM_WEBHOOK_SECRET=some-random-string                  # strongly recommended
```

Telegram requires HTTPS with a trusted certificate, so run the bot behind a TLS-terminating proxy (Coolify's domain, nginx, Caddy): Telegram → proxy (TLS) → noroshi on `TELEGRAM_WEBHOOK_PORT`. Use an unguessable path (as above) plus `TELEGRAM_WEBHOOK_SECRET`; requests with a missing/wrong secret header are rejected, so internet noise can't inject fake updates. The webhook is registered on startup and deregistered on shutdown.

## Deploy on Coolify

1. **Create a new resource** in Coolify and connect your Git repository (GitHub App or deploy key).
2. **Select Dockerfile** as the build pack (or use "Docker Image" with `ghcr.io/mediacom99/noroshi:latest`).
3. **Set the port to `8080`** in the General tab (Coolify defaults to 3000).
4. **Add persistent storage** — Storage tab, volume with destination path `/app/data`.
5. **Set environment variables**: `TELEGRAM_TOKEN` and `TELEGRAM_CHAT_ID` (required), plus any optional ones from the table above.
6. **Health checks** work automatically — the Dockerfile `HEALTHCHECK` takes precedence over Coolify's UI settings.
7. **Domain** is optional — by default the bot uses Telegram long polling (no inbound traffic). Only assign one if you want external access to `/healthz`, `/badge`, `/metrics`, or if you enable webhook mode (`TELEGRAM_WEBHOOK_URL`).
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
cmd/monitor/        entrypoint: wiring, signal handling, /healthz server (+ /api/ dashboard API)
internal/config/    env-var configuration
internal/apperror/  structured errors with sentinels
internal/storage/   SQLite store + goose migrations
internal/monitor/   HTTP checker (retryablehttp) + gocron scheduler
internal/bot/       Telegram bot: handlers, callbacks, formatting, validation
internal/api/       Dashboard JSON API: auth, CORS, endpoint CRUD/stats/incidents
web/                Optional React dashboard (Vite + TypeScript + Tailwind + TanStack)
```

Releasing: push a `vX.Y.Z` tag — the release workflow builds and pushes a multi-arch image to `ghcr.io/mediacom99/noroshi` with `latest` and semver tags.

## License

[MIT](LICENSE)

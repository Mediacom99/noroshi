# Feature Landscape

**Domain:** Self-hosted uptime monitoring (Telegram-bot-only interface)
**Researched:** 2026-03-26
**Overall confidence:** HIGH (based on analysis of Uptime Kuma, Gatus, UptimeRobot, Checkmate, and several Telegram-specific uptime bots)

## Current State

Noroshi already has a functional core: add/delete/list endpoints, periodic HTTP checks via gocron+retryablehttp, failure/recovery notifications with consecutive failure tracking, notification cap, inline keyboard UI, named endpoints with lookup by name/ID/URL, SQLite persistence, Docker deployment. This is a working monitor. The question is what separates "working" from "complete" and "impressive."

---

## Table Stakes

Features users expect from any uptime monitor. Missing these makes the product feel like a toy project rather than a real tool.

| Feature | Why Expected | Complexity | Noroshi Status | Notes |
|---------|--------------|------------|----------------|-------|
| **Response time tracking** | Every competitor tracks latency per check. Users need to know if their site is slow, not just up/down. | Medium | MISSING | Requires storing `response_time_ms` per check. The `HTTPChecker.Check()` already has timing info available (measure before/after the HTTP call). Display in detail view and notifications. |
| **HTTP status code in notifications** | Uptime Kuma, Gatus, UptimeRobot all show the actual HTTP status. Noroshi shows "connection error" for everything. | Low | PARTIALLY BUILT | `FormatFailureWithCode` exists but is never called. `statusCode` flows through store methods but is not persisted. Wire it up and add a DB column. |
| **On-demand status check (`/status`)** | Users expect to trigger a check without waiting for the next interval. Every competitor has this. UptimeRobot has "Test" button, Uptime Kuma has "Test" per monitor. | Medium | MISSING | Designed in DESIGN.md but never implemented. Should iterate endpoints, call `checker.Check()`, and reply with live results. Also add a "Check Now" inline keyboard button on the detail view. |
| **Uptime percentage** | The single most-expected metric. "99.7% uptime in the last 24h." Every monitoring tool from UptimeRobot's free tier to Gatus shows this. | Medium | MISSING | Requires a `check_log` table storing each check result (status, response_time, timestamp). Calculate: `(successful_checks / total_checks) * 100` over 24h/7d/30d windows. Show in `/list` detail view. |
| **Pause/resume monitoring** | Users deploy changes, run maintenance. They need to suppress alerts without deleting the endpoint. Uptime Kuma, UptimeRobot, Better Stack all have this. | Low | MISSING | Add a `paused` boolean to endpoints table. Scheduler skips paused endpoints. Add `/pause` and `/resume` commands, plus inline keyboard toggle. |
| **Expected status codes** | Not all healthy endpoints return 200. APIs may return 204, 301 redirects may be correct. Uptime Kuma and Gatus both support configuring expected status codes. | Low | MISSING | Currently hardcoded `statusCode != 200` means failure. Add `expected_status` column (default 200). Compare against it in `checkAndNotify`. |
| **Name validation** | Names with special characters, extreme lengths, or HTML injection could break formatting. | Low | MISSING | Already identified in CONCERNS.md. Alphanumeric + hyphens + underscores, max 50 chars. |

## Differentiators

Features that make Noroshi stand out as a portfolio piece. Not expected, but impressive when present -- especially for someone reviewing a GitHub repo.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Response time history with sparkline in Telegram** | Show a mini ASCII/Unicode sparkline of response times in the detail view (e.g., `[latency] _-^-__-^`). Unique for a Telegram-only tool. Shows engineering creativity. | Medium | Telegram supports monospace (`<code>`) blocks. A simple sparkline using Unicode block characters shows trend at a glance. Requires check_log table. |
| **Daily/weekly uptime digest** | Proactive summary message: "Weekly Report: 3/3 endpoints healthy, avg response 234ms, uptime 99.8%." No competitor Telegram bot does scheduled reporting well. | Medium | Use gocron to schedule a daily/weekly job. Query check_log for aggregates. Demonstrates the scheduler is used for more than just health checks. |
| **SSL certificate expiry monitoring** | Alert N days before cert expires. Uptime Kuma v2.1 just added domain expiry monitoring. Go's `crypto/tls` makes this straightforward -- no new dependencies needed. | Medium | `tls.Dial` + check `PeerCertificates[0].NotAfter`. Schedule a daily check per HTTPS endpoint. Alert at 14/7/1 day thresholds. Uses only stdlib. |
| **Response time threshold alerts** | "Your API responded in 3200ms (threshold: 1000ms)." Gatus has this as a core feature. Goes beyond up/down binary. | Low | Compare measured response time against a configurable `max_response_ms` per endpoint. Alert when exceeded. Pairs naturally with response time tracking. |
| **Keyword/body content check** | Verify response body contains expected string. Catches the "200 OK but error page" scenario. Uptime Kuma calls this "keyword monitoring." | Medium | Read response body (with size limit), check for substring. Add `expected_keyword` column. Requires reading response body in checker, which currently discards it. |
| **Incident timeline** | `/incidents <name>` shows last N state transitions with timestamps: "Down 14:30 -> Up 14:45 (15m), Down 09:12 -> Up 09:14 (2m)." | Medium | Requires a `state_changes` table recording up->down and down->up transitions. Demonstrates proper event sourcing thinking. |
| **Export/backup** | `/export` dumps all endpoint configs as JSON. Useful for backup, migration, or sharing configs. | Low | Query all endpoints, marshal to JSON, send as document via Telegram's `sendDocument`. Shows polish. |

## Anti-Features

Features to explicitly NOT build. These would bloat the project, violate the "Telegram-only" philosophy, or add complexity without proportional value.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| **Web dashboard / status page** | Noroshi's entire value proposition is "no dashboard to maintain." Adding one contradicts the core identity. Uptime Kuma, Gatus, Checkmate already do this better. | Lean into the Telegram-only angle. Make the Telegram UI so good that a dashboard isn't missed. The project stands out by NOT having one. |
| **Multi-user support / RBAC** | Adds authentication complexity, permission models, user management. Overkill for a personal/small-team tool. The chat ID guard is appropriate. | If group use is needed, configure a Telegram group chat ID. All group members can use the bot. |
| **Non-HTTP monitor types (TCP, DNS, ICMP, ping)** | Each protocol needs its own checker, error handling, and display logic. Dilutes the "HTTP uptime monitor" focus. Gatus supports 10+ protocols but that is a different product. | Stay focused on HTTP/HTTPS. Do HTTP well (status codes, response time, keyword, SSL). Depth over breadth. |
| **PostgreSQL / MySQL support** | SQLite handles tens-to-hundreds of endpoints. Adding database drivers violates the "no new deps" constraint and complicates deployment (now you need a database server). | SQLite with WAL mode. If someone needs Postgres scale, they need a different tool entirely. |
| **Webhook mode for Telegram** | Long polling works perfectly for a single-instance bot. Webhooks require a public HTTPS endpoint, TLS termination, and complicate the deployment story. | Keep long polling. It is simpler and more reliable for self-hosted single-instance use. |
| **Custom notification channels (Slack, Discord, email)** | Each channel needs its own integration, formatting, authentication. Noroshi is a Telegram bot -- that IS the notification channel. Adding Slack defeats the purpose. | Telegram only. Users who need multi-channel notifications should use UptimeRobot or Uptime Kuma. |
| **Config files (YAML/TOML/JSON)** | Project constraint says `os.Getenv` only. Config files add parsing, validation, file watching. Gatus is config-as-code; Noroshi is chat-as-code. | Environment variables for system config, Telegram commands for endpoint config. Different paradigm, equally valid. |
| **Response body storage** | Storing full response bodies balloons the database. Even for keyword checks, store the match result, not the body. | For keyword check: store boolean "keyword_found" in check_log, not the body itself. |

## Feature Dependencies

```
Response time tracking -------> Uptime percentage (needs check_log table)
                       \------> Response time sparkline (needs check_log table)
                        \-----> Response time threshold alerts (needs timing data)
                         \----> Daily/weekly digest (needs aggregate data)

HTTP status code in notifications --> Expected status codes (both need status code pipeline)

check_log table ---> Uptime percentage
               \---> Sparkline
                \--> Incident timeline
                 \-> Digest reports

Pause/resume monitoring (independent, no dependencies)
SSL cert expiry monitoring (independent, no dependencies)
Keyword/body check (independent, modifies checker)
Export/backup (independent, queries existing data)
Name validation (independent, modifies validation)
```

The critical dependency is the **check_log table**. Multiple differentiator features depend on having a historical record of individual checks. Building response time tracking first (which requires this table) unlocks uptime percentage, sparklines, digests, and incident timelines.

## MVP Recommendation

### Phase 1: Fix the Plumbing (Low-hanging fruit)

These are partially built or trivially achievable. They fix existing broken/incomplete pipelines.

1. **HTTP status code in notifications** -- `FormatFailureWithCode` already exists, `statusCode` already flows through the system. Wire it up. Add `last_status_code` column.
2. **Name validation** -- Already identified in CONCERNS.md. Simple `ValidateName()` function.
3. **Expected status codes** -- Add column, change one comparison in `checkAndNotify`. Tiny change, big usability win.

### Phase 2: The Foundation Table (Unlocks everything)

4. **Response time tracking + check_log table** -- Add migration for `check_log(id, endpoint_id, status_code, response_time_ms, success, checked_at)`. Measure and store response time in every check. Display in detail view. This single feature unlocks Phases 3 and 4.
5. **Uptime percentage** -- Query check_log for 24h/7d percentages. Display in detail view and `/list` summary. This is the feature that makes a monitor feel "real."

### Phase 3: Operational Essentials

6. **On-demand status check (`/status`)** -- Trigger live checks. Add "Check Now" button to detail view.
7. **Pause/resume monitoring** -- `/pause`, `/resume`, inline keyboard toggle. Essential for deployments and maintenance.

### Phase 4: Portfolio Impressors (Differentiators)

Pick 2-3 based on taste and time:

8. **SSL certificate expiry monitoring** -- Pure stdlib, no new deps, impressive feature.
9. **Daily/weekly digest** -- Shows the scheduler doing more than just checks.
10. **Response time sparkline** -- Unique to Telegram-only monitors, demonstrates creativity.

### Defer Indefinitely

- **Keyword/body check** -- Useful but adds complexity to the checker. Nice-to-have, not essential for portfolio.
- **Incident timeline** -- Cool but redundant if uptime percentage and failure/recovery notifications are solid.
- **Export/backup** -- Polish feature. Add if everything else is done.
- **Response time threshold alerts** -- Impressive but overlaps with response time tracking. Consider adding as a simple extension of threshold tracking later.

## Portfolio Impact Assessment

What a GitHub reviewer notices when evaluating this project:

| Signal | Feature That Demonstrates It |
|--------|------------------------------|
| **"This person ships complete products"** | Uptime percentage, response time tracking, pause/resume -- table stakes that show the monitor is not a weekend hack |
| **"Good engineering decisions"** | Interface-based DI, migration-driven schema, clean error handling (already present). Expected status codes and check_log table show thoughtful data modeling. |
| **"Creative problem solving"** | Response time sparkline in Telegram, daily digest reports -- things that make a reviewer think "oh, that's clever" |
| **"Production-ready thinking"** | SSL cert monitoring, pause for maintenance, proper status codes in alerts -- shows awareness of real operational needs |
| **"Clean, tested code"** | Bot handler tests, consistent structured logging, CI pipeline (from existing milestone items) |

The highest-impact features for portfolio impression per line of code: uptime percentage > response time tracking > SSL cert monitoring > pause/resume > sparkline.

## Sources

- [Uptime Kuma GitHub](https://github.com/louislam/uptime-kuma) -- Feature reference for table stakes (HIGH confidence)
- [Gatus GitHub](https://github.com/TwiN/gatus) -- Condition-based monitoring patterns (HIGH confidence)
- [UptimeRobot Pricing](https://uptimerobot.com/pricing/) -- Free tier feature baseline (HIGH confidence)
- [Gatus Alerting Docs](https://gatus.io/docs/alerting-getting-started) -- Response time condition syntax (HIGH confidence)
- [Better Stack Monitoring Comparison](https://betterstack.com/community/comparisons/open-source-website-monitoring/) -- Landscape overview (MEDIUM confidence)
- [Telegram Bot Features](https://core.telegram.org/bots/features) -- Inline keyboard capabilities (HIGH confidence)
- [telegram-uptime-monitor](https://github.com/BekiChemeda/telegram-uptime-monitor) -- Comparable Telegram-only monitor with maintenance windows, keyword checks, SSL monitoring (MEDIUM confidence)
- [Uptime Calculation Guide](https://uptimerobot.com/blog/how-to-calculate-uptime/) -- Uptime percentage formula and time windows (HIGH confidence)
- [UptimeRobot Maintenance Windows](https://uptimerobot.com/blog/new-feature-maintenance-windows-for-the-pro-plan/) -- Maintenance window patterns (HIGH confidence)
- [Checkmate GitHub](https://github.com/bluewave-labs/Checkmate) -- Response time visualization patterns (MEDIUM confidence)

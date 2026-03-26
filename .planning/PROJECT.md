# Noroshi — Uptime Monitor

## What This Is

A self-contained uptime monitor built in Go that uses a Telegram bot as its sole interface. Users add HTTP endpoints to monitor via chat commands, and the bot sends alerts on failures and recoveries. Runs as a single Docker container with SQLite for persistence. Designed to be simple, useful, and easy to self-host.

## Core Value

Reliable uptime monitoring with zero-friction setup — one Docker container, one Telegram bot, no dashboards to maintain.

## Requirements

### Validated

- ✓ Add/delete endpoints via Telegram commands (`/add`, `/delete`) — existing
- ✓ Periodic HTTP health checks via gocron scheduler with retryablehttp — existing
- ✓ Failure notifications with consecutive failure tracking and notification cap — existing
- ✓ Recovery notifications with downtime duration — existing
- ✓ List endpoints via `/list` with inline keyboard detail views — existing
- ✓ Update check intervals via `/interval` command — existing
- ✓ Named endpoints with lookup by name, ID, or URL — existing
- ✓ SQLite persistence with goose migrations — existing
- ✓ Docker deployment with health checks — existing
- ✓ Coolify deployment support — existing
- ✓ Chat ID-based authorization guard — existing
- ✓ URL and interval validation — existing
- ✓ Graceful shutdown with context propagation — existing

### Active

- [ ] Fix Scheduler to depend on Checker interface, not concrete HTTPChecker
- [ ] Pass HTTP status code through notification pipeline (use FormatFailureWithCode)
- [ ] Persist or remove unused statusCode parameter in store methods
- [ ] Implement `/status` command that triggers live health checks
- [ ] Trigger immediate health check and show result when user adds an endpoint
- [ ] Add tests for bot handlers and callbacks (mock tele.Context + Store + Scheduler)
- [ ] Improve structured logging with consistent fields across all packages
- [ ] Add name validation (alphanumeric, hyphens, underscores, max length)
- [ ] Update DESIGN.md to match current implementation
- [ ] Professional README with badges, architecture section, clear setup/deploy docs
- [ ] GitHub Actions CI pipeline (build, vet, test, lint)
- [ ] Clean up stale TODO.md items and sync with actual state

### Out of Scope

- Web dashboard — Telegram is the interface, keep it simple
- Multi-user support / user management — single chat ID is sufficient for v1
- PostgreSQL migration — SQLite handles the expected scale (tens of endpoints)
- Response time tracking / uptime statistics — good v2 feature, not this milestone
- Webhook mode for Telegram — long polling works well for a single-instance bot
- OAuth / advanced authentication — chat ID guard is appropriate for personal/small-team use
- Rate limiting on commands — low risk for single-user/small-group use

## Context

- **Owner:** Freelance software developer building a portfolio of polished, open-source Go projects
- **Goal:** This is a showcase project — the code quality, documentation, and presentation should reflect professional standards that impress clients and the dev community
- **Current state:** Core functionality works and is deployed on a VPS via Coolify. The codebase has technical debt (interface violations, dead code, stale docs) and missing features (live status checks, bot handler tests, CI pipeline)
- **Codebase map:** `.planning/codebase/` contains 7 analysis documents from 2026-03-26
- **Tech debt highlights:** Scheduler depends on concrete `*HTTPChecker` instead of interface; `FormatFailureWithCode` defined but never called; `statusCode` parameter flows through store methods unused; DESIGN.md stale in multiple places; bot handlers/callbacks untested

## Constraints

- **Dependencies:** Only the libraries listed in CLAUDE.md — no new deps without explicit approval
- **CGO:** Must build with `CGO_ENABLED=0` (pure Go SQLite driver)
- **Testing:** Every non-main package must have `_test.go` files (exception: `internal/bot/` historically, but this milestone adds bot tests)
- **Context:** No `context.Background()` outside `main.go`
- **Interfaces:** Defined at point of use, not in implementing package

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Telegram-only interface | Keeps project simple, no frontend to maintain, unique angle for portfolio | ✓ Good |
| SQLite over PostgreSQL | Single container simplicity, no external DB dependency, sufficient for scale | ✓ Good |
| Fix tech debt before adding features | Solid foundation prevents compounding issues | — Pending |
| Add GitHub Actions CI | Shows professionalism, enables badge in README, catches regressions | — Pending |
| Professional README with badges/diagrams | First impression for portfolio visitors — must be polished | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-03-26 after initialization*

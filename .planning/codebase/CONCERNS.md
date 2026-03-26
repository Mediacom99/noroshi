# Codebase Concerns

**Analysis Date:** 2026-03-26

## Tech Debt

**Scheduler depends on concrete HTTPChecker, not an interface:**
- Issue: `Scheduler` in `internal/monitor/scheduler.go` holds `checker *HTTPChecker` (line 33), a concrete type. CLAUDE.md mandates "define interfaces at the point of use." The scheduler should define a `Checker` interface (e.g., `Check(ctx context.Context, url string) (int, error)`) and depend on that, not the concrete `*HTTPChecker`.
- Files: `internal/monitor/scheduler.go`
- Impact: Cannot inject a mock checker in scheduler tests. The scheduler tests (`internal/monitor/scheduler_test.go`) work around this by spinning up real `httptest.Server` instances, which makes tests slower and couples scheduler logic to real HTTP. It also violates the stated convention.
- Fix approach: Define a `Checker` interface in `internal/monitor/scheduler.go` with `Check(ctx context.Context, url string) (int, error)`. Change the `Scheduler` struct field to use the interface. Tests can then use a mock checker.

**FormatFailureWithCode is defined but never called:**
- Issue: `FormatFailureWithCode()` in `internal/bot/format.go` (line 88) is tested in `internal/bot/format_test.go` but never invoked by any production code. The `TelegramNotifier.NotifyFailure()` calls `FormatFailure()` which always shows "connection error" regardless of whether a non-200 status code was received. This means users see "connection error" even when the endpoint returned a 503.
- Files: `internal/bot/format.go`, `internal/bot/bot.go` (line 132)
- Impact: Misleading failure notifications. A 503 response is reported as "connection error" rather than showing the actual HTTP status code.
- Fix approach: The `Notifier` interface should accept the status code, or `FormatFailure` should receive the status code from the endpoint. The `NotifyFailure` method signature in `internal/monitor/scheduler.go` (line 24) would need to pass the status code, and `TelegramNotifier.NotifyFailure` should decide between `FormatFailure` (code 0 = connection error) and `FormatFailureWithCode` (non-zero code).

**statusCode parameter accepted but unused in store methods:**
- Issue: `UpdateEndpointStatus`, `RecordFailure`, and `RecordRecovery` in `internal/storage/store.go` all accept a `statusCode int` parameter but none of them persist it to the database. There is no `status_code` or `last_status_code` column in the schema.
- Files: `internal/storage/store.go` (lines 175, 212, 233), `internal/storage/migrations/001_create_endpoints.sql`, `internal/storage/migrations/002_add_endpoint_name.sql`
- Impact: The status code parameter flows through the system doing nothing. Either remove it from signatures (cleaner) or add a DB column and store it (more useful, enables showing last HTTP status in `/list` detail view).
- Fix approach: Add a `last_status_code` column via migration 003, persist it in `RecordFailure` and `RecordRecovery`, and display it in the endpoint detail view. Alternatively, remove the unused parameter if the feature is not wanted.

**Removed /status command still referenced in documentation:**
- Issue: The `/status` command is documented in `DESIGN.md` (line 265-269) and `README.md` (line 32), but it is not registered in `internal/bot/handlers.go`. The `registerHandlers()` function (line 17) registers `/add`, `/delete`, `/list`, `/interval`, `/help` -- no `/status`. The `/list` command replaced it, but docs are stale.
- Files: `DESIGN.md` (line 265), `README.md` (line 32), `internal/bot/handlers.go`
- Impact: Users who read the README may try `/status` and get no response. Documentation diverges from implementation.
- Fix approach: Either re-implement `/status` (which per DESIGN.md should trigger immediate health checks, unlike `/list`) or remove all references from `DESIGN.md` and `README.md`.

**DESIGN.md is stale in multiple places:**
- Issue: DESIGN.md has not been updated to reflect recent changes. Specific divergences:
  - Project structure (line 7-39) does not list `internal/bot/callbacks.go`, `internal/bot/validate.go`, `internal/bot/validate_test.go`, or `internal/storage/migrations/002_add_endpoint_name.sql`.
  - `/add` command syntax (line 252) shows `<url> <interval>` but actual syntax is `<name> <url> [interval]`.
  - Store interface (line 233-243) signature for `AddEndpoint` lacks the `name` parameter, and `GetEndpointByName` is missing.
  - Message format section (lines 314-356) uses emoji + plain text, but actual implementation uses HTML formatting with `<b>` tags.
- Files: `DESIGN.md`
- Impact: DESIGN.md is referenced as the primary architectural reference. Stale documentation causes confusion and wrong assumptions.
- Fix approach: Update DESIGN.md to match current implementation. Add missing files to project structure, update Store interface, update command syntax, and update message format examples.

## Missing Functionality

**No immediate health check on add:**
- Issue: When a user adds an endpoint via `/add`, the system schedules it with `gocron.WithStartImmediately()` (in `internal/monitor/scheduler.go` line 66), which does run the first check soon. However, TODO.md (line 13) marks this as incomplete. The gocron immediate start may have slight delay depending on scheduler state; a truly immediate inline check with result shown to the user is not implemented.
- Files: `internal/bot/handlers.go` (handleAdd), `internal/monitor/scheduler.go` (Add method)
- Impact: User adds an endpoint and does not see its status until the scheduled job fires. Low impact since `WithStartImmediately()` is used.
- Fix approach: In `handleAdd`, after `scheduler.Add`, perform a synchronous check and reply with the initial status. Requires the bot to have access to the checker or scheduler to expose a `CheckNow` method.

**Missing /status command (live checks):**
- Issue: DESIGN.md specifies a `/status` command that triggers immediate health checks on all endpoints and returns results. This is distinct from `/list` which only shows stored data. The command is not implemented.
- Files: `internal/bot/handlers.go`
- Impact: No way for users to trigger on-demand health checks via chat. The inline keyboard "Refresh" button (`cbRefresh` in `internal/bot/callbacks.go`) only re-fetches stored data, not live checks.
- Fix approach: Implement `/status` handler that iterates endpoints, calls `checker.Check()` on each, updates store, and replies with results.

**Structured logging improvements (TODO.md):**
- Issue: TODO.md (line 18) flags inconsistent structured logging. Current `slog.Error` calls lack consistent fields. For example, `internal/monitor/scheduler.go` logs endpoint ID but not URL. Bot handlers log errors but not which command triggered them, or which user/chat.
- Files: All files using `slog.Error`/`slog.Info` (see grep results above)
- Impact: Harder to debug issues in production. No request correlation.
- Fix approach: Define standard slog attributes (endpoint_id, endpoint_url, endpoint_name, command, chat_id) and use them consistently. Consider adding a logger with base attributes to the Scheduler and Bot structs.

**Message formatting improvements (TODO.md):**
- Issue: TODO.md (line 4) flags message formatting as needing improvement. The inline keyboard UI has been implemented, which was a major UX upgrade, but the TODO item remains open.
- Files: `internal/bot/format.go`
- Impact: Low priority -- the current HTML formatting and inline keyboards are functional and clean.
- Fix approach: Review specific formatting pain points. This TODO may be stale and could be closed.

## Security Concerns

**No authorization beyond chat ID check:**
- Risk: The bot only checks that the incoming message's chat ID matches `TELEGRAM_CHAT_ID` (via the `guarded()` wrapper in `internal/bot/bot.go` line 96). Any member of that chat can control the bot -- add endpoints, delete endpoints, change intervals.
- Files: `internal/bot/bot.go` (guarded function, line 96)
- Current mitigation: Only users in the configured chat can interact. This is acceptable for a private group.
- Recommendations: For shared groups, consider adding an allowed-users list (Telegram user IDs) as an optional config. Not urgent for private use.

**No input length limits on name/URL fields:**
- Risk: Users can submit arbitrarily long endpoint names or URLs. Very long values could cause display issues in Telegram messages or bloat the SQLite database.
- Files: `internal/bot/handlers.go` (handleAdd, line 27), `internal/bot/validate.go`
- Current mitigation: URL is validated for scheme and host. Name has no validation at all.
- Recommendations: Add max-length validation for name (e.g., 50 chars) and URL (e.g., 2000 chars). Add character restrictions for name (alphanumeric, hyphens, underscores).

**No rate limiting on bot commands:**
- Risk: A user (or compromised Telegram account) could spam commands, causing excessive database writes or health checks.
- Files: `internal/bot/handlers.go`
- Current mitigation: None. Telegram itself has some rate limiting on bot API calls.
- Recommendations: Low priority for single-user/small-group use. Could add per-user command cooldown if needed.

**Entrypoint runs as root then drops privileges:**
- Risk: `entrypoint.sh` runs as root to fix volume permissions, then uses `su-exec` to drop to `appuser`. The `chown` command runs every start, which is fine, but running as root at all is a minor concern.
- Files: `entrypoint.sh`
- Current mitigation: `su-exec` drops privileges immediately after chown.
- Recommendations: Acceptable pattern. An alternative is using `initContainers` or Docker's `--user` flag with pre-configured volume permissions.

## Scalability & Performance

**SQLite single-writer limitation:**
- Problem: SQLite with WAL mode allows concurrent reads but only one writer at a time. The `_busy_timeout=5000` in `internal/storage/store.go` (line 22) provides a 5-second wait before failing on write contention.
- Files: `internal/storage/store.go` (OpenDB function)
- Cause: Inherent SQLite limitation. Each health check writes to the DB (updating status), and concurrent gocron jobs may contend.
- Improvement path: Not a problem at the expected scale (tens of endpoints). If scaling to hundreds, consider batching status updates or using a connection pool with write serialization. Migration to PostgreSQL would be needed for true concurrent writes, but is overkill for this use case.

**Checker creates one retryablehttp client shared across all checks:**
- Problem: All health check jobs share a single `HTTPChecker` with one `*retryablehttp.Client`. This is actually fine -- `retryablehttp.Client` is safe for concurrent use. Not a real bottleneck.
- Files: `internal/monitor/checker.go`
- Cause: N/A -- this is the correct pattern.
- Improvement path: No action needed.

**No endpoint count limit:**
- Problem: Users can add unlimited endpoints via `/add`. Each endpoint creates a gocron job that performs HTTP requests at the configured interval.
- Files: `internal/bot/handlers.go` (handleAdd)
- Cause: No limit enforced.
- Improvement path: Add a configurable `MAX_ENDPOINTS` environment variable and check it in `handleAdd` before adding. Low priority for personal use.

## Testing Gaps

**Bot handlers and callbacks untested:**
- What's not tested: All Telegram handler logic in `internal/bot/handlers.go` and `internal/bot/callbacks.go` -- argument parsing, error handling flows, scheduler interactions, delete-confirmation flow.
- Files: `internal/bot/handlers.go`, `internal/bot/callbacks.go`
- Risk: Regressions in command parsing, error message formatting, or scheduler coordination would not be caught. These files contain significant business logic (findEndpoint lookup chain, interval validation, delete confirmation).
- Priority: Medium. The CLAUDE.md exempts `internal/bot/` from test requirements because it "requires Telegram API," but the handler logic could be tested by mocking the `tele.Context` interface and the `Store`/`Scheduler` interfaces.

**Scheduler tests use context.Background():**
- What's not tested: Tests in `internal/monitor/scheduler_test.go` and `internal/monitor/checker_test.go` use `context.Background()` throughout. This is acceptable in tests (CLAUDE.md's prohibition is for production code), but the tests do not verify context cancellation behavior in the scheduler's `checkAndNotify` method.
- Files: `internal/monitor/scheduler_test.go`, `internal/monitor/checker_test.go`
- Risk: Low. Context cancellation in checker is tested (`TestCheckerCancelledContext`). The scheduler's response to context cancellation is not directly tested.
- Priority: Low.

**No integration tests:**
- What's not tested: End-to-end flow from adding an endpoint through scheduler running a check through notification. Current tests are unit-level with mocks.
- Files: N/A
- Risk: Integration issues between store, scheduler, and notifier layers would not be caught.
- Priority: Low for a project of this size.

## Dependency Risks

**telebot v4 is a beta release:**
- Risk: `gopkg.in/telebot.v4 v4.0.0-beta.7` -- the dependency is a pre-release beta version. API may change between beta versions.
- Impact: Future updates to telebot v4 could require code changes to callback handling, context APIs, or send options.
- Migration plan: Pin the exact version (already pinned in go.mod). Monitor for stable v4 release and update when available. The telebot v3 is the stable alternative if v4 is abandoned.

**All other dependencies are stable:**
- `gocron/v2` v2.19.1 -- actively maintained, stable API.
- `go-retryablehttp` v0.7.8 -- HashiCorp maintained, stable.
- `goose/v3` v3.27.0 -- widely used, stable.
- `modernc.org/sqlite` v1.46.1 -- actively maintained, pure Go.

## Improvement Opportunities

**Extract Checker interface for testability:**
- The `Scheduler` struct should depend on a `Checker` interface, not the concrete `*HTTPChecker`. This would make scheduler tests faster and more focused (no real HTTP requests needed) and aligns with CLAUDE.md conventions.
- Files: `internal/monitor/scheduler.go`
- Approach: Define `type Checker interface { Check(ctx context.Context, url string) (int, error) }` in the scheduler file. Update the struct field.

**Pass HTTP status code through notification pipeline:**
- Currently `NotifyFailure` does not receive the HTTP status code, so `FormatFailure` always displays "connection error." The status code is available in `checkAndNotify` but dropped before notification.
- Files: `internal/monitor/scheduler.go` (checkAndNotify), `internal/bot/bot.go` (NotifyFailure), `internal/bot/format.go`
- Approach: Add `statusCode int` to the `NotifyFailure` interface method. Use `FormatFailureWithCode` when statusCode > 0, `FormatFailure` when statusCode == 0.

**Clean up unused statusCode parameters in store:**
- `UpdateEndpointStatus`, `RecordFailure`, and `RecordRecovery` all accept `statusCode` but ignore it. Either persist it or remove it from signatures.
- Files: `internal/storage/store.go`

**Add name validation in handleAdd:**
- Endpoint names have no validation. Could contain spaces (though `strings.Fields` parsing prevents this), special characters, HTML, or be excessively long.
- Files: `internal/bot/handlers.go`, `internal/bot/validate.go`
- Approach: Add `ValidateName(name string) error` in `validate.go` -- alphanumeric, hyphens, underscores, max 50 chars.

**Consider making recovery notifications configurable:**
- Recovery notifications are always sent (silently via `SendSilentMessage`). Some users might want loud recovery notifications, or might want to disable them.
- Files: `internal/bot/bot.go` (NotifyRecovery), `internal/config/config.go`
- Approach: Add a `RECOVERY_NOTIFICATION_SILENT` bool config (default true for backward compatibility).

---

*Concerns audit: 2026-03-26*

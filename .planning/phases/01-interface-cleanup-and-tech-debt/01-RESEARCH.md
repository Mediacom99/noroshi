# Phase 1: Interface Cleanup and Tech Debt - Research

**Researched:** 2026-03-26
**Domain:** Go interface design, SQLite migrations, input validation, dead code elimination
**Confidence:** HIGH

## Summary

Phase 1 addresses five concrete code quality issues in a small Go codebase (~1100 lines of production code across 6 packages). Every change is internal refactoring or bug fixing -- no new features, no new dependencies, no new packages. The work decomposes into four independent workstreams: (1) extract a Checker interface in the monitor package, (2) persist and display HTTP status codes through the notification pipeline, (3) eliminate dead exported symbols, and (4) add name validation to the bot. All four workstreams touch different files with minimal overlap, but the status code pipeline (workstream 2) has the widest blast radius: it touches migrations, models, store methods, store tests, notification logic, formatting functions, and format tests.

The codebase is well-structured with clear conventions. Interfaces are defined at point of use (consumer side). Errors use `apperror.Wrap(sentinel, cause)`. Tests use stdlib `testing` with table-driven patterns. Migrations use goose with embedded SQL files. All of these patterns are already established and should be followed exactly.

**Primary recommendation:** Implement in dependency order -- Checker interface first (unblocks TEST-04 in Phase 2), then status code pipeline (migration + store + notification + formatting), then dead code sweep, then name validation. Each workstream should be a separate commit.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Define a `Checker` interface in `internal/monitor/scheduler.go` with method `Check(ctx context.Context, url string) (int, error)` -- same signature as the existing `HTTPChecker.Check`
- **D-02:** Change `Scheduler.checker` field from `*HTTPChecker` to `Checker`, and update `NewScheduler` parameter accordingly
- **D-03:** `HTTPChecker` already satisfies the interface -- no changes to `checker.go` needed
- **D-04:** Persist `statusCode` to a new `last_status_code` INTEGER column via goose migration (not remove the parameter) -- this enables status display in notifications and detail view
- **D-05:** Add `LastStatusCode int` field to `storage.Endpoint` model and update all scan calls
- **D-06:** Store methods (`UpdateEndpointStatus`, `RecordFailure`, `RecordRecovery`) persist the `statusCode` parameter to the new column
- **D-07:** `NotifyFailure` reads `ep.LastStatusCode` from the endpoint -- no Notifier interface signature change needed
- **D-08:** Notification format: show "HTTP: 503" when `LastStatusCode > 0`, show "HTTP: connection error" when `LastStatusCode == 0` -- use `FormatFailureWithCode` for the former, `FormatFailure` for the latter (or unify into one function with conditional logic)
- **D-09:** Show `last_status_code` in the endpoint detail view (`FormatEndpointDetail`) when the endpoint is in `not_ok` status
- **D-10:** `FormatFailureWithCode` becomes live code after D-07/D-08 wiring -- no longer dead
- **D-11:** Perform a focused sweep of all exported functions and types; remove any that have zero callers in the codebase
- **D-12:** Scope is tight -- just exported symbols, not internal refactoring
- **D-13:** Allowed characters: `[a-zA-Z0-9_-]` (alphanumeric, hyphens, underscores)
- **D-14:** Length: minimum 1 character, maximum 50 characters
- **D-15:** No leading or trailing hyphens allowed
- **D-16:** Case-preserving -- don't force lowercase (users may want "MyAPI" or "prod-server")
- **D-17:** Add `ValidateName(name string) error` to `internal/bot/validate.go`
- **D-18:** Error message: "Name must be 1-50 characters: letters, numbers, hyphens, underscores"
- **D-19:** Call validation in the `/add` handler before any store interaction

### Claude's Discretion
- Whether to unify `FormatFailure` and `FormatFailureWithCode` into one function or keep both with conditional dispatch
- Exact migration file numbering (next sequential number after existing migrations)
- Order of implementation within the phase (though interface work should come first since it unblocks everything)
- Whether the dead code sweep finds anything beyond `FormatFailureWithCode` -- handle as discovered

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| QUAL-01 | Scheduler depends on a Checker interface, not concrete `*HTTPChecker` | D-01/D-02/D-03: Define `Checker` interface at point of use in scheduler.go, change field type and constructor parameter. HTTPChecker already satisfies it. Existing tests use concrete checker -- they compile unchanged. |
| QUAL-02 | HTTP status code flows through notification pipeline -- `NotifyFailure` receives status code and uses `FormatFailureWithCode` for non-zero codes | D-07/D-08: No Notifier interface change needed. `NotifyFailure` reads `ep.LastStatusCode` (persisted by RecordFailure). Conditional formatting dispatch. |
| QUAL-03 | Store methods either persist `statusCode` to a `last_status_code` DB column or remove the unused parameter from signatures | D-04/D-05/D-06: New goose migration 003 adds `last_status_code` column. All three store methods updated to SET the column. All scan calls updated to read it. |
| QUAL-04 | Dead code removed -- unused functions, stale references cleaned up | D-10/D-11/D-12: `FormatFailureWithCode` becomes live after QUAL-02 wiring. Research found `ErrInvalidInput` is defined but never used -- becomes live after QUAL-05 (if used for validation errors, discretion). `SendMessage`/`SendSilentMessage` are only called within `internal/bot` but serve as legitimate public API on `Bot` -- keep exported. |
| QUAL-05 | Endpoint name validation enforced (alphanumeric, hyphens, underscores, max 50 chars) | D-13 through D-19: Add `ValidateName` in validate.go with regex `^[a-zA-Z0-9][a-zA-Z0-9_-]*[a-zA-Z0-9]$` (or character-level check for 1-char names). Call in handleAdd before store interaction. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **No new dependencies.** Only the libraries in CLAUDE.md's Mandatory Libraries table.
- **CGO_ENABLED=0** must pass for build.
- **go vet ./...** must pass before every commit.
- **go test ./...** must pass before every commit.
- **Interfaces at point of use.** The new `Checker` interface goes in `scheduler.go` (the consumer), not in `checker.go`.
- **context.Background() only in main.go and tests.** All I/O functions take `context.Context` first param.
- **Goose migrations only.** Never inline CREATE TABLE in Go code.
- **stdlib testing only.** No testify, no gomock.
- **Table-driven tests** where applicable.
- **Every non-main package must have _test.go files.** (Exception: internal/bot historically, but it already has format_test.go and validate_test.go.)
- **apperror.Wrap** for error wrapping, **errors.Is** for checking.
- **HTML-escape** all user-provided content in Telegram messages.

## Standard Stack

This phase uses only existing dependencies. No additions.

### Core (already in go.mod)
| Library | Version | Purpose | Phase Usage |
|---------|---------|---------|-------------|
| `github.com/pressly/goose/v3` | v3.27.0 | DB migrations | New migration 003 for `last_status_code` column |
| `modernc.org/sqlite` | v1.46.1 | SQLite driver | Existing -- used by store tests with `:memory:` DB |
| `database/sql` | stdlib | SQL interface | Existing -- all store methods |
| `testing` | stdlib | Test framework | Update existing tests for new field |
| `regexp` | stdlib | Name validation | New usage in `ValidateName` |

### No New Dependencies
This phase adds zero new packages to go.mod. Every tool needed is already available.

## Architecture Patterns

### File Modification Map

```
internal/monitor/scheduler.go     # Add Checker interface, change field/constructor
internal/storage/models.go        # Add LastStatusCode field to Endpoint
internal/storage/store.go         # Update SQL in 3 methods + all Scan calls
internal/storage/migrations/003_add_last_status_code.sql  # New goose migration
internal/bot/bot.go               # Update NotifyFailure to use ep.LastStatusCode
internal/bot/format.go            # Update FormatEndpointDetail for status code display
internal/bot/validate.go          # Add ValidateName function
internal/bot/handlers.go          # Add ValidateName call in handleAdd
internal/storage/store_test.go    # Update tests for new column
internal/monitor/scheduler_test.go # Update mockStore (if needed)
internal/bot/format_test.go       # Update FormatEndpointDetail tests for status code
internal/bot/validate_test.go     # Add ValidateName tests
cmd/monitor/main.go               # Update NewScheduler call (type change)
```

### Pattern 1: Interface at Point of Use (QUAL-01)
**What:** Define the `Checker` interface where it is consumed, not where it is implemented.
**When to use:** Always, per CLAUDE.md convention.
**Example:**
```go
// In internal/monitor/scheduler.go, alongside existing Store and Notifier interfaces:

// Checker performs HTTP health checks.
type Checker interface {
    Check(ctx context.Context, url string) (int, error)
}

// Scheduler manages periodic health checks using gocron.
type Scheduler struct {
    cron                    gocron.Scheduler
    store                   Store
    checker                 Checker  // was *HTTPChecker
    notifier                Notifier
    maxFailureNotifications int
    ctx                     context.Context
}

func NewScheduler(ctx context.Context, store Store, checker Checker, notifier Notifier, maxFailureNotifications int) (*Scheduler, error) {
    // ... unchanged body
}
```

### Pattern 2: Goose Migration for Column Addition (QUAL-03)
**What:** Use ALTER TABLE ADD COLUMN for simple nullable column additions in SQLite.
**When to use:** When adding a column with no UNIQUE, NOT NULL, or expression default constraints.
**Example:**
```sql
-- +goose Up
ALTER TABLE endpoints ADD COLUMN last_status_code INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE endpoints DROP COLUMN last_status_code;
```

Note: SQLite supports ALTER TABLE ADD COLUMN (verified). The column has DEFAULT 0, so existing rows get 0 (meaning "no status code recorded yet" -- same semantic as "connection error"). SQLite 3.35.0+ supports DROP COLUMN for the down migration. The project uses Alpine 3.21 which ships SQLite 3.47+, so DROP COLUMN is available.

### Pattern 3: Conditional Notification Formatting (QUAL-02)
**What:** Use `ep.LastStatusCode` to decide formatting, no interface change needed.
**When to use:** The Notifier interface signature stays the same because the endpoint already carries the status code after RecordFailure persists it.
**Example:**
```go
// In bot.go NotifyFailure:
func (n *TelegramNotifier) NotifyFailure(ctx context.Context, ep storage.Endpoint) error {
    var msg string
    if ep.LastStatusCode > 0 {
        msg = FormatFailureWithCode(ep, ep.LastStatusCode, n.maxFail)
    } else {
        msg = FormatFailure(ep, n.maxFail)
    }
    return n.bot.SendMessage(msg)
}
```

### Pattern 4: Validation Function (QUAL-05)
**What:** Pure function that validates a string and returns an error.
**When to use:** Follows existing `ValidateURL` pattern in the same file.
**Example:**
```go
// In internal/bot/validate.go:
func ValidateName(name string) error {
    if len(name) == 0 || len(name) > 50 {
        return fmt.Errorf("Name must be 1-50 characters: letters, numbers, hyphens, underscores")
    }
    for _, r := range name {
        if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
            return fmt.Errorf("Name must be 1-50 characters: letters, numbers, hyphens, underscores")
        }
    }
    if name[0] == '-' || name[len(name)-1] == '-' {
        return fmt.Errorf("Name must not start or end with a hyphen")
    }
    return nil
}
```

Note: Character-level checking (no `regexp` import needed) matches the style of `ValidateURL` which also does manual checks rather than regex. This avoids adding an import for a simple validation.

### Anti-Patterns to Avoid
- **Changing the Notifier interface:** D-07 explicitly locks this -- the status code comes from `ep.LastStatusCode`, not a new parameter. Changing the interface would cascade to scheduler.go, bot.go, scheduler_test.go, and future bot tests (Phase 2).
- **Table rebuild migration:** Migration 002 used the full table rebuild pattern because it added a UNIQUE column. Migration 003 just adds a nullable/defaulted column -- ALTER TABLE ADD COLUMN is sufficient and much simpler.
- **Using `regexp` for simple character validation:** The existing `ValidateURL` function uses manual string checks. Keep consistent. Regex adds an import for no benefit here.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| DB schema changes | Inline `CREATE TABLE` / `ALTER TABLE` in Go code | Goose migration SQL files | CLAUDE.md explicitly forbids inline schema. Goose handles versioning, up/down, and embedding. |
| Error type matching | String comparison on error messages | `errors.Is(err, apperror.ErrNotFound)` | CLAUDE.md mandates `errors.Is` / `errors.As`. The `AppError.Is` method compares by Code field. |

## Common Pitfalls

### Pitfall 1: Scan Call Mismatch After Adding Column
**What goes wrong:** Adding `LastStatusCode` to the `Endpoint` struct but forgetting to update one of the 5+ SELECT queries and their corresponding `Scan` calls. Runtime panic: "sql: expected N destination arguments in Scan, not N+1".
**Why it happens:** There are 6 separate query locations that scan into `Endpoint`: `GetEndpoint`, `GetEndpointByURL`, `GetEndpointByName`, `ListEndpoints`, and `RecordRecovery` (which calls `GetEndpoint`), plus `AddEndpoint` (which also calls `GetEndpoint`).
**How to avoid:** Update ALL SELECT queries to include `last_status_code` and ALL Scan calls to include `&ep.LastStatusCode`. Compile and run `go test ./internal/storage/...` immediately after.
**Warning signs:** Tests fail with "Scan" errors. Count the number of `Scan` calls in store.go -- there should be 5 (GetEndpoint, GetEndpointByURL, GetEndpointByName, ListEndpoints in the loop, and the query in RecordFailure).

### Pitfall 2: SQL UPDATE Missing New Column
**What goes wrong:** Adding the `last_status_code` column and updating `RecordFailure` to SET it, but forgetting `UpdateEndpointStatus` or `RecordRecovery`. The column gets stale data.
**Why it happens:** Three separate methods accept `statusCode int` but only RecordFailure seems "obvious" for persisting it.
**How to avoid:** All three methods -- `UpdateEndpointStatus`, `RecordFailure`, `RecordRecovery` -- must SET `last_status_code = ?` in their UPDATE queries and bind the `statusCode` parameter.
**Warning signs:** Status code shows correctly on failure but shows stale value after recovery.

### Pitfall 3: main.go Constructor Signature Change
**What goes wrong:** Changing `NewScheduler` to accept `Checker` interface but forgetting to verify `main.go` still compiles. Since `*HTTPChecker` satisfies `Checker`, this compiles silently -- but it's easy to forget if you accidentally change the wrong parameter.
**Why it happens:** The constructor change is in `internal/monitor/scheduler.go` but the caller is in `cmd/monitor/main.go`.
**How to avoid:** Run `CGO_ENABLED=0 go build ./cmd/monitor/` after the interface change.
**Warning signs:** Build failure in main.go.

### Pitfall 4: RecordRecovery Should Reset last_status_code
**What goes wrong:** After recovery, the endpoint still shows the old failure status code in the detail view because `RecordRecovery` does not reset `last_status_code` to 0 (or to the successful status code).
**Why it happens:** The existing `RecordRecovery` SQL resets `consecutive_failures`, `failure_notifications_sent`, and `last_failure_at` but the new column might be forgotten.
**How to avoid:** `RecordRecovery` should SET `last_status_code = ?` binding the `statusCode` parameter (which is the recovery status code, typically 200). The detail view should only show `last_status_code` when status is `not_ok` (per D-09), so even if it shows 200, it will not display -- but it is cleaner to update it.
**Warning signs:** After recovery, if user triggers another failure, the detail view might flash the old code briefly.

### Pitfall 5: Migration Numbering
**What goes wrong:** Naming the migration `002_*.sql` instead of `003_*.sql`, causing goose to fail because 002 already exists.
**Why it happens:** Not checking existing migration files.
**How to avoid:** Next migration is `003_add_last_status_code.sql`. Existing: 001, 002.

### Pitfall 6: Dead Code Sweep Removing Test-Only Exports
**What goes wrong:** The dead code sweep (D-11) might identify `FormatFailureWithCode` as dead before it gets wired up (if sweep runs first), or might miss items that become dead after other changes.
**Why it happens:** Order of operations matters.
**How to avoid:** Do the status code pipeline (which wires `FormatFailureWithCode`) BEFORE the dead code sweep. After wiring, the sweep will correctly identify only truly dead symbols.

## Code Examples

### Exact Current State of Files to Modify

Key integration points with exact line references:

**scheduler.go:32** -- `checker *HTTPChecker` becomes `checker Checker`
**scheduler.go:39** -- `checker *HTTPChecker` parameter becomes `checker Checker`
**scheduler.go:97** -- `s.checker.Check(ctx, ep.URL)` unchanged (same method signature)

**store.go:175-192** -- `UpdateEndpointStatus` currently ignores `statusCode`. SQL must change to SET `last_status_code = ?`.
**store.go:212-231** -- `RecordFailure` currently ignores `statusCode`. SQL must change to SET `last_status_code = ?`.
**store.go:233-261** -- `RecordRecovery` currently ignores `statusCode`. SQL must change to SET `last_status_code = ?`.

**store.go: All SELECT queries** -- Must add `last_status_code` to column list (5 locations total):
- Line 79-80 (GetEndpoint)
- Line 98-99 (GetEndpointByURL)
- Line 117-118 (GetEndpointByName)
- Line 150-151 (ListEndpoints)
(AddEndpoint delegates to GetEndpoint, RecordFailure delegates to GetEndpoint, RecordRecovery queries via GetEndpoint.)

**bot.go:131-134** -- `NotifyFailure` currently calls `FormatFailure(ep, n.maxFail)`. Must add conditional dispatch using `ep.LastStatusCode`.

**format.go:152-185** -- `FormatEndpointDetail` must add status code display when `ep.Status == "not_ok" && ep.LastStatusCode > 0`.

**handlers.go:33** -- After `name := args[0]`, add `ValidateName(name)` call before `ValidateURL`.

**main.go:60** -- `monitor.NewScheduler(ctx, store, checker, ...)` -- no code change needed, just verify it compiles (checker type changes but `*HTTPChecker` satisfies `Checker`).

### Dead Code Inventory (Research Finding)

Exported symbols with zero production callers (verified by grep):

| Symbol | Location | Status After Phase |
|--------|----------|-------------------|
| `FormatFailureWithCode` | format.go:88 | BECOMES LIVE (D-08 wires it via NotifyFailure) |
| `ErrInvalidInput` | apperror.go:45 | Defined but unused. Stays unused unless ValidateName uses it (discretion). Current ValidateURL returns plain errors, so ValidateName should too for consistency. |
| `SendMessage` | bot.go:106 | Only called within internal/bot (by TelegramNotifier). Legitimate public API surface on Bot -- KEEP exported. |
| `SendSilentMessage` | bot.go:113 | Same as SendMessage -- KEEP exported. |

**Recommendation:** After wiring FormatFailureWithCode (D-08), the only potentially dead exported symbol is `ErrInvalidInput`. Since it is a defined sentinel in the error package and may be used by Phase 2 (bot handler tests), leave it. It represents intentional API surface, not accidental dead code.

### Test Impact Analysis

| Test File | Changes Needed | Reason |
|-----------|---------------|--------|
| `store_test.go` | Update tests that verify RecordFailure and RecordRecovery to check `LastStatusCode` field | New field in Endpoint struct |
| `scheduler_test.go` | Update `NewScheduler` calls (type already compatible). Optionally add test with mock checker. | Constructor accepts `Checker` interface now |
| `format_test.go` | Add test for `FormatEndpointDetail` showing status code. Update existing tests if Endpoint struct changes break compilation. | New `LastStatusCode` field in test struct literals |
| `validate_test.go` | Add `TestValidateName` table-driven test | New function |
| `checker_test.go` | No changes needed | HTTPChecker unchanged |
| `apperror_test.go` | No changes needed | No apperror changes |
| `config_test.go` | No changes needed | No config changes |

### Migration File

```sql
-- File: internal/storage/migrations/003_add_last_status_code.sql

-- +goose Up
ALTER TABLE endpoints ADD COLUMN last_status_code INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE endpoints DROP COLUMN last_status_code;
```

SQLite supports `ALTER TABLE ADD COLUMN` for non-UNIQUE, non-PRIMARY-KEY columns with a default value. SQLite 3.35.0+ supports `DROP COLUMN`. Alpine 3.21 ships SQLite 3.47+.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | stdlib `testing` (Go 1.26.1) |
| Config file | None -- Go test conventions, no config file |
| Quick run command | `go test ./...` |
| Full suite command | `CGO_ENABLED=0 go build ./cmd/monitor/ && go vet ./... && go test ./...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| QUAL-01 | Scheduler accepts Checker interface | unit | `go test ./internal/monitor/ -run TestCheckAndNotify -x` | Existing (scheduler_test.go) -- tests use concrete HTTPChecker, still compile after interface extraction |
| QUAL-02 | NotifyFailure uses FormatFailureWithCode for non-zero status codes | unit | `go test ./internal/bot/ -run TestFormatFailure -x` | Existing (format_test.go) -- add test for NotifyFailure dispatch logic if testable |
| QUAL-03 | last_status_code column persisted by store methods | unit | `go test ./internal/storage/ -run TestRecordFailure -x` | Existing (store_test.go) -- update to assert LastStatusCode field |
| QUAL-04 | No dead exported symbols | build | `CGO_ENABLED=0 go build ./cmd/monitor/ && go vet ./...` | N/A -- verified by compilation and grep sweep |
| QUAL-05 | Name validation rejects invalid names | unit | `go test ./internal/bot/ -run TestValidateName -x` | New -- Wave 0 gap |

### Sampling Rate
- **Per task commit:** `go test ./...`
- **Per wave merge:** `CGO_ENABLED=0 go build ./cmd/monitor/ && go vet ./... && go test ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/bot/validate_test.go::TestValidateName` -- covers QUAL-05
- [ ] `internal/storage/store_test.go` -- update existing RecordFailure/RecordRecovery tests to assert `LastStatusCode` field -- covers QUAL-03

(No framework install needed -- stdlib testing already in use. No new config files needed.)

## Open Questions

1. **Unify FormatFailure/FormatFailureWithCode or keep both?**
   - What we know: D-08 says "use FormatFailureWithCode for the former, FormatFailure for the latter (or unify into one function with conditional logic)". Claude's discretion area.
   - Recommendation: **Unify into one function** with conditional logic. Both functions are nearly identical (only the HTTP status line differs). A single `FormatFailure(ep storage.Endpoint, maxFailures int) string` that checks `ep.LastStatusCode > 0` is cleaner. However, this changes the existing `FormatFailure` signature (no, it does not -- LastStatusCode is on the Endpoint struct). Keep both functions but have `NotifyFailure` dispatch between them. This avoids changing existing test expectations for `FormatFailure`.
   - **Final recommendation:** Keep both functions. `NotifyFailure` dispatches. Simpler, less test churn, and both functions remain independently testable.

2. **Should ValidateName return apperror.ErrInvalidInput?**
   - What we know: `ValidateURL` returns plain `fmt.Errorf` errors. `ErrInvalidInput` exists but is unused.
   - Recommendation: **Follow ValidateURL pattern -- return plain errors.** The handler already catches the error and sends a user-friendly message directly. Using `apperror.ErrInvalidInput` would add complexity for no benefit here. The sentinel is better suited for cases where the caller needs to distinguish error types, which is not the case for validation in handlers (they just send the error message to the user).

## Sources

### Primary (HIGH confidence)
- Direct codebase inspection of all files in `internal/` and `cmd/monitor/` -- all code findings are verified against actual source
- SQLite ALTER TABLE documentation: https://www.sqlite.org/lang_altertable.html -- confirms ADD COLUMN and DROP COLUMN support

### Secondary (MEDIUM confidence)
- Migration 002 pattern establishes convention for this project's goose usage
- CONTEXT.md decisions D-01 through D-19 lock implementation approach

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all libraries already in use and verified in go.mod
- Architecture: HIGH -- patterns follow existing codebase conventions exactly, all file locations and line numbers verified
- Pitfalls: HIGH -- derived from actual code inspection, identified all 5+ Scan call locations and 3 UPDATE methods that need changes

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable -- all dependencies pinned, no external API changes expected)

# Phase 1: Interface Cleanup and Tech Debt - Context

**Gathered:** 2026-03-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Fix interface violations, wire dead code, remove unused parameters, and add input validation. Every interface follows point-of-use convention, no exported symbols are dead code, and all user input is validated. This phase creates a clean foundation for Phase 2 (bot handler tests) by making all dependencies mockable.

</domain>

<decisions>
## Implementation Decisions

### Checker interface (QUAL-01)
- **D-01:** Define a `Checker` interface in `internal/monitor/scheduler.go` with method `Check(ctx context.Context, url string) (int, error)` — same signature as the existing `HTTPChecker.Check`
- **D-02:** Change `Scheduler.checker` field from `*HTTPChecker` to `Checker`, and update `NewScheduler` parameter accordingly
- **D-03:** `HTTPChecker` already satisfies the interface — no changes to `checker.go` needed

### Status code pipeline (QUAL-02 + QUAL-03)
- **D-04:** Persist `statusCode` to a new `last_status_code` INTEGER column via goose migration (not remove the parameter) — this enables status display in notifications and detail view
- **D-05:** Add `LastStatusCode int` field to `storage.Endpoint` model and update all scan calls
- **D-06:** Store methods (`UpdateEndpointStatus`, `RecordFailure`, `RecordRecovery`) persist the `statusCode` parameter to the new column
- **D-07:** `NotifyFailure` reads `ep.LastStatusCode` from the endpoint — no Notifier interface signature change needed
- **D-08:** Notification format: show "HTTP: 503" when `LastStatusCode > 0`, show "HTTP: connection error" when `LastStatusCode == 0` — use `FormatFailureWithCode` for the former, `FormatFailure` for the latter (or unify into one function with conditional logic)
- **D-09:** Show `last_status_code` in the endpoint detail view (`FormatEndpointDetail`) when the endpoint is in `not_ok` status

### Dead code cleanup (QUAL-04)
- **D-10:** `FormatFailureWithCode` becomes live code after D-07/D-08 wiring — no longer dead
- **D-11:** Perform a focused sweep of all exported functions and types; remove any that have zero callers in the codebase
- **D-12:** Scope is tight — just exported symbols, not internal refactoring

### Name validation (QUAL-05)
- **D-13:** Allowed characters: `[a-zA-Z0-9_-]` (alphanumeric, hyphens, underscores)
- **D-14:** Length: minimum 1 character, maximum 50 characters
- **D-15:** No leading or trailing hyphens allowed
- **D-16:** Case-preserving — don't force lowercase (users may want "MyAPI" or "prod-server")
- **D-17:** Add `ValidateName(name string) error` to `internal/bot/validate.go`
- **D-18:** Error message: "Name must be 1-50 characters: letters, numbers, hyphens, underscores"
- **D-19:** Call validation in the `/add` handler before any store interaction

### Claude's Discretion
- Whether to unify `FormatFailure` and `FormatFailureWithCode` into one function or keep both with conditional dispatch
- Exact migration file numbering (next sequential number after existing migrations)
- Order of implementation within the phase (though interface work should come first since it unblocks everything)
- Whether the dead code sweep finds anything beyond `FormatFailureWithCode` — handle as discovered

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

No external specs — requirements fully captured in decisions above and in these project files:

### Requirements and constraints
- `.planning/REQUIREMENTS.md` — QUAL-01 through QUAL-05 acceptance criteria
- `.planning/ROADMAP.md` §Phase 1 — Success criteria (5 items)
- `CLAUDE.md` — Mandatory libraries, code style, interface conventions, error handling patterns

### Codebase analysis
- `.planning/codebase/` — 7 analysis documents from 2026-03-26 (architecture, conventions, tech debt)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/bot/validate.go`: Already has `ValidateURL` — add `ValidateName` alongside it
- `internal/bot/format.go`: `FormatFailureWithCode` already written, just needs to be wired in
- `internal/apperror/`: Error types and sentinels ready for use in validation errors

### Established Patterns
- Interfaces defined at point of use (bot defines `Store`, `Scheduler`; scheduler defines `Store`, `Notifier`)
- Goose migrations in `internal/storage/migrations/*.sql` with `-- +goose Up/Down`
- All store methods take `context.Context` first, use `apperror.Wrap` for errors
- Endpoint model in `internal/storage/models.go` with `sql.NullTime` for optional fields

### Integration Points
- `Scheduler` struct in `scheduler.go:29-36` — field and constructor change for Checker interface
- `Store` interface in `scheduler.go:15-19` — methods already accept `statusCode int`, just need to persist it
- `NotifyFailure` in `bot.go:131-133` — reads endpoint, calls formatter (wiring point for status code display)
- `/add` handler in `handlers.go` — validation call point for name validation
- `FormatEndpointDetail` in `format.go:152` — add status code display for failed endpoints

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches. User asked for sensible defaults.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 01-interface-cleanup-and-tech-debt*
*Context gathered: 2026-03-26*

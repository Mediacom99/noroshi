---
phase: 01-interface-cleanup-and-tech-debt
plan: 02
subsystem: bot
tags: [go-validation, notification-pipeline, dead-code-sweep, telegram-bot]

# Dependency graph
requires:
  - phase: 01-01
    provides: "LastStatusCode field in Endpoint model and persisted in all store queries"
provides:
  - ValidateName function for endpoint name input validation
  - NotifyFailure conditional dispatch using LastStatusCode
  - FormatEndpointDetail with HTTP status code display
  - Dead code sweep complete (FormatFailureWithCode now live)
affects: [phase-2-tests]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Character-level validation matching existing ValidateURL manual style"
    - "Conditional notification dispatch based on status code value"

key-files:
  created: []
  modified:
    - internal/bot/validate.go
    - internal/bot/validate_test.go
    - internal/bot/handlers.go
    - internal/bot/bot.go
    - internal/bot/format.go
    - internal/bot/format_test.go

key-decisions:
  - "ValidateName uses character-level checking (no regexp) to match ValidateURL style"
  - "ValidateName wired before ValidateURL in handleAdd to fail fast on name issues"
  - "ErrInvalidInput sentinel kept as intentional API surface for Phase 2 bot handler tests"

patterns-established:
  - "Input validation: define at bot package, call in handler before store interaction"

requirements-completed: [QUAL-02, QUAL-04, QUAL-05]

# Metrics
duration: 3min
completed: 2026-03-26
---

# Phase 01 Plan 02: Status Code Wiring, Dead Code Sweep, Name Validation Summary

**ValidateName with 15 table-driven tests, NotifyFailure dispatching by HTTP status code, and dead code sweep eliminating orphaned FormatFailureWithCode**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-26T09:48:38Z
- **Completed:** 2026-03-26T09:51:47Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Added `ValidateName` function enforcing 1-50 char length, `[a-zA-Z0-9_-]` character set, no leading/trailing hyphens -- with 15 table-driven test cases
- Wired `ValidateName` into `handleAdd` before `ValidateURL`, rejecting malformed names before any store interaction
- Updated `NotifyFailure` to conditionally dispatch to `FormatFailureWithCode` (HTTP status > 0) or `FormatFailure` (connection error), making `FormatFailureWithCode` live code
- Added HTTP status code display to `FormatEndpointDetail` for failed endpoints with non-zero status codes
- Confirmed dead code sweep complete: no exported symbols without callers remain

## Task Commits

Each task was committed atomically:

1. **Task 1: Add ValidateName function with tests and wire into handleAdd** - `8b6468e` (feat)
   - TDD RED: `db4b33a` (test) - failing tests for ValidateName
   - TDD GREEN: `8b6468e` (feat) - implementation + handler wiring
2. **Task 2: Wire status code into NotifyFailure dispatch, update detail view, sweep dead code** - `8f3ca5f` (feat)

## Files Created/Modified
- `internal/bot/validate.go` - Added `ValidateName` function with character-level validation
- `internal/bot/validate_test.go` - Added `TestValidateName` with 15 table-driven test cases
- `internal/bot/handlers.go` - Added `ValidateName` call in `handleAdd` before `ValidateURL`
- `internal/bot/bot.go` - Updated `NotifyFailure` with conditional dispatch based on `ep.LastStatusCode`
- `internal/bot/format.go` - Added HTTP status code line to `FormatEndpointDetail` for failed endpoints
- `internal/bot/format_test.go` - Updated `TestFormatEndpointDetail` with `LastStatusCode: 503`, added `TestFormatEndpointDetailNoStatusCode`

## Decisions Made
- Used character-level checking (no regexp) in `ValidateName` to match `ValidateURL`'s manual validation style -- consistent code style across the package
- `ValidateName` is called before `ValidateURL` in `handleAdd` to fail fast on name issues before URL parsing
- `ErrInvalidInput` sentinel in `apperror` package kept despite having no current callers -- it's intentional API surface that will be used in Phase 2 bot handler tests

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Phase 1 is now complete: all 5 QUAL requirements addressed (QUAL-01 through QUAL-05)
- Bot package is ready for Phase 2 (handler tests): ValidateName, FormatFailureWithCode, and all handlers are wired and testable
- All exported symbols have callers, enabling clean dead-code analysis in future phases

## Self-Check: PASSED

All 6 files verified present. All 3 task commits (db4b33a, 8b6468e, 8f3ca5f) verified in git log.

---
*Phase: 01-interface-cleanup-and-tech-debt*
*Completed: 2026-03-26*

---
phase: 01-interface-cleanup-and-tech-debt
plan: 01
subsystem: monitor, storage
tags: [go-interfaces, sqlite, goose-migration, dependency-injection]

# Dependency graph
requires: []
provides:
  - Checker interface enabling mock-based scheduler testing
  - last_status_code column persisted in all endpoint queries and updates
  - Updated Endpoint model with LastStatusCode field
affects: [01-02, phase-2-tests]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Interface extraction at point of use (Checker in scheduler.go)"
    - "Migration-driven schema evolution for new columns"

key-files:
  created:
    - internal/storage/migrations/003_add_last_status_code.sql
  modified:
    - internal/monitor/scheduler.go
    - internal/storage/models.go
    - internal/storage/store.go
    - internal/storage/store_test.go

key-decisions:
  - "Checker interface placed in scheduler.go alongside Store and Notifier (point-of-use convention)"
  - "last_status_code defaults to 0 (same as connection-error status code from HTTPChecker)"

patterns-established:
  - "Interface extraction: define at consumer, concrete type satisfies implicitly"

requirements-completed: [QUAL-01, QUAL-03]

# Metrics
duration: 3min
completed: 2026-03-26
---

# Phase 01 Plan 01: Checker Interface and Status Code Persistence Summary

**Checker interface extracted in scheduler.go for mock-based testing; last_status_code persisted across all store queries and update methods with migration 003**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-26T09:40:59Z
- **Completed:** 2026-03-26T09:44:17Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Extracted `Checker` interface in `scheduler.go` so `Scheduler` depends on an interface, not the concrete `*HTTPChecker` -- unblocks mock-based scheduler tests in Phase 2
- Created goose migration 003 adding `last_status_code INTEGER NOT NULL DEFAULT 0` column to endpoints table
- Updated all 4 SELECT queries, 3 UPDATE queries, and all Scan calls in `store.go` to read/write `last_status_code`
- Added `LastStatusCode` assertions to 4 existing store tests confirming end-to-end persistence

## Task Commits

Each task was committed atomically:

1. **Task 1: Extract Checker interface and add last_status_code migration + model** - `1adadb1` (feat)
2. **Task 2: Update all store SQL queries and tests to persist last_status_code** - `a419498` (feat)

## Files Created/Modified
- `internal/monitor/scheduler.go` - Added Checker interface, changed Scheduler field and NewScheduler parameter from `*HTTPChecker` to `Checker`
- `internal/storage/migrations/003_add_last_status_code.sql` - Goose migration adding last_status_code column
- `internal/storage/models.go` - Added `LastStatusCode int` field to Endpoint struct
- `internal/storage/store.go` - Updated all SELECT column lists, Scan calls, and UPDATE statements to include last_status_code
- `internal/storage/store_test.go` - Added LastStatusCode assertions to TestAddAndGetEndpoint, TestRecordFailure, TestRecordRecovery, TestUpdateEndpointStatus

## Decisions Made
- Placed Checker interface in `scheduler.go` alongside existing Store and Notifier interfaces, following the project convention of defining interfaces at point of use
- `last_status_code` defaults to 0 (integer), matching the value HTTPChecker returns on connection errors -- semantically consistent

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Checker interface is ready for Plan 02 (notification pipeline) and Phase 2 (mock-based scheduler tests)
- `last_status_code` is now persisted and available for display in failure/recovery notifications (Plan 02 scope)
- All existing tests pass, no regressions introduced

## Self-Check: PASSED

All 5 files verified present. Both task commits (1adadb1, a419498) verified in git log.

---
*Phase: 01-interface-cleanup-and-tech-debt*
*Completed: 2026-03-26*

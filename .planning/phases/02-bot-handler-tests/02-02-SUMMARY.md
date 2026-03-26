---
phase: 02-bot-handler-tests
plan: 02
subsystem: testing
tags: [go, testing, mocks, table-driven-tests, telegram-bot, callbacks, scheduler]

requires:
  - phase: 02-bot-handler-tests
    plan: 01
    provides: "mockContext, mockStore, mockScheduler shared test infrastructure in internal/bot/mock_test.go"
provides:
  - "Table-driven tests for all 7 callback handlers in internal/bot/callbacks_test.go"
  - "mockChecker type and 5 mock-based deterministic scheduler tests in internal/monitor/scheduler_test.go"
  - "Checker interface at point of use in scheduler.go enabling mock-based testing"
affects: [03-ci-pipeline]

tech-stack:
  added: []
  patterns:
    - "Checker interface at point of use in scheduler.go (was concrete *HTTPChecker)"
    - "mockChecker with function-field pattern for deterministic HTTP check simulation"
    - "Respond/Edit assertion pattern for callback handler testing"

key-files:
  created:
    - internal/bot/callbacks_test.go
  modified:
    - internal/monitor/scheduler.go
    - internal/monitor/scheduler_test.go

key-decisions:
  - "Extracted Checker interface from concrete *HTTPChecker to enable mock injection in scheduler tests"
  - "Used respondCalls slice scanning (not index-0 check) for confirm-delete and set-interval handlers that issue multiple Respond calls"

patterns-established:
  - "Callback test pattern: inject data via callbackFn, assert on editedMessages and respondCalls"
  - "Mock-based scheduler tests run in 0ms vs 1.5s+ for httptest-based equivalents"

requirements-completed: [TEST-03, TEST-04]

duration: 5min
completed: 2026-03-26
---

# Phase 02 Plan 02: Callback Handler Tests and Mock Scheduler Tests Summary

**30 table-driven callback tests covering all 7 inline keyboard handlers, plus 5 deterministic mock-based scheduler tests with Checker interface extraction**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-26T12:04:19Z
- **Completed:** 2026-03-26T12:09:29Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- Created 30 table-driven test cases across 7 callback handler test functions covering happy paths, error paths, scheduler interactions, and nil-scheduler safety
- Added mockChecker type and 5 mock-based scheduler tests that run deterministically without HTTP overhead
- Extracted Checker interface at point of use in scheduler.go, following project convention (was concrete *HTTPChecker)
- Full test suite passes: `go test ./...` green with zero regressions, all existing httptest-based tests preserved

## Task Commits

Each task was committed atomically:

1. **Task 1: Create table-driven tests for all 7 callback handlers** - `e23fa7f` (test)
2. **Task 2: Add mockChecker and mock-based scheduler tests** - `c177e35` (test)

## Files Created/Modified
- `internal/bot/callbacks_test.go` - 30 test cases: TestHandleDetailCallback (3), TestHandleDeleteCallback (3), TestHandleConfirmDeleteCallback (7), TestHandleBackCallback (3), TestHandleIntervalCallback (3), TestHandleSetIntervalCallback (8), TestHandleRefreshCallback (3)
- `internal/monitor/scheduler.go` - Added Checker interface, changed checker field and NewScheduler parameter from *HTTPChecker to Checker
- `internal/monitor/scheduler_test.go` - Added mockChecker type, newMockScheduler helper, and 5 mock-based tests (MockOK, MockFailure, MockConnectionError, MockFailureCap, MockRecovery)

## Decisions Made
- **Extracted Checker interface:** The scheduler used concrete `*HTTPChecker` type, preventing mock injection. Extracted a `Checker` interface with `Check(ctx, url) (int, error)` at point of use in `scheduler.go`, following the project's established pattern (interfaces at consumer side). This is a backward-compatible change since `*HTTPChecker` satisfies the interface.
- **Respond call scanning:** For handlers that issue multiple `c.Respond()` calls (confirm-delete issues "Deleted!" then editEndpointList; set-interval issues "Interval updated" then editEndpointList), used slice scanning instead of index-0 assertion to match the expected text.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Extracted Checker interface from concrete *HTTPChecker**
- **Found during:** Task 2 (mock-based scheduler tests)
- **Issue:** The plan assumed a `Checker` interface existed in `scheduler.go` (referenced in plan interfaces section and research D-09), but the actual code used concrete `*HTTPChecker` type. The `NewScheduler` function accepted `*HTTPChecker`, making mock injection impossible.
- **Fix:** Added `Checker` interface with `Check(ctx context.Context, url string) (int, error)` method in `scheduler.go`. Changed `Scheduler.checker` field from `*HTTPChecker` to `Checker`. Changed `NewScheduler` parameter from `*HTTPChecker` to `Checker`. This follows the project's convention of defining interfaces at point of use.
- **Files modified:** internal/monitor/scheduler.go
- **Verification:** All existing tests pass (httptest-based tests still work since `*HTTPChecker` satisfies `Checker`), build succeeds, `main.go` compiles without changes.
- **Committed in:** c177e35 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Essential for enabling mock-based scheduler testing. Follows established project convention. No scope creep.

## Issues Encountered
None

## Known Stubs
None -- all tests exercise real handler logic through mocks, no placeholder data.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Complete bot handler test coverage: 30 handler tests (Plan 01) + 30 callback tests (Plan 02) = 60 bot test cases total
- All packages now have test coverage: apperror, config, storage, monitor, bot
- CI pipeline (Phase 03) can use `go test ./...` as-is -- full suite passes green
- Mock-based scheduler tests demonstrate the deterministic testing benefit of the Checker interface extraction

## Self-Check: PASSED

- FOUND: internal/bot/callbacks_test.go
- FOUND: internal/monitor/scheduler_test.go
- FOUND: internal/monitor/scheduler.go
- FOUND: 02-02-SUMMARY.md
- FOUND: commit e23fa7f (Task 1)
- FOUND: commit c177e35 (Task 2)

---
*Phase: 02-bot-handler-tests*
*Completed: 2026-03-26*

---
phase: 03-ci-pipeline
plan: 01
subsystem: infra
tags: [golangci-lint, errorlint, linting, ci, go]

# Dependency graph
requires:
  - phase: 02-tests
    provides: passing test suite that linting must not break
provides:
  - .golangci.yml with golangci-lint v2 config (5 linters, version: "2")
  - errorlint-clean Go source (errors.Is throughout)
affects: [03-ci-pipeline/03-02]

# Tech tracking
tech-stack:
  added: [golangci-lint v2.11.4 (local), golangci-lint-action@v9 (CI)]
  patterns:
    - "golangci-lint v2 config: version: \"2\" + linters.default: none + explicit enable list"
    - "errors.Is() for all sentinel error comparisons (not == or !=)"

key-files:
  created:
    - .golangci.yml
  modified:
    - internal/storage/store.go
    - cmd/monitor/main.go
    - internal/apperror/apperror_test.go

key-decisions:
  - "golangci-lint v2 with linters.default: none enables exactly 5 linters (bodyclose, contextcheck, errorlint, sloglint, sqlclosecheck) with no surprises"
  - "All 4 errorlint findings fixed as real code changes — no nolint suppressions"
  - "contextcheck did not flag bot handlers (b.rootCtx pattern accepted without nolint comments)"

patterns-established:
  - "golangci-lint v2 config must have version: \"2\" as first non-comment line"
  - "All sentinel error comparisons use errors.Is() — applies to test files too"

requirements-completed:
  - CICD-02

# Metrics
duration: 2min
completed: 2026-03-28
---

# Phase 3 Plan 1: Fix Lint Violations and Create golangci-lint v2 Config Summary

**golangci-lint v2 config with 5 linters plus 4 errorlint fixes making the entire codebase lint-clean with zero findings**

## Performance

- **Duration:** ~2 min
- **Started:** 2026-03-28T09:58:54Z
- **Completed:** 2026-03-28T10:00:45Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- Created `.golangci.yml` with golangci-lint v2 config (version: "2", default: none, 5 linters)
- Fixed all 4 errorlint violations: 3 in `internal/storage/store.go`, 1 in `cmd/monitor/main.go`
- Fixed 1 additional errorlint finding discovered at runtime in `internal/apperror/apperror_test.go`
- golangci-lint reports 0 issues; build, vet, and tests all pass

## Task Commits

Each task was committed atomically:

1. **Task 1: Fix errorlint violations and create golangci-lint v2 config** - `3a54504` (chore)
2. **Task 2: Fix errorlint finding in apperror test** - `fbb8d2b` (fix)

## Files Created/Modified

- `.golangci.yml` - golangci-lint v2 configuration with version: "2", default: none, 5 linters enabled
- `internal/storage/store.go` - Added "errors" import; replaced 3 `err == sql.ErrNoRows` with `errors.Is(err, sql.ErrNoRows)`
- `cmd/monitor/main.go` - Added "errors" import; replaced `err != http.ErrServerClosed` with `!errors.Is(err, http.ErrServerClosed)`
- `internal/apperror/apperror_test.go` - Replaced `wrapped.Unwrap() != cause` with `!errors.Is(wrapped.Unwrap(), cause)`

## Decisions Made

- Used `linters.default: none` (not `standard`) to ensure exactly 5 linters run with zero ambiguity
- Fixed all violations as real code changes per D-04 — no nolint comments on pre-existing code
- contextcheck did not flag `b.rootCtx` usages in bot handlers — no nolint comments needed

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Additional errorlint finding in apperror_test.go not in plan's known violations list**
- **Found during:** Task 2 (golangci-lint run)
- **Issue:** `wrapped.Unwrap() != cause` at line 30 of `apperror_test.go` was flagged by errorlint — not listed in the plan's 4 known violations
- **Fix:** Replaced with `!errors.Is(wrapped.Unwrap(), cause)` which is semantically equivalent (plain fmt.Errorf errors use pointer equality in errors.Is)
- **Files modified:** `internal/apperror/apperror_test.go`
- **Verification:** golangci-lint reports 0 issues; go test ./... passes
- **Committed in:** `fbb8d2b` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 - bug)
**Impact on plan:** One additional errorlint finding in test file not identified during research phase; fixed as correct code change, no scope creep.

## Issues Encountered

None — the contextcheck false-positive scenario documented in the plan (Pitfall 4) did not occur. Bot handler methods using `b.rootCtx` were not flagged by contextcheck.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `.golangci.yml` is in place and lint-clean; ready for Plan 02 (GitHub Actions workflow)
- All code passes the 5 configured linters with zero findings
- Build, vet, and test suite all pass

---
*Phase: 03-ci-pipeline*
*Completed: 2026-03-28*

## Self-Check: PASSED

- FOUND: `.golangci.yml`
- FOUND: `03-01-SUMMARY.md`
- FOUND: commit `3a54504` (chore: golangci-lint config + errorlint fixes)
- FOUND: commit `fbb8d2b` (fix: apperror test errorlint)

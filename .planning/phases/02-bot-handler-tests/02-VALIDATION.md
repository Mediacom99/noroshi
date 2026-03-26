---
phase: 2
slug: bot-handler-tests
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` |
| **Config file** | none — stdlib, no config needed |
| **Quick run command** | `go test ./internal/bot/ ./internal/monitor/` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/bot/ ./internal/monitor/`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 02-01-01 | 01 | 1 | TEST-01 | unit | `go test ./internal/bot/ -run TestHandle` | ❌ W0 | ⬜ pending |
| 02-01-02 | 01 | 1 | TEST-02 | unit | `go test ./internal/bot/ -run "TestHandle(Add\|Delete\|List\|Interval\|Help)" -v` | ❌ W0 | ⬜ pending |
| 02-01-03 | 01 | 1 | TEST-03 | unit | `go test ./internal/bot/ -run "TestHandle.*Callback" -v` | ❌ W0 | ⬜ pending |
| 02-01-04 | 01 | 1 | TEST-04 | unit | `go test ./internal/monitor/ -run "TestCheckAndNotifyMock" -v` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/bot/mock_test.go` — shared mock infrastructure (TEST-01)
- [ ] `internal/bot/handlers_test.go` — command handler tests (TEST-02)
- [ ] `internal/bot/callbacks_test.go` — callback handler tests (TEST-03)
- [ ] `internal/monitor/scheduler_test.go` — add mockChecker + mock-based tests (TEST-04, modify existing file)

---

## Manual-Only Verifications

All phase behaviors have automated verification.

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending

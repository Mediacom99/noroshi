---
phase: 1
slug: interface-cleanup-and-tech-debt
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | stdlib `testing` (Go 1.26.1) |
| **Config file** | None — Go test conventions, no config file |
| **Quick run command** | `go test ./...` |
| **Full suite command** | `CGO_ENABLED=0 go build ./cmd/monitor/ && go vet ./... && go test ./...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./...`
- **After every plan wave:** Run `CGO_ENABLED=0 go build ./cmd/monitor/ && go vet ./... && go test ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01 | 1 | QUAL-01 | unit | `go test ./internal/monitor/ -run TestCheckAndNotify` | Existing | pending |
| 01-02-01 | 02 | 1 | QUAL-03 | unit | `go test ./internal/storage/ -run TestRecordFailure` | Existing (update needed) | pending |
| 01-02-02 | 02 | 1 | QUAL-02 | unit | `go test ./internal/bot/ -run TestFormatFailure` | Existing | pending |
| 01-03-01 | 03 | 2 | QUAL-04 | build | `CGO_ENABLED=0 go build ./cmd/monitor/ && go vet ./...` | N/A | pending |
| 01-04-01 | 04 | 2 | QUAL-05 | unit | `go test ./internal/bot/ -run TestValidateName` | W0 | pending |

*Status: pending · green · red · flaky*

---

## Wave 0 Requirements

- [ ] `internal/bot/validate_test.go::TestValidateName` — stubs for QUAL-05
- [ ] `internal/storage/store_test.go` — update existing RecordFailure/RecordRecovery tests to assert `LastStatusCode` field (QUAL-03)

*Existing infrastructure covers QUAL-01, QUAL-02, QUAL-04. Wave 0 fills gaps for QUAL-03 assertions and QUAL-05 test file.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Telegram notification shows HTTP status code | QUAL-02 | Telegram API not available in test environment | Send test notification via bot, verify "HTTP: 503" appears in failure message |

*All other phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending

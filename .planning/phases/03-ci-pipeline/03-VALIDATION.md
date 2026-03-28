---
phase: 3
slug: ci-pipeline
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-28
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — stdlib testing, no config needed |
| **Quick run command** | `go test ./...` |
| **Full suite command** | `CGO_ENABLED=0 go build ./cmd/monitor/ && go vet ./... && go test ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./...`
- **After every plan wave:** Run `CGO_ENABLED=0 go build ./cmd/monitor/ && go vet ./... && go test ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 03-01-01 | 01 | 1 | CICD-02 | lint | `golangci-lint run ./...` | ❌ W0 | ⬜ pending |
| 03-01-02 | 01 | 1 | CICD-02 | unit | `go vet ./...` | ✅ | ⬜ pending |
| 03-02-01 | 02 | 1 | CICD-01 | integration | GitHub Actions run | ❌ W0 | ⬜ pending |
| 03-02-02 | 02 | 1 | CICD-03 | build | `CGO_ENABLED=0 go build ./cmd/monitor/` | ✅ | ⬜ pending |
| 03-02-03 | 02 | 1 | CICD-04 | vet | `go vet ./...` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `.golangci.yml` — golangci-lint v2 configuration with required linters
- [ ] `.github/workflows/ci.yml` — GitHub Actions workflow file

*Existing infrastructure covers build, vet, and test commands.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| GitHub Actions workflow triggers on push/PR | CICD-01 | Requires actual GitHub push | Push branch, verify workflow runs in Actions tab |
| Branch protection rules on main | CICD-01 | GitHub settings, not code | Configure via GitHub repo Settings > Branches |
| Concurrency cancellation | CICD-01 | Requires multiple rapid pushes | Push twice quickly, verify first run cancelled |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending

# Phase 3: CI Pipeline - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-28
**Phase:** 03-ci-pipeline
**Areas discussed:** CI trigger scope, Linter strictness, Lint compliance, Workflow extras

---

## CI Trigger Scope

| Option | Description | Selected |
|--------|-------------|----------|
| Push + PRs | Runs on push to develop/main AND on pull requests to those branches | ✓ |
| Push only | Runs on push to develop/main only. No PR status checks. | |
| PRs only | Runs only on pull requests. No push validation. | |

**User's choice:** Push + PRs (Recommended)
**Notes:** Standard for portfolio projects — shows green checks on PRs.

### Branch Protection Follow-up

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — require passing CI | Enforce status checks on main. PRs can't merge unless CI is green. | ✓ |
| No — just CI checks | CI runs and reports status, but merging is not blocked. | |
| You decide | Claude picks based on project context. | |

**User's choice:** Yes — require passing CI
**Notes:** Professional standard.

---

## Linter Strictness

| Option | Description | Selected |
|--------|-------------|----------|
| Required linters only | The 5 linters from requirements (contextcheck, bodyclose, sqlclosecheck, errorlint, sloglint) plus Go defaults. | ✓ |
| Strict — enable more linters | Add gosec, gocritic, revive, ineffassign, unused, etc. Higher fix burden. | |
| Enable-all minus noisy | Maximum coverage but highest fix burden. | |

**User's choice:** Required linters only (Recommended)
**Notes:** Clean, focused, no false positives.

---

## Lint Compliance

| Option | Description | Selected |
|--------|-------------|----------|
| Fix all in this phase | Clean up any warnings so CI passes green from the start. | ✓ |
| Baseline — ignore pre-existing | Use --new-from-rev to only lint new code. | |
| nolint comments where needed | Annotate false positives with //nolint:lintername. | |

**User's choice:** Fix all in this phase (Recommended)
**Notes:** With only 5 linters the fix burden should be small. Clean portfolio from day one.

---

## Workflow Extras

| Option | Description | Selected |
|--------|-------------|----------|
| Go module caching | Cache go mod download between runs. | ✓ |
| Job timeout | Set a timeout so hung jobs don't burn Actions minutes. | ✓ |
| Go version from go.mod | Read Go version from go.mod instead of hardcoding. | ✓ |
| Concurrency limits | Cancel in-progress CI runs when a new push arrives. | ✓ |

**User's choice:** All four options selected.
**Notes:** None.

---

## Claude's Discretion

- Exact timeout duration
- Workflow file name and structure details
- golangci-lint action version selection
- Cache implementation details
- Step ordering within jobs

## Deferred Ideas

None — discussion stayed within phase scope.

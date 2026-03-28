# Phase 3: CI Pipeline - Context

**Gathered:** 2026-03-28
**Status:** Ready for planning

<domain>
## Phase Boundary

GitHub Actions CI pipeline that automatically builds, vets, tests, and lints every push to develop/main and every pull request. golangci-lint v2 with Go-specific linters. All existing code passes clean.

</domain>

<decisions>
## Implementation Decisions

### CI trigger scope
- **D-01:** Workflow runs on push to `develop` and `main`, AND on pull requests targeting those branches
- **D-02:** Branch protection rules on `main` requiring CI to pass before merging

### Linter configuration
- **D-03:** golangci-lint v2 with exactly the 5 required linters: contextcheck, bodyclose, sqlclosecheck, errorlint, sloglint — no extras
- **D-04:** All existing lint warnings must be fixed in this phase — no baseline exclusions, no nolint comments for pre-existing code. CI passes green from the first commit.

### Workflow structure
- **D-05:** Two parallel jobs: `lint` (golangci-lint) and `build-and-test` (build + vet + test) — per CICD-01
- **D-06:** Go module caching enabled between runs
- **D-07:** Job timeout set (e.g., 10 minutes) to prevent hung jobs burning Actions minutes
- **D-08:** Go version read from `go.mod` (not hardcoded) — stays in sync automatically
- **D-09:** Concurrency limits: cancel in-progress CI runs when a new push arrives to the same branch

### Claude's Discretion
- Exact timeout duration (10 minutes is a reasonable default)
- Workflow file name and structure details
- golangci-lint action version selection
- Whether to use `actions/setup-go` or `actions/cache` separately vs combined caching
- Order of steps within each job
- Any golangci-lint settings beyond linter selection (e.g., timeouts, severity)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements and constraints
- `.planning/REQUIREMENTS.md` — CICD-01 through CICD-04 acceptance criteria
- `.planning/ROADMAP.md` §Phase 3 — Success criteria (4 items)
- `CLAUDE.md` — Build command (`CGO_ENABLED=0 go build ./cmd/monitor/`), vet command (`go vet ./...`), test command (`go test ./...`)

### Build reference
- `Dockerfile` — Existing multi-stage build with `golang:1.26.1-alpine`, shows production build flags
- `go.mod` — Go version (1.26.1), module path, dependency list

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- None — no existing CI configuration, `.github/` directory, or `.golangci.yml`

### Established Patterns
- Build: `CGO_ENABLED=0 go build -ldflags="-s -w" -o /monitor ./cmd/monitor/` (from Dockerfile)
- Test: `go test ./...` (5 packages: apperror, bot, config, monitor, storage)
- Vet: `go vet ./...` (required before every commit per CLAUDE.md)
- Go 1.26.1 (from go.mod and Dockerfile)

### Integration Points
- `.github/workflows/` — new directory, workflow YAML file(s)
- `.golangci.yml` — new file at repo root for linter configuration
- GitHub repository settings — branch protection rules (manual or documented)

</code_context>

<specifics>
## Specific Ideas

No specific requirements — standard GitHub Actions patterns apply. User selected all recommended options.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 03-ci-pipeline*
*Context gathered: 2026-03-28*

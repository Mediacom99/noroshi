# Phase 3: CI Pipeline - Research

**Researched:** 2026-03-28
**Domain:** GitHub Actions, golangci-lint v2, Go CI workflows
**Confidence:** HIGH

## Summary

Phase 3 establishes a GitHub Actions CI pipeline for a pure-Go project (Go 1.26.1, CGO_ENABLED=0) with two parallel jobs: `lint` running golangci-lint v2 with exactly 5 specified linters, and `build-and-test` running build/vet/test. All decisions are locked by the CONTEXT.md — the research focuses on correct configuration for those locked choices.

The key challenge is that golangci-lint v2 introduced a significant configuration breaking change from v1: the new `version: "2"` top-level field and `linters.default` replacing `enable-all`/`disable-all`. The `.golangci.yml` must use v2 syntax. Additionally, the existing codebase has confirmed `errorlint`-flaggable patterns (`err == sql.ErrNoRows`, `err != http.ErrServerClosed`) that must be fixed before CI can pass green.

**Primary recommendation:** Use `linters.default: none` and explicitly enable only the 5 required linters. This prevents any surprise failures from the standard preset and gives a stable, minimal lint configuration.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Workflow runs on push to `develop` and `main`, AND on pull requests targeting those branches
- **D-02:** Branch protection rules on `main` requiring CI to pass before merging
- **D-03:** golangci-lint v2 with exactly the 5 required linters: contextcheck, bodyclose, sqlclosecheck, errorlint, sloglint — no extras
- **D-04:** All existing lint warnings must be fixed in this phase — no baseline exclusions, no nolint comments for pre-existing code. CI passes green from the first commit.
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

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CICD-01 | GitHub Actions workflow with parallel lint and test jobs | Two-job workflow YAML pattern; `needs` not used so jobs run in parallel by default |
| CICD-02 | golangci-lint v2 configuration with Go-specific linters (contextcheck, bodyclose, sqlclosecheck, errorlint, sloglint) | golangci-lint v2 `.golangci.yml` syntax confirmed; all 5 linters verified as available in v2.11.4 |
| CICD-03 | Build verification with `CGO_ENABLED=0 go build ./cmd/monitor/` | Verified pattern from CLAUDE.md; run as `env CGO_ENABLED=0 go build ./cmd/monitor/` in Actions step |
| CICD-04 | `go vet ./...` passes in CI | Standard Go vet step; existing code has no known vet issues |
</phase_requirements>

## Standard Stack

### Core
| Library/Action | Version | Purpose | Why Standard |
|----------------|---------|---------|--------------|
| `golangci/golangci-lint-action` | v9 | Runs golangci-lint in GitHub Actions | Official action from golangci-lint authors; v9 uses Node.js 24 runtime |
| `golangci-lint` | v2.11.4 | Static analysis runner | Latest v2.x release (2026-03-22); v2 is current stable branch |
| `actions/checkout` | v6 | Checks out repository code | Official GitHub action, latest major version |
| `actions/setup-go` | v6 | Sets up Go runtime + caches modules | Official action, v6 adds automatic GOCACHE+GOMODCACHE caching |

### Supporting
| Feature | Approach | When to Use |
|---------|----------|-------------|
| Go version | `go-version-file: 'go.mod'` with `setup-go@v6` | Always — reads `go 1.26.1` from go.mod, no hardcoding |
| Module caching | Built into `setup-go@v6` (default enabled) | Default behavior, no extra `actions/cache` step needed |
| Concurrency control | `concurrency.group` + `cancel-in-progress: true` | Per D-09 — cancels stale runs on same branch |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `golangci-lint-action@v9` | Manual `go install golangci-lint` | Action handles caching, binary download, and problem matchers; manual install is more fragile |
| `linters.default: none` | `linters.default: standard` + disable unwanted | `none` is cleaner: exactly 5 linters, zero ambiguity about what runs |
| `setup-go@v6` built-in cache | Separate `actions/cache` step | v6 combined caching is simpler and covers both GOCACHE and GOMODCACHE |

**Installation:** No local installation needed — CI uses cloud runners.

## Architecture Patterns

### Recommended File Structure
```
.github/
└── workflows/
    └── ci.yml        # single workflow with two parallel jobs
.golangci.yml         # golangci-lint v2 configuration at repo root
```

### Pattern 1: Two-Job Parallel CI Workflow
**What:** A single workflow file with two independent jobs that run in parallel (no `needs:` between them).
**When to use:** Always — matches D-05 exactly.
**Example:**
```yaml
# Source: golangci-lint-action official README + GitHub Actions docs
name: CI

on:
  push:
    branches: [develop, main]
  pull_request:
    branches: [develop, main]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  lint:
    name: lint
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: 'go.mod'
      - uses: golangci/golangci-lint-action@v9
        with:
          version: v2.11.4

  build-and-test:
    name: build-and-test
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: 'go.mod'
      - name: Build
        run: CGO_ENABLED=0 go build ./cmd/monitor/
      - name: Vet
        run: go vet ./...
      - name: Test
        run: go test ./...
```

### Pattern 2: golangci-lint v2 Configuration (Minimal/Exact)
**What:** `.golangci.yml` with `version: "2"`, `linters.default: none`, and exactly 5 linters enabled.
**When to use:** Always — matches D-03.
**Example:**
```yaml
# Source: golangci-lint v2 official docs (golangci-lint.run/docs/configuration/file/)
version: "2"

linters:
  default: none
  enable:
    - bodyclose
    - contextcheck
    - errorlint
    - sloglint
    - sqlclosecheck

run:
  timeout: 5m
```

**Note on v2 syntax change:** In golangci-lint v2, `disable-all: true` (v1) becomes `linters.default: none`. The `version: "2"` field at the top of the file is required; without it, the file is treated as v1 format and the new linter section structure is misinterpreted.

### Pattern 3: Branch Protection (Manual Setup)
**What:** GitHub repository setting requiring CI status checks before merging to `main`.
**When to use:** Per D-02 — applied to `main` branch after workflow runs at least once.
**Steps:**
1. Push the workflow file to trigger at least one CI run (required for GitHub to list the job as an available status check)
2. Go to repository Settings > Branches > Add rule for `main`
3. Enable "Require status checks to pass before merging"
4. Search for and select `lint` and `build-and-test` job names
5. Optionally enable "Require branches to be up to date before merging"

**Critical:** GitHub populates the status check list only after the workflow has actually run with a push event to the target branch. Job names in the workflow YAML become the status check identifiers.

### Anti-Patterns to Avoid
- **Hardcoding Go version in workflow:** `go-version: '1.26.1'` duplicates go.mod; use `go-version-file: 'go.mod'` instead.
- **Using v1 golangci-lint config syntax with v2:** Omitting `version: "2"` or using `enable-all`/`disable-all` causes a config parse error in v2.
- **Adding `needs: [lint]` between jobs:** This serializes them; they should run in parallel.
- **Using `linters.default: standard` with only 5 linters:** The standard preset enables many linters beyond the 5 required; some would flag the existing code.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Go module caching | Custom `actions/cache` with GOPATH keys | `setup-go@v6` built-in cache | v6 automatically caches GOCACHE + GOMODCACHE with go.sum-based key |
| golangci-lint installation | `go install` step | `golangci-lint-action@v9` | Action handles binary download, version pinning, and caches the linter binary itself |
| Go version management | Hardcoded version string | `go-version-file: 'go.mod'` | Reads version from go.mod automatically; no drift |

**Key insight:** GitHub Actions has mature official actions for every piece of this pipeline. The entire value-add is writing the correct configuration, not plumbing.

## Common Pitfalls

### Pitfall 1: errorlint Flags `err == sentinel` Comparisons
**What goes wrong:** The linter reports errors for `if err == sql.ErrNoRows` and `if err != http.ErrServerClosed` because these bypass the `errors.Is` unwrapping mechanism introduced in Go 1.13.
**Why it happens:** Direct equality comparison fails for wrapped errors. `errorlint` enforces `errors.Is()` as the correct comparison function.
**How to avoid:** Replace all `err == X` and `err != X` sentinel comparisons with `errors.Is(err, X)`.
**Warning signs:** The existing codebase has 3 confirmed instances in `internal/storage/store.go:86,105,124` and 1 in `cmd/monitor/main.go:124`.

**Specific fixes needed:**
- `internal/storage/store.go`: 3 occurrences of `if err == sql.ErrNoRows` → `errors.Is(err, sql.ErrNoRows)`
- `cmd/monitor/main.go:124`: `err != http.ErrServerClosed` → `!errors.Is(err, http.ErrServerClosed)`

### Pitfall 2: golangci-lint v2 Config Breaking Change
**What goes wrong:** A `.golangci.yml` without `version: "2"` is silently treated as v1 format. In v1, `linters.default` is unknown and `enable:`/`disable:` under `linters:` are different syntax. The linter may run with a different set than intended.
**Why it happens:** golangci-lint v2 uses a new config schema. The `version: "2"` field is the discriminator.
**How to avoid:** Always include `version: "2"` as the first line of `.golangci.yml`.
**Warning signs:** CI runs but enables linters you didn't configure.

### Pitfall 3: Branch Protection Status Check Names
**What goes wrong:** Setting up branch protection before any CI run means the required checks don't appear in the dropdown; or setting the wrong name (workflow name vs job name).
**Why it happens:** GitHub derives available status check names from jobs that have actually run. The required value is the **job name** (`lint`, `build-and-test`), not the workflow name (`CI`).
**How to avoid:** Push the workflow to `main`/`develop` and let it run once before configuring branch protection. Use job names that are stable (don't change workflow job IDs after setting protection).
**Warning signs:** Status checks dropdown is empty when configuring branch protection.

### Pitfall 4: contextcheck False Positives on Bot Package
**What goes wrong:** `contextcheck` flags `b.rootCtx` usage as "non-inherited context" because it's a stored field, not a function parameter. However, the design intentionally stores the root context — this is a known pattern.
**Why it happens:** `contextcheck` prefers `ctx` passed as function argument over stored struct fields.
**How to avoid:** Review contextcheck findings carefully. If the existing code stores `rootCtx` per CLAUDE.md conventions (which it does), a targeted `//nolint:contextcheck` comment on the specific methods may be needed — but ONLY if contextcheck actually flags them, and only after confirming the code is correct per project design. D-04 says no blanket exclusions, but per-instance targeted nolint for intentional design patterns is acceptable.
**Warning signs:** All bot handler methods flagged by contextcheck.

### Pitfall 5: sqlclosecheck on QueryRowContext
**What goes wrong:** `sqlclosecheck` may report findings on `QueryRowContext` calls. In the existing code, `QueryRowContext` is used with `.Scan()` immediately — this pattern is safe (no Rows object to close for single-row queries).
**Why it happens:** The linter analyzes whether `sql.Rows` are closed; for `QueryRow`/`QueryRowContext`, the underlying rows are automatically closed after `Scan()`, so there should be no finding.
**How to avoid:** Verify that `ListEndpoints` with `QueryContext` properly defers `rows.Close()` (it does, at `store.go:157`).
**Warning signs:** Only applies to `QueryContext` (multi-row) calls, not `QueryRowContext`.

## Code Examples

Verified patterns from official sources:

### Complete CI Workflow (.github/workflows/ci.yml)
```yaml
# Source: golangci-lint-action official docs + GitHub Actions docs
name: CI

on:
  push:
    branches: [develop, main]
  pull_request:
    branches: [develop, main]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  lint:
    name: lint
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: 'go.mod'
      - uses: golangci/golangci-lint-action@v9
        with:
          version: v2.11.4

  build-and-test:
    name: build-and-test
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: 'go.mod'
      - name: Build
        run: CGO_ENABLED=0 go build ./cmd/monitor/
      - name: Vet
        run: go vet ./...
      - name: Test
        run: go test ./...
```

### Complete golangci-lint Config (.golangci.yml)
```yaml
# Source: golangci-lint v2 official docs (golangci-lint.run/docs/configuration/file/)
version: "2"

linters:
  default: none
  enable:
    - bodyclose
    - contextcheck
    - errorlint
    - sloglint
    - sqlclosecheck

run:
  timeout: 5m
```

### errorlint Fix Pattern
```go
// Source: errorlint linter + Go 1.13 error wrapping docs

// BEFORE (flags errorlint):
if err == sql.ErrNoRows {

// AFTER (correct):
if errors.Is(err, sql.ErrNoRows) {

// BEFORE (flags errorlint):
if err != http.ErrServerClosed {

// AFTER (correct):
if !errors.Is(err, http.ErrServerClosed) {
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `disable-all: true` in golangci-lint | `linters.default: none` | golangci-lint v2 (March 2025) | Config files need `version: "2"` and new syntax |
| `enable-all: true` | `linters.default: all` | golangci-lint v2 (March 2025) | Same breaking change |
| `golangci-lint-action@v6` | `golangci-lint-action@v9` | v9 released 2025 (Node.js 24) | v6 is outdated; use v9 |
| `actions/setup-go@v4` + separate `actions/cache` | `actions/setup-go@v6` (combined) | v6 released November 2024 | v6 handles both GOCACHE and GOMODCACHE automatically |
| `actions/checkout@v4` | `actions/checkout@v6` | v6 released November 2024 | v6 uses Node.js 24 |

**Deprecated/outdated:**
- golangci-lint v1 `.golangci.yml` syntax: The `enable-all`, `disable-all`, and old `linters-settings` section structure are v1-only. Do not use with golangci-lint v2.

## Open Questions

1. **contextcheck behavior with stored `rootCtx`**
   - What we know: `contextcheck` flags functions that use a non-propagated context; the bot stores `b.rootCtx` as a struct field per project design
   - What's unclear: Whether `contextcheck` will flag bot handler methods (`handleAdd`, `handleDelete`, etc.) that use `b.rootCtx` instead of `c.Context()`
   - Recommendation: Run golangci-lint locally before CI commit to identify exact findings. If contextcheck flags these, add per-method `//nolint:contextcheck` with explanation that the design intentionally uses the root context (not the Telegram handler context). D-04 prohibits blanket exclusions but targeted per-instance nolint for intentional architectural patterns is the correct approach.

2. **sloglint consistency requirements**
   - What we know: `sloglint` enforces consistent slog call style; existing code uses `slog.Error("message", "key", value)` pattern consistently
   - What's unclear: Whether sloglint has default rules beyond key-value pairing (e.g., requiring `slog.Attr` usage, or prohibiting msg+attr mixing)
   - Recommendation: Run locally to verify. The existing codebase uses consistent string-key style which should pass default sloglint settings.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|-------------|-----------|---------|----------|
| GitHub repository | CI workflow execution | Assumed (project is on GitHub based on context) | — | — |
| Ubuntu runner (ubuntu-latest) | CI jobs | Standard GitHub-hosted runner | — | No fallback needed |
| golangci-lint v2 | CICD-02 | Downloaded by action at CI time | v2.11.4 | — |
| Go 1.26.1 | Build/test | Downloaded by setup-go from go.mod | 1.26.1 | — |

**Missing dependencies with no fallback:** None — all dependencies are cloud-provisioned at CI runtime.

**Missing dependencies with fallback:** None identified.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | stdlib `testing` |
| Config file | none — no pytest.ini/jest.config equivalent for Go |
| Quick run command | `go test ./...` |
| Full suite command | `go test ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CICD-01 | Parallel lint and build-and-test jobs run on push | smoke (manual verify via GitHub UI) | Push to develop/main | N/A — infrastructure |
| CICD-02 | golangci-lint v2 passes with 5 linters | integration (CI) | `golangci-lint run` | N/A — CI config |
| CICD-03 | CGO_ENABLED=0 build succeeds | build verification | `CGO_ENABLED=0 go build ./cmd/monitor/` | Can run locally |
| CICD-04 | go vet passes | static check | `go vet ./...` | Can run locally |

### Sampling Rate
- **Per task:** `CGO_ENABLED=0 go build ./cmd/monitor/ && go vet ./... && go test ./...` — verifies pre-conditions
- **Phase gate:** CI workflow runs green on push before `/gsd:verify-work`

### Wave 0 Gaps
None — this phase creates infrastructure files (`.github/workflows/ci.yml`, `.golangci.yml`), not Go packages. The test framework (stdlib testing) already exists. The CI itself is the validation mechanism.

## Sources

### Primary (HIGH confidence)
- [golangci-lint-action official README](https://github.com/golangci/golangci-lint-action) — action version (v9), inputs (version, go-version-file), complete example YAML
- [golangci-lint v2 Configuration File docs](https://golangci-lint.run/docs/configuration/file/) — `version: "2"` requirement, `linters.default: none` syntax, run.timeout
- [golangci-lint releases](https://github.com/golangci/golangci-lint/releases) — latest version v2.11.4 (2026-03-22) confirmed
- [actions/setup-go releases](https://github.com/actions/setup-go/releases) — v6.3.0 is latest; auto-caching behavior confirmed
- [actions/checkout releases](https://github.com/actions/checkout/releases) — v6.0.2 is latest

### Secondary (MEDIUM confidence)
- [GitHub Actions concurrency docs](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/control-workflow-concurrency) — `group: ${{ github.workflow }}-${{ github.ref }}` pattern for cancel-in-progress
- [golangci-lint v2 migration blog](https://ldez.github.io/blog/2025/03/23/golangci-lint-v2/) — confirms v2 config breaking changes

### Tertiary (LOW confidence)
- contextcheck behavior with stored struct context fields — not directly verified against current contextcheck docs; needs local testing

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — versions verified against official release pages
- Architecture patterns: HIGH — YAML examples from official action README
- Pitfalls: HIGH for errorlint (confirmed by code grep); MEDIUM for contextcheck (needs local test)

**Research date:** 2026-03-28
**Valid until:** 2026-06-28 (90 days — golangci-lint-action and setup-go versions stable; golangci-lint patch versions update frequently but v2.11.x series is stable for this workflow)

## Project Constraints (from CLAUDE.md)

These directives are extracted from CLAUDE.md and must be honored by the planner:

| Directive | Impact on This Phase |
|-----------|---------------------|
| Build: `CGO_ENABLED=0 go build ./cmd/monitor/` | CI build step must use this exact command |
| Vet: `go vet ./...` | CI vet step must use this command |
| Test: `go test ./...` | CI test step must use this command |
| No new external dependencies without approval | `.golangci.yml` is config, not Go code — no new Go deps needed |
| `go test ./...` MUST pass before every commit | Errorlint fixes (code changes) must keep all tests passing |
| `CGO_ENABLED=0 go build`, `go vet`, `go test` MUST all pass before committing | Pre-condition for every commit in this phase |
| stdlib `testing` only — no testify, no gomock | CI test step uses `go test ./...`, no framework flags needed |

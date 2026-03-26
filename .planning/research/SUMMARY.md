# Project Research Summary

**Project:** Noroshi (Uptime Monitor)
**Domain:** Self-hosted uptime monitoring with Telegram bot interface
**Researched:** 2026-03-26
**Confidence:** HIGH

## Executive Summary

Noroshi has a working, deployed core: add/delete/list endpoints, periodic HTTP checks, failure/recovery notifications, inline keyboard UI, SQLite persistence, and Docker/Coolify deployment. The codebase is architecturally sound in the large — the layered decomposition into `cmd/monitor/`, `internal/config/`, `internal/apperror/`, `internal/storage/`, `internal/monitor/`, and `internal/bot/` is correct and maintainable. However, it carries specific, concrete technical debt that must be resolved before adding features: one interface-at-point-of-use violation (`Scheduler` holds `*HTTPChecker` instead of a `Checker` interface), a dead-code pipeline (`FormatFailureWithCode` defined, tested, but never called), a mock error type that diverges from `apperror` sentinels, and bot handlers/callbacks that are entirely untested. Each of these items independently blocks either testability or portfolio credibility.

The recommended approach is a strict dependency-driven sequence. Interface hygiene must come first because it unblocks deterministic scheduler tests, which in turn makes CI non-flaky. CI must be running and green before README badges are added, because a red badge causes more damage than no badge. Bot handler tests are the single largest coverage gap and the biggest testing credibility signal for a Go showcase project — they require the mock `tele.Context` infrastructure to be built once and shared. Feature additions (status code pipeline, `/status` command, name validation, logging consistency) can be threaded into the sequence where their dependencies allow, but none of them should be started until interfaces are clean. Documentation — DESIGN.md update and README rewrite — must come last, because they reference everything above.

The primary risks are: (1) cascading interface changes that touch mocks, callers, and implementations simultaneously (mitigate with one-commit-per-concern discipline); (2) the `tele.Context` mock becoming a 400-line maintenance burden (mitigate with the struct-embedding "panic on unimplemented" approach); and (3) CI flakiness from `go test -race` combined with real HTTP in scheduler tests (mitigate by extracting the `Checker` interface before adding CI, and deferring `-race` until tests are fully deterministic). None of these risks is novel; all have known mitigations documented in the research.

## Key Findings

### Recommended Stack

The core application stack is locked (Go, SQLite via modernc.org/sqlite, gocron, retryablehttp, telebot v4, goose) and not under research. This research covered the showcase tooling layer. For CI, GitHub Actions is the clear choice with two parallel jobs: `build-and-test` (checkout, setup-go with `go-version-file: go.mod`, `go vet`, `go build`, `go test -coverprofile`) and `lint` (golangci-lint-action v9). For linting, golangci-lint v2 with the standard preset plus project-specific additions (`contextcheck`, `bodyclose`, `noctx`, `sqlclosecheck`, `errorlint`, `sloglint`) enforces the rules already stated in CLAUDE.md. Architecture diagrams belong in README and DESIGN.md as Mermaid blocks — GitHub renders them natively, they are text-diffable, and they require no build step.

**Core showcase tooling:**
- `actions/checkout@v6` + `actions/setup-go@v6`: CI checkout and Go setup — current stable, `go-version-file: go.mod` avoids hardcoding
- `golangci-lint v2.11` + `golangci/golangci-lint-action@v9`: meta-linter with 90+ linters — v2 is the only supported line, v1 is EOL
- `CGO_ENABLED: 0` at job level: ensures CI matches Dockerfile build and satisfies CLAUDE.md constraint
- Mermaid (GitHub-native): architecture diagrams in README — no tooling, no build, diffable
- Manual `mockContext` with embedded `tele.Context`: bot handler testing without external deps, using struct-embedding "panic on unimplemented" base

**Critical version note:** golangci-lint must be pinned to v2.11.x (supports Go 1.26). Unpinned installs may default to a version built against an older Go and fail with a confusing toolchain mismatch error.

### Expected Features

Noroshi's current feature set covers the minimum viable monitor but misses several items that every comparable tool (Uptime Kuma, Gatus, UptimeRobot) includes. The most damaging gap is the HTTP status code pipeline — `FormatFailureWithCode` exists but is never called, meaning failure notifications currently show no status code even though the infrastructure for it is partially built.

**Must have (table stakes — missing these signals "weekend hack"):**
- HTTP status code in failure notifications — partially built, needs wiring; every competitor shows this
- On-demand `/status` command — designed in DESIGN.md, never implemented; users expect to trigger checks on demand
- Name validation (alphanumeric, hyphens, underscores, max 50 chars) — already called out in CONCERNS.md; currently absent
- Expected status codes configurable per endpoint — hardcoded `!= 200` means all 204/301 endpoints false-alarm

**Should have (differentiators — impress portfolio reviewers):**
- Response time tracking + `check_log` table — the foundation that unlocks uptime percentage, sparklines, and digests; highest ROI single feature
- Uptime percentage (24h/7d) — the most-expected metric in any monitor; nothing signals "real tool" more than showing 99.8% uptime
- Pause/resume monitoring — operational necessity for deployments and maintenance
- SSL certificate expiry monitoring — pure stdlib (`crypto/tls`), no new deps, impressive capability

**Defer to v2+:**
- Response time sparklines, daily/weekly digest, keyword/body checks, incident timeline, export/backup — all valid differentiators but none are required for this milestone's portfolio goal

**Explicit anti-features (do not build):**
- Web dashboard — contradicts the "Telegram-only" identity that makes Noroshi distinct
- Multi-user RBAC — overkill; a Telegram group chat covers the use case
- Non-HTTP monitor types — depth over breadth; do HTTP well
- Additional notification channels — Telegram is the product

The most impactful feature dependency chain: fix the status code pipeline first (lowest effort, highest visibility), then build the `check_log` table (unlocks all historical metrics), then implement `/status` command and pause/resume (operational completeness).

### Architecture Approach

The current architecture requires five targeted changes: (1) extract a `Checker` interface in `scheduler.go` so the concrete `*HTTPChecker` dependency is inverted; (2) fix mock error types in `scheduler_test.go` to use `apperror.Wrap(apperror.ErrNotFound, ...)` instead of a local `notFoundError{}`; (3) add bot handler and callback tests via a shared `mock_test.go` with the function-field mock pattern for `tele.Context`; (4) add component-scoped loggers via `slog.With("component", ...)` injected via constructors; (5) establish the CI pipeline. No changes to data flow, startup sequence, or component boundaries are needed — the architecture is correct, only these specific gaps need filling.

**Major components (current, no structural changes needed):**
1. `cmd/monitor/` — wiring, lifecycle, signal handling; no changes
2. `internal/storage/` — SQLite store + goose migrations; add `check_log` table migration for v2 features
3. `internal/monitor/` — Scheduler (gocron) + HTTPChecker (retryablehttp); extract Checker interface, inject logger
4. `internal/bot/` — Telegram handlers, callbacks, formatting, validation; add handler/callback tests, inject logger
5. `.github/workflows/ci.yml` — does not exist yet; add build-and-test + lint parallel jobs

**Key patterns to follow:**
- Interface-at-point-of-use (not in implementing package) — already established everywhere except `Scheduler.checker`
- Constructor accepts all dependencies (no global state, no `Set*` except the documented `SetScheduler` exception)
- Table-driven tests with `t.Run()` named subtests for every multi-scenario test function
- Test behavior, not output — assert that `Send` was called and `Store.AddEndpoint` received correct args; format output is tested separately in `format_test.go`

### Critical Pitfalls

1. **Cascading interface changes break everything at once** — When changing `Notifier` signature to pass `statusCode`, the interface, the scheduler caller, the bot implementer, and all mocks change simultaneously. Mitigate with strict commit sequence: interface change first (compile-only), then behavior wiring, then formatting update. Never touch interface and logic in the same commit.

2. **tele.Context mock becomes the project** — With 48 methods, a naive hand-rolled mock is 400+ lines of boilerplate. Use the struct-embedding approach: define a base struct implementing all 48 methods with `panic("not implemented")`, embed it in test-specific structs that override only the 5-8 methods each test needs. Each test mock stays under 30 lines.

3. **Scheduler tests coupled to real HTTP, causing CI flakiness** — Current `scheduler_test.go` uses real `httptest.Server` instances because `Scheduler` holds `*HTTPChecker` concretely. Extract the `Checker` interface before adding CI, then rewrite scheduler tests to use a `mockChecker`. Real HTTP tests belong in `checker_test.go` only.

4. **Mock errors diverge from apperror sentinels** — `scheduler_test.go` currently uses a local `notFoundError{}` that does not wrap `apperror.ErrNotFound`. This means `errors.Is(err, apperror.ErrNotFound)` returns false in the mock path but true in production, creating a hidden test coverage gap for all `findEndpoint` branches. Fix this before writing any new bot handler mocks.

5. **README badges before CI is green** — A red badge or "no status" placeholder is worse than no badge. Strict ordering: CI pipeline must be running and green on the default branch before any badge markdown is added to README.

## Implications for Roadmap

Based on the dependency chain discovered across all four research dimensions:

### Phase 1: Tech Debt Cleanup

**Rationale:** All other phases depend on clean, interface-correct code. Bot handler tests cannot be written correctly until mock error types use `apperror` sentinels. CI will be flaky unless the `Checker` interface is extracted first. The dead `FormatFailureWithCode` function is visible to every portfolio reviewer. These are cheap, high-leverage changes that compound positively.

**Delivers:** A codebase where every interface follows the point-of-use convention, every mock uses canonical error types, and no exported symbols are dead code.

**Work items:**
- Extract `Checker` interface in `scheduler.go` (one-line type change + interface definition)
- Fix `scheduler_test.go` mock to use `apperror.Wrap(apperror.ErrNotFound, ...)` instead of `notFoundError{}`
- Wire up the status code pipeline: update `Notifier` interface, update `checkAndNotify` caller, update `TelegramNotifier` implementer — in three separate commits (interface first, then wiring, then formatting)
- Persist or remove the unused `statusCode` parameter in store methods (add migration 003 if persisting, or remove from all signatures in one compiler-verified refactor)
- Add name validation (`ValidateName()` in `validate.go`) — independent, zero-risk

**Avoids:** Pitfall 1 (cascading interface changes), Pitfall 7 (mock error divergence), Pitfall 15 (dead exports)

**Research flag:** No deeper research needed. Changes are clearly scoped in ARCHITECTURE.md and PITFALLS.md.

### Phase 2: Bot Handler Tests

**Rationale:** Bot handlers and callbacks are the largest untested surface in the application and the most visible signal to a Go developer evaluating the codebase. This phase requires Phase 1 to be complete (interfaces clean, error types canonical) so the mocks are correct from the start. The `tele.Context` mock infrastructure is built once in `mock_test.go` and reused across all handler and callback tests.

**Delivers:** Full test coverage for `handlers.go` and `callbacks.go`, a shared `mock_test.go` with `mockContext`, `mockStore`, and `mockScheduler`, and `handlers_test.go` + `callbacks_test.go` with table-driven tests for every handler/callback including error paths.

**Work items:**
- Create `internal/bot/mock_test.go` with function-field `mockContext` (struct-embedding base with panic-on-unimplemented), `mockStore`, and `mockScheduler`
- Write `handlers_test.go`: `handleAdd`, `handleDelete`, `handleList`, `handleInterval`, `handleHelp`, `findEndpoint` — all scenarios including error paths
- Write `callbacks_test.go`: `handleDetailCallback`, `handleDeleteCallback`, `handleConfirmDeleteCallback`, `handleSetIntervalCallback`, `handleBackCallback`, `handleRefreshCallback`

**Avoids:** Pitfall 2 (mock boilerplate explosion), Pitfall 11 (snapshot testing of HTML output)

**Research flag:** No deeper research needed. Exact mock pattern is specified in STACK.md and ARCHITECTURE.md.

### Phase 3: CI Pipeline

**Rationale:** CI must come after tests are deterministic (Phase 1 + 2). Adding CI to flaky tests creates a broken-badge problem that is worse than no CI. With the Checker interface extracted (Phase 1) and bot handler tests written (Phase 2), `go test ./...` is fully deterministic and CI will be green from the first run.

**Delivers:** `.github/workflows/ci.yml` with parallel `build-and-test` and `lint` jobs, `.golangci.yml` with project-appropriate linter set, and green CI on the `develop` and `main` branches.

**Work items:**
- Create `.github/workflows/ci.yml`: two parallel jobs (`build-and-test` and `lint`), `CGO_ENABLED: 0` at job level, `go-version-file: go.mod`, golangci-lint-action@v9 pinned to v2.11.x
- Create `.golangci.yml`: standard preset + `contextcheck`, `bodyclose`, `noctx`, `sqlclosecheck`, `errorlint`, `sloglint` (attr-only), `gosec`, `gocritic`; exclude `_test.go` from `gosec`/`noctx`/`goconst`
- Fix any lint issues surfaced by golangci-lint on the existing codebase (especially `sloglint` for inconsistent slog attribute usage)
- Do NOT add `-race` to CI yet (add only after verifying no SQLite timing flakiness)

**Avoids:** Pitfall 3 (HTTP-coupled scheduler tests causing CI flakiness), Pitfall 5 (badges before CI is green), Pitfall 8 (golangci-lint version mismatch with Go 1.26), Pitfall 9 (-race with SQLite), Pitfall 14 (missing CGO_ENABLED=0)

**Research flag:** No deeper research needed. Exact workflow skeleton is in STACK.md and ARCHITECTURE.md.

### Phase 4: Feature Additions

**Rationale:** With clean interfaces, full handler tests, and a running CI pipeline, new features can be added with immediate validation. Each feature in this phase is independent or has a clear dependency on a prior feature. The ordering within this phase follows feature impact: status code pipeline is already half-done; `/status` command has the highest user-facing value per implementation effort; logging and name validation are low-risk polish.

**Delivers:** The remaining active items from PROJECT.md that are feature-shaped: live status checks, consistent logging, name validation improvements, and the immediate-check-on-add behavior.

**Work items (in dependency order):**
- Implement `/status` command: iterate endpoints, call `checker.Check()`, reply with live results per endpoint; add "Check Now" inline keyboard button to detail view
- Trigger immediate health check when user adds a new endpoint (leverages the same checker.Check() call, shows result inline)
- Improve structured logging: add `slog.With("component", ...)` loggers injected via constructors in `Scheduler` and `Bot`; enforce typed attribute helpers (`slog.String(...)`, `slog.Int64(...)`) throughout; fix any remaining raw key-value slog calls surfaced by `sloglint` in CI

**Avoids:** Pitfall 4 (stale DESIGN.md accumulating more drift as features are added without doc updates — update DESIGN.md in the same commit as each interface change)

**Research flag:** No deeper research needed for `/status` or logging. If response time tracking or `check_log` table is added here (pulled forward from v2), those need a migration plan — but they are currently out of scope for this milestone.

### Phase 5: Documentation and README

**Rationale:** README must come last because it references CI badges (needs Phase 3), architecture (needs Phase 1 interface fix reflected), features (needs Phase 4 complete), and commands (needs everything working). DESIGN.md must be updated before README because README may reference DESIGN.md. Writing README first and then updating it repeatedly as code changes is wasted effort.

**Delivers:** A professional README with CI badge, Go Report Card badge, Go Reference badge, Mermaid architecture diagram, feature list, configuration table, command reference, quick start, and deployment docs. Updated DESIGN.md reflecting current interfaces and file structure. Cleaned-up TODO.md (deleted or consolidated into PROJECT.md).

**Work items:**
- Update DESIGN.md: correct Store interface signatures, add `callbacks.go` and `validate.go` to file map, update message format examples, reflect current `AddEndpoint` signature with name parameter, note `/status` command implementation
- Delete or consolidate TODO.md — PROJECT.md is the source of truth; stale parallel tracking is a portfolio negative
- Write README.md: badges section (CI, Go Report Card, Go Reference, License), one-paragraph description, features list, quick start (`docker run`), configuration table (env vars), commands table, architecture Mermaid diagram, development section (build/test/lint commands), deployment section (Docker, Docker Compose, Coolify)
- Verify all badges resolve correctly and CI is green on `main` before finalizing

**Avoids:** Pitfall 4 (stale DESIGN.md), Pitfall 5 (badges before green CI), Pitfall 12 (TODO.md divergence), Pitfall 13 (.env.example falling out of date)

**Research flag:** No deeper research needed. README structure and badge URLs are specified in STACK.md and ARCHITECTURE.md.

### Phase Ordering Rationale

- **Interfaces before tests:** Bot handler mocks must use canonical error types and correct interface signatures. Writing tests against broken interfaces means rewriting tests after fixing interfaces.
- **Tests before CI:** CI running against flaky or incomplete tests creates a broken-badge problem. Every test must be deterministic before CI is enabled.
- **CI before badges:** A red CI badge on a portfolio project is worse than no badge. The badge goes in the final commit of Phase 5, not Phase 3.
- **Features after CI:** New features need CI validation to catch regressions. Adding features without CI means regressions are only caught locally.
- **Docs last:** DESIGN.md and README are accurate only when they reference completed, tested, CI-validated work. Writing them earlier means updating them repeatedly.

### Research Flags

Phases with standard patterns (no deeper research needed during planning):
- **Phase 1 (Tech Debt):** All changes are clearly scoped. Interface extraction pattern is standard Go. Commit sequence is specified in PITFALLS.md.
- **Phase 2 (Bot Tests):** Mock pattern is fully specified in STACK.md and ARCHITECTURE.md. No unknowns.
- **Phase 3 (CI):** Workflow skeleton is in STACK.md. golangci-lint config is in ARCHITECTURE.md. Version numbers are verified and pinned.
- **Phase 4 (Features):** `/status` command and logging are straightforward. No new dependencies. No complex integrations.
- **Phase 5 (Docs):** README structure and badge URLs are fully specified. No research needed.

No phase in this milestone requires a `/gsd:research-phase` call. All necessary research has been completed in this pre-planning cycle.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All version numbers verified from official GitHub release pages. golangci-lint-action v9 docs confirm v2.11 compatibility. |
| Features | HIGH | Based on direct analysis of Uptime Kuma, Gatus, UptimeRobot, and Checkmate source/docs. Table stakes features confirmed across all four references. |
| Architecture | HIGH | Interface analysis based on direct telebot v4 source code reading (`context.go`). Scheduler interface violation verified by reading `internal/monitor/scheduler.go`. Patterns (function-field mock, slog injection) are well-documented Go idioms. |
| Pitfalls | HIGH | Most pitfalls are derived from direct codebase analysis (dead code, mock error types, concrete type dependencies) rather than speculation. A few (golangci-lint/Go version mismatch) are verified from official bug tracker. |

**Overall confidence:** HIGH

### Gaps to Address

- **`-race` flag strategy:** Whether to include `-race` in CI or limit it to the storage package only is a judgment call that depends on how SQLite timing behaves in GitHub Actions runners. Start without `-race`, add it to the storage package first, then expand if it proves stable.
- **`check_log` table timing:** Response time tracking and uptime percentage are out of scope for this milestone per PROJECT.md, but they are the highest-ROI feature additions for v2. If scope expands, add migration 003 for `check_log` at the end of Phase 4 before documentation is finalized.
- **DESIGN.md disposition:** Research recommends keeping DESIGN.md as a living architecture document scoped to patterns and decisions (not function signatures). Whether to restructure it or just update it is a Phase 5 planning decision.

## Sources

### Primary (HIGH confidence)

- [telebot v4 context.go](https://github.com/tucnak/telebot/blob/v4/context.go) — Context interface method count (48), confirmed as interface not struct
- [golangci-lint releases](https://github.com/golangci/golangci-lint/releases) — v2.11.4 is current; v2.11 supports Go 1.26
- [golangci-lint-action](https://github.com/golangci/golangci-lint-action) — v9.0.0 compatibility with golangci-lint v2.x confirmed
- [actions/checkout releases](https://github.com/actions/checkout/releases) — v6.0.2 (Jan 2026) is current stable
- [actions/setup-go releases](https://github.com/actions/setup-go/releases) — v6.3.0 (Feb 2025) is current stable
- [Uptime Kuma GitHub](https://github.com/louislam/uptime-kuma) — feature baseline for table stakes
- [Gatus GitHub](https://github.com/TwiN/gatus) — condition-based monitoring patterns, expected status codes
- [UptimeRobot Pricing](https://uptimerobot.com/pricing/) — free tier feature baseline
- [GitHub Mermaid support](https://github.blog/developer-skills/github/include-diagrams-markdown-files-mermaid/) — native rendering confirmed
- [Go slog official blog post](https://go.dev/blog/slog) — slog injection patterns
- [golangci-lint v2 configuration docs](https://golangci-lint.run/docs/configuration/file/) — YAML structure, presets

### Secondary (MEDIUM confidence)

- [Function-field mock pattern (SafetyCulture)](https://medium.com/safetycultureengineering/flexible-mocking-for-testing-in-go-f952869e34f5) — mock design pattern
- [golangci-lint v2 announcement](https://ldez.github.io/blog/2025/03/23/golangci-lint-v2/) — v2 config structure, preset list
- [Better Stack Monitoring Comparison](https://betterstack.com/community/comparisons/open-source-website-monitoring/) — feature landscape overview
- [telegram-uptime-monitor](https://github.com/BekiChemeda/telegram-uptime-monitor) — comparable Telegram-only monitor
- [Checkmate GitHub](https://github.com/bluewave-labs/Checkmate) — response time visualization patterns
- [Go CI pipeline guide (OneUptime)](https://oneuptime.com/blog/post/2025-12-20-go-ci-pipeline-github-actions/view) — workflow structure

### Tertiary (LOW confidence — used for directional guidance only)

- [Golden golangci-lint config](https://gist.github.com/maratori/47a4d00457a92aa426dbd48a18776322) — community-maintained linter selection (cross-referenced against official docs)
- [Go README best practices](https://dev.to/github/how-to-create-the-perfect-readme-for-your-open-source-project-1k69) — README structure guidance

---
*Research completed: 2026-03-26*
*Ready for roadmap: yes*

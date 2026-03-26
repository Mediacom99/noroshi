# Domain Pitfalls

**Domain:** Go project production hardening and portfolio presentation
**Project:** Noroshi (Telegram-based uptime monitor)
**Researched:** 2026-03-26

## Critical Pitfalls

Mistakes that cause rewrites, broken production behavior, or significant rework.

### Pitfall 1: Cascading Interface Changes Break Everything at Once

**What goes wrong:** Changing the `Notifier` interface to accept `statusCode int` (required to fix FormatFailureWithCode) forces simultaneous changes in `scheduler.go` (caller), `bot.go` (implementer), `scheduler_test.go` (mock), and any future bot handler tests (new mocks). If done in a single "fix everything" commit, a bug in one file is hard to isolate, and a failed test could be in the mock, the implementation, or the caller.

**Why it happens:** The current Notifier interface (`NotifyFailure(ctx, endpoint)`) needs to become `NotifyFailure(ctx, endpoint, statusCode)`. The scheduler mock, the `TelegramNotifier` implementation, and the `checkAndNotify` call site all need updating simultaneously. Developers try to "fix it all at once" because the compiler forces all callers/implementers to match.

**Consequences:** If the mock is subtly wrong (e.g., records `statusCode` but does not update the test endpoint's state), tests pass green but production behavior diverges from test expectations. The scheduler tests currently use a `notFoundError{}` type that does not use `apperror.ErrNotFound` -- this means the mock's error behavior already diverges from real store behavior. Adding more interface changes compounds this drift.

**Prevention:**
1. Change interface signature first, update all mocks/implementations to compile, commit. No behavior change yet.
2. Second commit: wire the status code through the notification pipeline (scheduler passes it, notifier uses it).
3. Third commit: update notification formatting to use `FormatFailureWithCode` when statusCode > 0.
4. Run `go vet ./...` and `go test ./...` after each commit.

**Detection:** If you have a commit that touches more than 3 files across different packages and changes both interfaces and logic simultaneously, you are in the danger zone.

**Phase relevance:** Tech debt cleanup phase. This is the first thing to tackle because bot handler tests (later phase) need the corrected interfaces.

---

### Pitfall 2: Mocking telebot's 48-Method Context Interface Becomes the Project

**What goes wrong:** `tele.Context` has approximately 48 methods. Writing a hand-rolled mock (required by CLAUDE.md -- no gomock allowed) means implementing all 48 methods even though handlers only use 5-8 of them. The mock becomes hundreds of lines of boilerplate that is tedious to maintain. Worse, if telebot v4 adds methods in a future beta release, the mock breaks on dependency update.

**Why it happens:** Go interfaces are satisfied implicitly. A struct must implement every method of an interface to satisfy it. There is no "partial implementation" in Go. Since CLAUDE.md prohibits gomock and testify, auto-generation is off the table.

**Consequences:** Either the mock file becomes a maintenance burden (400+ lines of stubs), or developers skip bot handler tests entirely (the current state). Both outcomes are bad for a showcase project.

**Prevention:**
1. Use struct embedding to inherit a base "panic on unimplemented" implementation. Define a `baseTeleContext` struct that implements all 48 methods with `panic("not implemented")` bodies. Then embed it in test-specific mocks that only override the methods each test needs.
2. Alternative: define a narrow handler-level interface in the bot package (e.g., `type handlerContext interface { Send(...) error; Message() *tele.Message; Chat() *tele.Chat; Callback() *tele.Callback; Respond(...) error; Edit(...) error }`) and have handlers accept that. This decouples handlers from the full tele.Context surface. However, telebot registers handlers with `tele.HandlerFunc` which requires `tele.Context`, so the narrow interface approach requires wrapper adapters.
3. The pragmatic choice: use the embed approach. It is idiomatic Go, requires no external deps, and keeps individual test mocks to 20-30 lines.

**Detection:** If the mock file for tele.Context exceeds 200 lines or takes more than an hour to write, the approach is wrong.

**Phase relevance:** Bot handler testing phase. Must be designed before writing the first test.

---

### Pitfall 3: Scheduler Tests Couple to Real HTTP, Masking Logic Bugs

**What goes wrong:** The current `scheduler_test.go` creates real `httptest.Server` instances and a real `HTTPChecker`. This means scheduler tests exercise HTTP retry logic, TCP connection handling, and response parsing alongside the business logic they are supposed to test. A test failure could be a scheduler logic bug, an HTTP timeout, or a test server timing issue.

**Why it happens:** The scheduler holds `checker *HTTPChecker` (concrete type) instead of a `Checker` interface. Without an interface, you cannot inject a mock checker. The tests compensate by using real HTTP servers.

**Consequences:** Tests are slower than necessary (real HTTP round-trips). More critically, when tests fail intermittently in CI (GitHub Actions runners have variable network/CPU), it is impossible to tell if the failure is a flaky HTTP interaction or an actual regression. This is the single most common cause of "flaky CI" in Go projects with external dependencies.

**Prevention:**
1. Extract a `Checker` interface in `scheduler.go`: `type Checker interface { Check(ctx context.Context, url string) (int, error) }`.
2. Update `Scheduler` to hold `checker Checker` instead of `checker *HTTPChecker`.
3. Create a `mockChecker` in `scheduler_test.go` that returns configurable status codes and errors.
4. Rewrite scheduler tests to use the mock checker. Keep the `httptest.Server` tests in `checker_test.go` where they belong.
5. This must happen before adding CI, or CI will be flaky from day one.

**Detection:** Any test that creates an `httptest.NewServer` in a file that is not testing HTTP behavior directly. The scheduler tests should never need a real server.

**Phase relevance:** Tech debt cleanup phase (interface extraction). Must be done before CI phase.

---

### Pitfall 4: DESIGN.md Becomes a Liability Instead of an Asset

**What goes wrong:** DESIGN.md currently documents features that do not exist (`/status` command), uses outdated signatures (`AddEndpoint` without `name`), omits files that exist (`callbacks.go`, `validate.go`), and shows message formats that differ from reality. A portfolio visitor who reads DESIGN.md and then reads the code will conclude the developer does not maintain their documentation -- the opposite of the intended professional impression.

**Why it happens:** DESIGN.md was written before features were implemented and was never updated as the implementation evolved. This is the natural outcome of "design docs written upfront" in a project without a doc update discipline.

**Consequences:** For a showcase project, stale documentation is worse than no documentation. It signals that the developer writes docs as a one-time checkbox exercise rather than a living practice. Stale DESIGN.md with accurate code is especially damaging because it looks like the developer either does not read their own docs or does not care about accuracy.

**Prevention:**
1. Update DESIGN.md to match current reality before adding any new features. This is a prerequisite, not a nice-to-have.
2. After updating, decide on DESIGN.md's role: is it a living architecture document or a historical design document? If living, it must be updated with every interface change. If historical, rename it to `DESIGN-v1.md` and note "reflects initial design, see code for current state."
3. For a portfolio project, the pragmatic choice: keep DESIGN.md as a living architecture doc but scope it to high-level patterns and decisions only (not exact function signatures that drift). Move detailed API contracts to Go doc comments on the interfaces themselves.
4. Add a CI check or pre-commit reminder (even a comment in the file) to review DESIGN.md when interfaces change.

**Detection:** Run a diff between DESIGN.md's Store interface and the actual `bot.Store`/`monitor.Store` interfaces. Any divergence means the doc is stale.

**Phase relevance:** Documentation phase. Must be done before the README rewrite (README should reference accurate DESIGN.md or replace it).

---

### Pitfall 5: README Badges Pointing to Broken or Missing CI

**What goes wrong:** Adding badges to README (build status, coverage, Go report card) before CI is actually working and green means portfolio visitors see red badges or "no status" placeholders. A red CI badge is worse than no badge -- it tells visitors "this project has broken builds and the developer either does not notice or does not care."

**Why it happens:** Developers add badges as part of the README polish phase, then plan to "fix CI later." The badges go live immediately via the default branch. There is no staging for badges.

**Consequences:** First impression is permanently damaged. GitHub caches badge images, so even after fixing CI, visitors may see stale red badges for hours. Go Report Card badges may show low scores if linting has not been cleaned up yet.

**Prevention:**
1. Strict ordering: CI pipeline must be green before badges are added to README.
2. Set up CI in this order: build -> vet -> test -> lint. Get each step green before adding the next.
3. Only add badges to README in the final commit of the CI phase, after verifying the pipeline passes on the default branch.
4. Test badges on a feature branch first (the badge URLs still work for non-default branches).

**Detection:** If README contains badge markdown but `.github/workflows/` does not exist or the last CI run is red, the badges are premature.

**Phase relevance:** CI phase and README phase. CI must be fully green before README is finalized.

---

## Moderate Pitfalls

### Pitfall 6: `statusCode` Parameter Removal Breaks Store Interface Consumers

**What goes wrong:** The `statusCode` parameter in `UpdateEndpointStatus`, `RecordFailure`, and `RecordRecovery` is unused but exists in the interface. There are two valid fixes: remove it (cleaner) or persist it (more useful). Choosing to persist it means adding a migration, but choosing to remove it means changing the Store interface signature, which breaks all consumers and mocks simultaneously.

**Prevention:**
1. Decide the direction first: the PROJECT.md active list says "persist or remove." For a showcase project, persisting is better (shows last HTTP status in `/list` detail view, demonstrates migration skills).
2. If persisting: add migration 003 first, then update store methods, then update mocks, then update UI. Four small commits, not one big one.
3. If removing: update the interface signature and all consumers/mocks in one commit (this is a safe "find and replace" refactor with compiler verification).

**Phase relevance:** Tech debt cleanup phase, after the Checker interface extraction (Pitfall 3) but before bot handler tests.

---

### Pitfall 7: Mock Error Types Diverge from Production Error Types

**What goes wrong:** The scheduler test's `notFoundError{}` type is a plain struct that does not wrap `apperror.ErrNotFound`. This means `errors.Is(err, apperror.ErrNotFound)` returns `false` in the mock path but `true` in production. If handler code (or future code) branches on `errors.Is(err, apperror.ErrNotFound)`, the test exercises a different code path than production.

**Why it happens:** The mock was written before the `apperror` package conventions were established, or the mock author took a shortcut.

**Consequences:** Tests pass but handler error-handling branches that depend on `errors.Is` are untested or tested against the wrong error type. This is especially dangerous for the bot handler tests, where `findEndpoint` checks `errors.Is(err, apperror.ErrNotFound)`.

**Prevention:**
1. All mock stores must return `apperror.Wrap(apperror.ErrNotFound, ...)` for not-found cases, not custom error types.
2. When writing bot handler test mocks, reuse the `apperror` sentinels.
3. Update the existing scheduler test mock to use `apperror.ErrNotFound` instead of `notFoundError{}`. This is a small change that prevents a class of bugs.

**Detection:** Grep test files for error types that do not use the `apperror` package. Any custom error struct in a `_test.go` file is suspicious.

**Phase relevance:** Tech debt cleanup phase, as a prerequisite before writing new handler tests.

---

### Pitfall 8: golangci-lint Version Incompatibility with Go 1.26

**What goes wrong:** golangci-lint must be built with a Go version >= the project's target Go version. If the GitHub Actions workflow uses `golangci-lint-action@v6` without pinning the golangci-lint version, it may install a version built against an older Go, producing the error: "the Go language version used to build golangci-lint is lower than the targeted Go version."

**Why it happens:** golangci-lint releases lag behind Go releases. Go 1.26 support was added in golangci-lint v2.x (released 2026-02-10). Package managers (homebrew, apt) propagate new versions slowly. The GitHub Action may default to an incompatible version.

**Consequences:** CI fails on the lint step with a confusing version mismatch error. Developers waste time debugging what looks like a lint issue but is actually a toolchain issue.

**Prevention:**
1. Pin the golangci-lint version explicitly in the GitHub Actions workflow: `version: v2.x.x` (the specific version that supports Go 1.26).
2. Use the `install-mode: binary` (default) to get the official pre-built binary, not a `go install` that might use the wrong Go version.
3. Set `go-version: '1.26.1'` explicitly in the `setup-go` step.
4. Add a comment in the workflow file noting the version coupling.

**Detection:** CI lint step fails with "Go language version used to build golangci-lint is lower than targeted Go version."

**Phase relevance:** CI pipeline phase. This is a setup-time issue, not a recurring one, but getting it wrong wastes hours.

---

### Pitfall 9: go test -race with SQLite Produces False Failures

**What goes wrong:** Running `go test -race ./...` with modernc.org/sqlite can produce false data race reports or "database is locked" errors under concurrent test execution. The race detector adds significant overhead, and SQLite's single-writer limitation means concurrent test functions that write to the same in-memory database will contend.

**Why it happens:** `go test` runs test functions within a package sequentially by default, but `-race` adds instrumentation that changes timing. When combined with SQLite's write serialization and the `_busy_timeout`, tests may hit timeout boundaries differently under race detection. Additionally, if CI runs with `-race` and the scheduler tests use real HTTP (Pitfall 3), timing becomes unpredictable.

**Consequences:** CI passes locally but fails intermittently in GitHub Actions. The `-race` flag is often recommended for CI but can be a source of flakiness with SQLite.

**Prevention:**
1. Each test function should use its own in-memory database via `testDB(t)` (already the pattern in `store_test.go` -- good).
2. Do not run `-race` in the CI pipeline initially. Add it later once all tests are deterministic and fast.
3. If adding `-race` later, ensure scheduler tests use mock checkers (Pitfall 3 resolved), so no real HTTP timing is involved.
4. Consider running `-race` only on the `storage` package tests (where it is most valuable for detecting concurrent access bugs) rather than globally.

**Detection:** Tests pass with `go test ./...` but fail intermittently with `go test -race ./...`. The failure message mentions "database is locked" or reports a data race in `modernc.org/sqlite` internals.

**Phase relevance:** CI pipeline phase. Decide the `-race` strategy when designing the workflow.

---

### Pitfall 10: Refactoring Scheduler While It Runs in Production

**What goes wrong:** The scheduler is currently deployed and running via Coolify. Changing the scheduler's internal structure (interface extraction, context handling) while it is actively monitoring endpoints risks introducing a subtle bug that only manifests under specific timing conditions (e.g., a check running during shutdown, a job firing while a reschedule is in progress).

**Why it happens:** Refactoring changes are tested locally but may not reproduce the exact conditions of a long-running scheduler with real endpoints and real network latency.

**Consequences:** A refactoring that works perfectly in tests causes missed notifications or double notifications in production. For an uptime monitor, missing a down notification defeats the entire purpose.

**Prevention:**
1. Keep the production deployment on the current commit during refactoring. Do not deploy refactoring commits until the full phase is complete and tested.
2. After deploying the refactored code, manually trigger a test scenario: add a test endpoint that returns 503, verify the failure notification arrives, make it return 200, verify the recovery notification.
3. The `gocron.WithStartImmediately()` behavior should be explicitly tested after interface changes to ensure no regression in immediate-check behavior.

**Detection:** After deploying, check Telegram for a test notification within 2 minutes. If no notification arrives for a known-down endpoint, something is broken.

**Phase relevance:** All refactoring phases. Deploy discipline applies throughout.

---

## Minor Pitfalls

### Pitfall 11: Handler Tests Become Snapshot Tests of HTML Output

**What goes wrong:** When testing bot handlers, it is tempting to assert the exact string returned by `c.Send(...)`. This couples tests to the exact HTML formatting, emoji choice, and wording. Any cosmetic change to notification messages breaks tests, making refactoring message formats painful.

**Prevention:**
1. Test handler behavior, not output formatting. Assert that `c.Send` was called (not what it was called with). Assert that `store.AddEndpoint` was called with the right arguments.
2. Formatting is already tested in `format_test.go`. Handler tests should verify the handler calls the right format function, not re-test the format output.
3. If you must assert send content, use `strings.Contains` for key substrings, not exact string matching.

**Phase relevance:** Bot handler testing phase.

---

### Pitfall 12: TODO.md, PROJECT.md Active Items, and GitHub Issues Drift Apart

**What goes wrong:** The project currently tracks work in TODO.md, PROJECT.md's Active section, and potentially GitHub Issues (once CI is set up). Three sources of truth inevitably diverge. A portfolio visitor sees stale TODO.md items that were completed months ago, or GitHub Issues that duplicate PROJECT.md.

**Prevention:**
1. Delete TODO.md entirely once PROJECT.md is the source of truth. One place for work tracking.
2. For portfolio presentation, use GitHub Issues with labels. Visitors expect issues, not internal planning files.
3. Close issues as commits reference them. This creates a visible history of progress.

**Phase relevance:** Documentation/cleanup phase. Do this early to avoid tracking confusion throughout the milestone.

---

### Pitfall 13: .env.example Missing New Environment Variables

**What goes wrong:** As new configuration options are added (or existing ones are documented better), `.env.example` falls out of date. A new user cloning the repo finds an `.env.example` that is missing variables documented in the README, or worse, contains variables that no longer exist.

**Prevention:**
1. Treat `.env.example` as part of the config test. After updating `config.go`, update `.env.example` in the same commit.
2. Add a comment in `.env.example` for each variable indicating whether it is required or optional (with default value).

**Phase relevance:** Documentation phase.

---

### Pitfall 14: Forgetting CGO_ENABLED=0 in CI Build Step

**What goes wrong:** The CI workflow runs `go build ./cmd/monitor/` without setting `CGO_ENABLED=0`. On GitHub Actions' Ubuntu runners, CGO is enabled by default. The build succeeds because `modernc.org/sqlite` works with CGO too, but the resulting binary is dynamically linked, which would not match the production `CGO_ENABLED=0` build from the Dockerfile.

**Prevention:**
1. Set `CGO_ENABLED: 0` as an environment variable at the job level in the GitHub Actions workflow, not just the build step. This ensures all steps (build, test, vet) use the same setting.
2. This matches the CLAUDE.md requirement: `CGO_ENABLED=0 go build ./cmd/monitor/`.

**Detection:** CI build step does not include `CGO_ENABLED=0` in the command or environment.

**Phase relevance:** CI pipeline phase.

---

### Pitfall 15: Unused Exports Visible to Portfolio Visitors

**What goes wrong:** `FormatFailureWithCode` is exported, tested, but never called in production code. A code reviewer or portfolio visitor browsing the codebase sees a function with tests but zero callers -- this looks like dead code or an abandoned feature. Similarly, the `statusCode` parameter flowing through store methods doing nothing is visible in the code.

**Prevention:**
1. Either wire up `FormatFailureWithCode` (by fixing the notification pipeline) or remove it. Do not leave tested-but-uncalled functions in a showcase project.
2. Every exported function should have at least one production caller. If it is only used in tests, make it unexported.

**Detection:** Run `gopls references` or use an IDE's "find usages" on every exported function. Any function with zero production callers is dead code.

**Phase relevance:** Tech debt cleanup phase.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|---|---|---|
| Checker interface extraction | Cascading changes break mocks and tests (Pitfall 1, 3) | Change interface first (compile-only commit), then update logic |
| Status code pipeline fix | Interface change + store migration + format change in one PR (Pitfall 1, 6) | Three separate commits: migration, interface, formatting |
| Bot handler tests | tele.Context mock boilerplate explosion (Pitfall 2) | Use struct embedding with panic-on-unimplemented base |
| Bot handler tests | Tests become HTML snapshot tests (Pitfall 11) | Test behavior (was Send called? with what args?) not output |
| Bot handler tests | Mock errors diverge from apperror sentinels (Pitfall 7) | Use `apperror.Wrap(apperror.ErrNotFound, ...)` in all mocks |
| CI pipeline | golangci-lint version mismatch (Pitfall 8) | Pin exact version that supports Go 1.26 |
| CI pipeline | Missing CGO_ENABLED=0 (Pitfall 14) | Set at job-level env, not step-level |
| CI pipeline | -race flag causes SQLite flakiness (Pitfall 9) | Omit -race initially, add after tests are deterministic |
| CI pipeline | Badges added before CI is green (Pitfall 5) | Badges go in the final README commit, after CI is verified green |
| DESIGN.md update | Updating docs to match code, then code changes again (Pitfall 4) | Update docs in the same phase as the code change, not separately |
| README rewrite | References stale DESIGN.md or nonexistent /status command (Pitfall 4) | Update DESIGN.md before README, or decouple README from DESIGN.md |
| Documentation cleanup | TODO.md, PROJECT.md, and Issues diverge (Pitfall 12) | Pick one source of truth, delete the rest |
| Production deployment | Refactored scheduler misses notifications (Pitfall 10) | Deploy only after full phase completion; manual smoke test |

## Sources

- [golangci-lint FAQ](https://golangci-lint.run/docs/welcome/faq/) - Version compatibility documentation
- [golangci-lint Go 1.26 support issue](https://github.com/golangci/golangci-lint/issues/6272) - Version support tracking
- [golangci-lint GitHub Action](https://github.com/golangci/golangci-lint-action) - Official CI action documentation
- [100 Go Mistakes](https://100go.co/) - Comprehensive Go pitfalls reference
- [7 Common Interface Mistakes in Go](https://medium.com/@andreiboar/7-common-interface-mistakes-in-go-1d3f8e58be60) - Interface design pitfalls
- [Best Practices for GitHub Markdown Badges](https://daily.dev/blog/best-practices-for-github-markdown-badges) - Badge strategy
- [telebot v4 context.go](https://github.com/tucnak/telebot/blob/v4/context.go) - Full Context interface surface area
- [Go synctest: Solving Flaky Tests](https://victoriametrics.com/blog/go-synctest/) - Flaky test patterns
- [Benchmarking SQLite Performance in Go](https://www.golang.dk/articles/benchmarking-sqlite-performance-in-go) - SQLite concurrency patterns

---

*Pitfalls audit: 2026-03-26*

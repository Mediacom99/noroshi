# Architecture Patterns

**Domain:** Production-ready Go uptime monitor (self-hosted, open-source portfolio piece)
**Researched:** 2026-03-26

## Recommended Architecture

Noroshi's current layered architecture is sound. The improvements below elevate it from "works correctly" to "signals senior craftsmanship." Five architectural dimensions need attention: testing architecture for bot handlers, CI pipeline structure, structured logging, documentation, and interface hygiene.

### Component Boundaries (Current + Proposed Changes)

| Component | Responsibility | Change Needed |
|-----------|---------------|---------------|
| `cmd/monitor/` | Wiring, lifecycle, health endpoint | None |
| `internal/config/` | Env var loading, validation | None |
| `internal/apperror/` | Sentinel errors, wrapping | None |
| `internal/storage/` | SQLite store, goose migrations | None |
| `internal/monitor/` | HTTPChecker, Scheduler | Fix: `Scheduler.checker` should be interface, not `*HTTPChecker` |
| `internal/bot/` | Telegram handlers, callbacks, formatting, validation | Add: handler/callback tests via mock `tele.Context` |
| `.github/workflows/` | CI pipeline | Add: lint, test, build jobs |

### Data Flow

No changes to data flow. The startup sequence, health check cycle, command handling, and callback flows are well-designed. The circular dependency resolution via `SetScheduler()` is an acceptable Go pattern for this scale.

---

## 1. Testing Architecture for Bot Handlers

**Confidence: HIGH** (based on telebot v4 source code analysis and established Go testing patterns)

### The Problem

Bot handlers (`handlers.go`, `callbacks.go`) are the only untested application logic. They are untested because `tele.Context` is a 46-method interface provided by the telebot library, making naive mocking painful.

### Recommended Pattern: Function-Field Mock

Use the function-field mock pattern. This is the idiomatic stdlib-only approach for large interfaces in Go. Define a mock struct where each method delegates to a function field. Methods not relevant to the test return safe zero values.

```go
// internal/bot/bot_test.go

type mockContext struct {
    sendFn     func(what interface{}, opts ...interface{}) error
    editFn     func(what interface{}, opts ...interface{}) error
    respondFn  func(resp ...*tele.CallbackResponse) error
    messageFn  func() *tele.Message
    chatFn     func() *tele.Chat
    callbackFn func() *tele.Callback

    sent []interface{} // captures what was sent
}

func newMockContext() *mockContext {
    mc := &mockContext{}
    mc.chatFn = func() *tele.Chat { return &tele.Chat{ID: 123} }
    return mc
}

func (m *mockContext) Send(what interface{}, opts ...interface{}) error {
    m.sent = append(m.sent, what)
    if m.sendFn != nil {
        return m.sendFn(what, opts...)
    }
    return nil
}

func (m *mockContext) Chat() *tele.Chat {
    if m.chatFn != nil {
        return m.chatFn()
    }
    return &tele.Chat{}
}

// ... remaining 44 methods return zero values
```

### Why This Pattern Over Alternatives

| Approach | Verdict | Reason |
|----------|---------|--------|
| Function-field mock | **Use this** | Stdlib only, compiler catches renames, selective overrides per test |
| `testify/mock` | Not allowed | Project constraint: no new dependencies |
| `mockgen` / `mockery` | Not allowed | Same constraint, plus adds code generation complexity |
| Embed `tele.nativeContext` | Fragile | Unexported type; breaks on library updates |
| Test via `Bot.ProcessUpdate()` | Too integrated | Requires a real `tele.Bot` instance, hits Telegram API setup |

### What to Test

Handlers interact with two dependencies (`Store` and `Scheduler`) plus the `tele.Context` for I/O. The testing strategy isolates these:

**Command handlers (`handlers.go`):**

| Handler | Test scenarios |
|---------|---------------|
| `handleAdd` | Missing args, invalid URL, invalid interval, interval too short, duplicate (ErrDuplicate), success, scheduler nil-safe |
| `handleDelete` | Missing arg, not found, success, scheduler nil-safe |
| `handleList` | Empty list, populated list, store error |
| `handleInterval` | Missing args, not found, invalid interval, success, scheduler reschedule |
| `handleHelp` | Returns help text |
| `findEndpoint` | Finds by ID, by name, by URL, cascading lookup |

**Callback handlers (`callbacks.go`):**

| Handler | Test scenarios |
|---------|---------------|
| `handleDetailCallback` | Valid endpoint, not found, invalid ID |
| `handleDeleteCallback` | Shows confirmation prompt |
| `handleConfirmDeleteCallback` | Deletes and refreshes list |
| `handleSetIntervalCallback` | Updates interval, reschedules |
| `handleBackCallback` | Returns to list |
| `handleRefreshCallback` | Re-renders list |

**Mock Store pattern** (already established in `scheduler_test.go`, reuse the approach):

```go
type mockStore struct {
    addEndpointFn       func(ctx context.Context, name, url string, interval int) (storage.Endpoint, error)
    getEndpointFn       func(ctx context.Context, id int64) (storage.Endpoint, error)
    getEndpointByURLFn  func(ctx context.Context, url string) (storage.Endpoint, error)
    getEndpointByNameFn func(ctx context.Context, name string) (storage.Endpoint, error)
    deleteEndpointFn    func(ctx context.Context, id int64) error
    listEndpointsFn     func(ctx context.Context) ([]storage.Endpoint, error)
    updateIntervalFn    func(ctx context.Context, id int64, interval int) error
}

func (m *mockStore) AddEndpoint(ctx context.Context, name, url string, interval int) (storage.Endpoint, error) {
    if m.addEndpointFn != nil {
        return m.addEndpointFn(ctx, name, url, interval)
    }
    return storage.Endpoint{ID: 1, Name: name, URL: url, IntervalSeconds: interval}, nil
}
```

### Test File Organization

```
internal/bot/
    bot.go
    handlers.go
    handlers_test.go     <- command handler tests
    callbacks.go
    callbacks_test.go    <- callback handler tests
    mock_test.go         <- shared mocks (mockContext, mockStore, mockScheduler)
    format.go
    format_test.go       <- already exists
    validate.go
    validate_test.go     <- already exists
```

Place all mock types in `mock_test.go` (test-only file, not compiled into binary). This keeps the mocks co-located and shared across handler/callback test files within the same package.

### Build Order Implication

The mock for `tele.Context` (46 methods) is the largest piece of work. Build it once, reuse it across all handler/callback tests. Write handler tests before callback tests since handlers are simpler (text commands vs. inline keyboard state).

---

## 2. Interface Hygiene: The Checker Dependency

**Confidence: HIGH** (visible in source code)

### The Problem

`Scheduler` depends on `*HTTPChecker` (concrete type) instead of an interface:

```go
// internal/monitor/scheduler.go:32
checker *HTTPChecker  // Should be an interface
```

This is the only interface-at-point-of-use violation in the codebase.

### The Fix

Define a `Checker` interface in `scheduler.go` (at the point of use, matching the project convention):

```go
// Checker performs health checks against a URL.
type Checker interface {
    Check(ctx context.Context, url string) (statusCode int, err error)
}
```

Change `Scheduler.checker` from `*HTTPChecker` to `Checker`. The constructor signature changes from `checker *HTTPChecker` to `checker Checker`.

### Impact

- Existing `scheduler_test.go` tests already use `NewHTTPChecker` directly, which implements the interface implicitly. Tests continue to pass unchanged.
- Future tests can inject a mock Checker that does not make real HTTP requests (faster, no network dependency).
- Aligns with Go proverb: "The bigger the interface, the weaker the abstraction." A 1-method interface is ideal.

### Build Order Implication

This is a one-line type change + one new interface definition. Do it first in the tech debt phase before writing new tests, so the Scheduler tests can optionally use a mock checker.

---

## 3. CI Pipeline Architecture

**Confidence: HIGH** (golangci-lint-action v9 docs, GitHub Actions patterns well-established)

### Recommended Pipeline Structure

Three parallel jobs, plus a conditional release job:

```
push/PR ──> [ lint ] ──\
            [ test ] ───> [ build ]
            [ vet  ] ──/
```

### Job Definitions

**Job 1: Lint (golangci-lint)**
```yaml
lint:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v6
    - uses: actions/setup-go@v6
      with:
        go-version: stable
    - uses: golangci/golangci-lint-action@v9
      with:
        version: v2.11
```

**Job 2: Test**
```yaml
test:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v6
    - uses: actions/setup-go@v6
      with:
        go-version: stable
    - run: go test -race -coverprofile=coverage.out ./...
    - name: Check coverage
      run: go tool cover -func=coverage.out
```

**Job 3: Build**
```yaml
build:
  needs: [lint, test]
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v6
    - uses: actions/setup-go@v6
      with:
        go-version: stable
    - run: CGO_ENABLED=0 go build ./cmd/monitor/
```

### golangci-lint Configuration

Use golangci-lint v2 config format. A focused config for this project size (not the 70-linter golden config, which is overkill):

```yaml
# .golangci.yml
version: "2"

linters:
  default: standard
  enable:
    - bodyclose
    - errname
    - errorlint
    - exhaustive
    - goconst
    - gocritic
    - godot
    - gosec
    - mirror
    - noctx
    - perfsprint
    - sloglint
    - unconvert
    - unparam
    - usestdlibvars

formatters:
  enable:
    - goimports
  settings:
    goimports:
      local-prefixes: noroshi

linters:
  settings:
    sloglint:
      attr-only: true
    gocritic:
      enabled-tags:
        - diagnostic
        - style
        - performance
    exhaustive:
      default-signifies-exhaustive: true
  exclusions:
    presets:
      - comments
      - std-error-handling
      - common-false-positives
    rules:
      - path: _test\.go
        linters:
          - goconst
          - gosec
          - noctx
          - funlen
```

**Key linters explained:**

| Linter | Why |
|--------|-----|
| `sloglint` with `attr-only: true` | Catches `slog.Error("msg", "key", val)` misuse; enforces `slog.String()` helpers |
| `noctx` | Catches HTTP requests without context (reinforces project's context propagation rule) |
| `errorlint` | Catches `err == ErrFoo` instead of `errors.Is(err, ErrFoo)` |
| `gosec` | Security-focused checks (relevant for a network-facing service) |
| `bodyclose` | Catches unclosed HTTP response bodies (relevant for health checker) |
| `gocritic` | Catches subtle style/performance issues |

### Badges for README

After CI is running, add these badges to README:

```markdown
[![CI](https://github.com/USER/noroshi/actions/workflows/ci.yml/badge.svg)](https://github.com/USER/noroshi/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/USER/noroshi)](https://goreportcard.com/report/github.com/USER/noroshi)
[![Go Reference](https://pkg.go.dev/badge/github.com/USER/noroshi.svg)](https://pkg.go.dev/github.com/USER/noroshi)
```

### Build Order Implication

Set up the CI workflow early in the milestone. Once it runs green, every subsequent change gets validated automatically. The linter will likely surface issues in existing code (especially `sloglint` if `attr-only` is enabled), so plan to fix lint issues immediately after enabling CI.

---

## 4. Structured Logging Architecture

**Confidence: HIGH** (slog is stdlib, patterns well-documented on go.dev)

### Current State

The project uses `slog` correctly but inconsistently:
- Mix of `slog.Error("action", "key", value, "error", err)` with loose key-value pairs
- No component-scoped loggers (scheduler manually prefixes `"scheduler: "`)
- No structured attribute helpers (uses raw string keys)

### Recommended Pattern: Component-Scoped Loggers via `slog.With`

Use `slog.With()` to create child loggers with a component attribute. Pass these via dependency injection (not stored in context, since this is not a request-per-goroutine HTTP server).

```go
// In cmd/monitor/main.go (wiring)
baseLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
slog.SetDefault(baseLogger)

// Create component loggers
schedulerLogger := baseLogger.With(slog.String("component", "scheduler"))
botLogger := baseLogger.With(slog.String("component", "bot"))
```

```go
// In internal/monitor/scheduler.go
type Scheduler struct {
    // ... existing fields
    logger *slog.Logger
}

func NewScheduler(ctx context.Context, store Store, checker Checker, notifier Notifier, maxFail int, logger *slog.Logger) (*Scheduler, error) {
    // ...
    return &Scheduler{
        // ...
        logger: logger,
    }, nil
}

// Usage in methods:
func (s *Scheduler) checkAndNotify(endpointID int64) {
    s.logger.Error("get endpoint failed", slog.Int64("id", endpointID), slog.Any("error", err))
    // Output: level=ERROR component=scheduler msg="get endpoint failed" id=42 error="..."
}
```

### Why Inject Loggers Instead of Using `slog.Default()`

| Approach | Testability | Production | Portfolio Signal |
|----------|------------|------------|-----------------|
| `slog.Default()` everywhere | Hard to capture in tests | Works fine | Junior pattern |
| Logger in `context.Context` | Moderate | Overkill for non-HTTP | Misapplied pattern |
| Logger via DI (constructor) | Easy: pass `slog.New(discard)` in tests | Clean, explicit | **Senior pattern** |

The DI approach is appropriate here because Noroshi is not an HTTP server handling concurrent requests. It has a fixed set of long-lived components, each needing its own logger scope.

### Consistent Attribute Pattern

Enforce typed attribute helpers (not raw key-value alternation):

```go
// Bad: silent failures if keys/values misalign
slog.Error("action", "id", id, "error", err)

// Good: compiler-checked, sloglint-enforced
s.logger.Error("action", slog.Int64("id", id), slog.Any("error", err))
```

The `sloglint` linter with `attr-only: true` catches the bad pattern in CI.

### Log Level Strategy

| Level | Use For | Example |
|-------|---------|---------|
| DEBUG | Detailed operational info | `"check complete" url=... status=200 duration=45ms` |
| INFO | Lifecycle events, configuration | `"bot started"`, `"loaded endpoints" count=5` |
| WARN | Recoverable issues, degraded state | `"notify failure failed, will retry"` |
| ERROR | Failures that affect functionality | `"get endpoint failed"`, `"record failure failed"` |

### Build Order Implication

Logger injection changes constructor signatures, which touches every call site in `main.go` and every test file. Do this alongside or right after the Checker interface fix, before writing new tests (so new tests use the injected logger pattern from the start).

---

## 5. Documentation Architecture for Open-Source Go Projects

**Confidence: MEDIUM** (patterns from successful open-source projects, some subjectivity in structure choices)

### README Structure

For a portfolio project, the README is the most important file. Structure it for two audiences: (1) users who want to deploy it, and (2) developers/clients evaluating code quality.

```markdown
# Noroshi - Uptime Monitor

[one-line description]

[badges: CI, Go Report Card, Go Reference, License]

[screenshot or terminal recording of Telegram interaction]

## Features
- bullet list of capabilities

## Quick Start
```bash
docker run ...
```

## Configuration
| Variable | Required | Default | Description |
|----------|----------|---------|-------------|

## Commands
| Command | Description | Example |
|---------|-------------|---------|

## Architecture
[ASCII or mermaid diagram showing component flow]

Brief paragraph explaining the design.

## Development
```bash
# Build
CGO_ENABLED=0 go build ./cmd/monitor/

# Test
go test ./...

# Lint
golangci-lint run
```

## Deployment
### Docker
### Docker Compose
### Coolify

## License
```

### Key Elements That Signal Quality

| Element | Why It Matters |
|---------|---------------|
| Badges (CI green, Go Report A+) | Immediate visual proof of quality |
| Screenshot/GIF of bot interaction | Shows the product works, not just code |
| Architecture diagram | Shows you think about system design |
| Clear configuration table | Shows you care about operator experience |
| Development section | Shows you expect contributors |

### Architecture Diagram Format

Use ASCII art (renders everywhere, no external tooling) or Mermaid (GitHub renders natively):

```
                    +------------------+
  Telegram <------> |   Bot (telebot)  |
                    +--------+---------+
                             |
                    +--------+---------+
                    |   Scheduler      |
                    |   (gocron)       |
                    +--------+---------+
                             |
              +--------------+--------------+
              |                             |
     +--------+--------+          +--------+--------+
     |  HTTP Checker    |          |  SQLite Store   |
     |  (retryablehttp) |          |  (goose + WAL)  |
     +-----------------+          +-----------------+
```

### Supporting Documentation

| File | Content | Audience |
|------|---------|----------|
| `README.md` | Quick start, features, config, deployment | Users + evaluators |
| `DESIGN.md` | Architecture decisions, why choices were made | Developers + evaluators |
| `CLAUDE.md` | AI assistant rules (already exists) | Development tooling |
| `CHANGELOG.md` | Version history | Users tracking updates |

Do NOT over-document. Four files total. No `/docs/` directory for a project this size.

### Build Order Implication

Write the README last in the milestone, after all code changes are done. The README references CI badges (needs CI set up first), architecture (needs tech debt fixed first), and features (needs any new features done first). Writing it last ensures accuracy.

---

## Anti-Patterns to Avoid

### Anti-Pattern 1: Over-Abstracting for Scale

**What:** Adding interfaces, service layers, or repository patterns beyond what the project needs.
**Why bad:** Noroshi monitors tens of endpoints from a single SQLite file. Adding a generic Repository pattern, event bus, or plugin system signals over-engineering.
**Instead:** Keep the current flat `Store` interface. Only introduce abstractions when you have two concrete implementations.

### Anti-Pattern 2: Testing via Integration When Unit Tests Suffice

**What:** Spinning up a real `tele.Bot` or real SQLite database to test handler logic.
**Why bad:** Slow, flaky, requires network access or file system state.
**Instead:** Use mocks for handlers. Reserve integration tests (real SQLite via `store_test.go`) for the storage layer only.

### Anti-Pattern 3: Global Logger in Tests

**What:** Tests that write to `slog.Default()` and pollute test output.
**Why bad:** Noisy, impossible to assert log output, masks failing tests.
**Instead:** Inject `slog.New(slog.NewTextHandler(io.Discard, nil))` in test constructors. For tests that need to assert log output, inject a handler that writes to a `bytes.Buffer`.

### Anti-Pattern 4: CI That Does Not Match Local Checks

**What:** CI runs `golangci-lint` but developers only run `go vet` locally.
**Why bad:** Developers get surprised by CI failures. Friction to contribute.
**Instead:** Document the exact lint command in README and CLAUDE.md. Consider a `Makefile` with `make lint` target.

### Anti-Pattern 5: README-Driven Development

**What:** Writing an impressive README for features that do not exist yet.
**Why bad:** Creates expectations the code cannot meet. Evaluators who read the code will notice.
**Instead:** README reflects only what is implemented and tested.

---

## Scalability Considerations

| Concern | Current (10 endpoints) | At 100 endpoints | At 1000+ endpoints |
|---------|----------------------|-------------------|---------------------|
| SQLite writes | Fine (WAL mode) | Fine (WAL mode) | May need write batching |
| gocron jobs | Fine | Fine | Fine (gocron uses goroutine pool) |
| Telegram API | Fine (long polling) | Fine | Rate limits may apply on notifications |
| Memory | Minimal | Minimal | Minimal (no in-memory cache) |

Noroshi does not need to scale beyond 100 endpoints. Do not pre-optimize.

---

## Patterns to Follow

### Pattern 1: Table-Driven Tests with Subtests

**What:** Use `t.Run()` with named subtests for each scenario.
**When:** Every test function with more than one scenario.

```go
func TestHandleAdd(t *testing.T) {
    tests := []struct {
        name     string
        payload  string
        wantSent string
        store    *mockStore
    }{
        {
            name:     "missing args",
            payload:  "",
            wantSent: "Usage:",
        },
        {
            name:     "invalid URL",
            payload:  "test ftp://bad",
            wantSent: "Invalid URL",
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ... test body
        })
    }
}
```

### Pattern 2: Constructor Accepts All Dependencies

**What:** Every unexported field is set via the constructor. No `Set*` methods except for circular dependency resolution.
**When:** Always.
**Why:** Makes nil-field bugs impossible. The one exception (`SetScheduler`) is documented and nil-checked.

### Pattern 3: Errors Are Part of the API Contract

**What:** Handler tests verify both the happy path response AND the error responses.
**When:** Every handler test.
**Why:** User-facing error messages are product behavior, not implementation details.

---

## Sources

- [telebot v4 Context interface (46 methods)](https://github.com/tucnak/telebot/blob/v4/context.go) -- HIGH confidence
- [golangci-lint-action v9 (official)](https://github.com/golangci/golangci-lint-action) -- HIGH confidence
- [golangci-lint v2 config](https://golangci-lint.run/docs/configuration/) -- HIGH confidence
- [Golden golangci-lint config](https://gist.github.com/maratori/47a4d00457a92aa426dbd48a18776322) -- MEDIUM confidence (community, not official)
- [Go slog guide (Dash0)](https://www.dash0.com/guides/logging-in-go-with-slog) -- HIGH confidence
- [Go slog official blog post](https://go.dev/blog/slog) -- HIGH confidence
- [Function-field mock pattern (SafetyCulture)](https://medium.com/safetycultureengineering/flexible-mocking-for-testing-in-go-f952869e34f5) -- HIGH confidence
- [Go CI pipeline guide (OneUptime)](https://oneuptime.com/blog/post/2025-12-20-go-ci-pipeline-github-actions/view) -- MEDIUM confidence
- [Go README best practices](https://dev.to/github/how-to-create-the-perfect-readme-for-your-open-source-project-1k69) -- MEDIUM confidence
- [Shields.io badges](https://github.com/dwyl/repo-badges) -- MEDIUM confidence

---

*Architecture research: 2026-03-26*

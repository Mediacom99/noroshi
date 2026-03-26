# Phase 2: Bot Handler Tests - Research

**Researched:** 2026-03-26
**Domain:** Go testing, Telegram bot mock infrastructure, table-driven handler tests
**Confidence:** HIGH

## Summary

This phase adds comprehensive test coverage to the `internal/bot/` package, which is the only package currently lacking handler/callback tests. The bot handlers (`handlers.go`) and callback handlers (`callbacks.go`) are methods on the `Bot` struct that interact with three dependencies: `tele.Context` (Telegram), `Store` (database), and `Scheduler` (job scheduling). All three must be mocked for unit testing.

The critical insight for this phase is that `tele.Context` is a large interface (40+ methods) but handlers only use 6 methods: `Send`, `Edit`, `Respond`, `Message`, `Callback`, and `Chat`. The CONTEXT.md-decided struct-embedding pattern (embed `tele.Context` to satisfy the interface, override only used methods) is the correct approach. The `Bot` struct can be constructed directly without `NewBot` for testing -- the handler methods never reference `b.bot` (the underlying `tele.Bot`), only `b.store`, `b.scheduler`, `b.rootCtx`, and `b.chatID`.

For TEST-04, the `Checker` interface was already extracted in Phase 1. The existing scheduler tests use real `httptest` servers; new mock-based tests will use a `mockChecker` with function-field pattern to be fully deterministic without HTTP overhead.

**Primary recommendation:** Build a `mock_test.go` file with `mockContext`, `mockStore`, and `mockScheduler` using function-field pattern, then write table-driven tests for all 5 command handlers and 7 callback handlers, and add mock-based scheduler tests alongside existing httptest-based ones.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Use struct embedding pattern -- define a `baseMockContext` struct that embeds `tele.Context` (satisfies the interface with panicking defaults), then override only the methods handlers actually call
- **D-02:** Capture outputs in slice/value fields: `sentMessages []string`, `editedMessages []string`, `respondCalls []tele.CallbackResponse` for assertion
- **D-03:** Use function-field pattern for input methods: `messageFn func() *tele.Message`, `callbackFn func() *tele.Callback`, `chatFn func() *tele.Chat` -- allows each test case to inject different inputs
- **D-04:** All three mock types (`mockContext`, `mockStore`, `mockScheduler`) live in `internal/bot/mock_test.go` as shared infrastructure
- **D-05:** `mockStore` and `mockScheduler` use function-field pattern (e.g., `addEndpointFn func(...) (...)`) so each test case can inject different behavior inline
- **D-06:** Use substring checks (`strings.Contains`) for bot response text, not exact string matching
- **D-07:** Assert on key content markers: endpoint names, URLs, error messages, emoji indicators -- not on full HTML structure
- **D-08:** Add new mock-based tests alongside existing httptest-based tests -- don't rewrite existing tests
- **D-09:** Create a `mockChecker` with function-field pattern (`checkFn func(ctx, url) (int, error)`) in `scheduler_test.go`
- **D-10:** New mock-based tests cover the same scenarios (OK, failure, failure cap, recovery) but run faster and are fully deterministic
- **D-11:** Test `guarded()` implicitly through handler tests -- include test cases with wrong chat ID to verify unauthorized messages are ignored
- **D-12:** Test `findEndpoint()` implicitly through `handleDelete` and `handleInterval` tests -- include test cases that look up by ID, by name, and by URL
- **D-13:** Test `editEndpointList()` implicitly through callback tests (back, confirm-delete, refresh all call it) -- no dedicated unit test needed

### Claude's Discretion
- Exact test case names and descriptions within the table-driven pattern
- Order of test file creation (mock infrastructure first is implied)
- Whether `mockContext` needs helper constructors or if inline struct literals suffice
- How to simulate `c.Message().Payload` (likely via a real `tele.Message` struct stored in mock)
- How to handle `c.Callback().Data` (likely via a real `tele.Callback` struct stored in mock)

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TEST-01 | Shared mock infrastructure for `tele.Context` interface using struct embedding pattern in `internal/bot/mock_test.go` | Fully supported -- struct embedding of `tele.Context` with function-field overrides for `Message()`, `Callback()`, `Chat()`, `Send()`, `Edit()`, `Respond()`; `mockStore` and `mockScheduler` with function-field pattern |
| TEST-02 | Table-driven tests for all bot command handlers (`/add`, `/delete`, `/list`, `/interval`, `/help`) | Fully supported -- 5 handlers identified with all code paths mapped; `Bot` struct constructible without `NewBot` for testing |
| TEST-03 | Tests for bot callback handlers (detail view, refresh, back, delete confirmation) | Fully supported -- 7 callback handlers identified; callback data passed via `tele.Callback.Data` field |
| TEST-04 | Scheduler tests use mock Checker interface instead of real HTTP servers | Fully supported -- `Checker` interface already exists with single `Check(ctx, url) (int, error)` method; `mockChecker` with function-field pattern |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `testing` | stdlib | Test framework | Only allowed test framework per CLAUDE.md |
| `strings` | stdlib | Substring assertions | Decision D-06 mandates `strings.Contains` checks |
| `context` | stdlib | Test context creation | `context.Background()` acceptable in tests |
| `gopkg.in/telebot.v4` | v4.0.0-beta.7 | `tele.Context` interface, `tele.Message`, `tele.Callback`, `tele.Chat` types | Already in go.mod; mock embeds `tele.Context` |
| `noroshi/internal/apperror` | local | `ErrNotFound`, `ErrDuplicate` sentinels | mockStore returns these to test error branches |
| `noroshi/internal/storage` | local | `storage.Endpoint` model struct | Used as return types from mockStore |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `fmt` | stdlib | Error message construction in mocks | When mock functions need to return formatted errors |
| `errors` | stdlib | `errors.Is()` in test assertions | Verifying error handling paths |
| `time` | stdlib | Duration values in interval tests | Testing interval validation and formatting |

**No new dependencies required.** All testing uses stdlib `testing` + existing project imports.

## Architecture Patterns

### Test File Structure
```
internal/bot/
  mock_test.go         # NEW: shared mock infrastructure (TEST-01)
  handlers_test.go     # NEW: command handler tests (TEST-02)
  callbacks_test.go    # NEW: callback handler tests (TEST-03)
  format_test.go       # EXISTS: pure function tests
  validate_test.go     # EXISTS: pure function tests

internal/monitor/
  scheduler_test.go    # MODIFIED: add mockChecker + mock-based tests (TEST-04)
```

### Pattern 1: Bot Construction for Testing (No Telegram Token)

**What:** Construct `Bot` struct directly without `NewBot` since handler methods never reference `b.bot`
**When to use:** Every handler and callback test
**Why safe:** Handler methods only access `b.store`, `b.scheduler`, `b.rootCtx`, and `b.chatID`. The `b.bot` field (real `tele.Bot`) is only used by `registerHandlers`, `registerCallbacks`, `registerCommands`, `Start`, `Stop`, `SendMessage`, `SendSilentMessage` -- none of which are under test here.

```go
func newTestBot(store Store, scheduler Scheduler) *Bot {
    return &Bot{
        store:     store,
        scheduler: scheduler,
        chatID:    123,
        rootCtx:   context.Background(),
    }
}
```

### Pattern 2: mockContext with Struct Embedding (D-01, D-02, D-03)

**What:** Embed `tele.Context` to satisfy the 40+ method interface; override 6 methods handlers actually call
**When to use:** All handler and callback tests

```go
type mockContext struct {
    tele.Context // panicking defaults for unused methods

    // Input injection (function-field pattern)
    messageFn  func() *tele.Message
    callbackFn func() *tele.Callback
    chatFn     func() *tele.Chat

    // Output capture
    sentMessages    []string
    editedMessages  []string
    respondCalls    []tele.CallbackResponse
    sendOpts        [][]interface{} // capture options passed to Send
}

func (m *mockContext) Message() *tele.Message {
    if m.messageFn != nil {
        return m.messageFn()
    }
    return nil
}

func (m *mockContext) Chat() *tele.Chat {
    if m.chatFn != nil {
        return m.chatFn()
    }
    return &tele.Chat{ID: 123} // default authorized chat
}

func (m *mockContext) Send(what interface{}, opts ...interface{}) error {
    if s, ok := what.(string); ok {
        m.sentMessages = append(m.sentMessages, s)
    }
    return nil
}

func (m *mockContext) Edit(what interface{}, opts ...interface{}) error {
    if s, ok := what.(string); ok {
        m.editedMessages = append(m.editedMessages, s)
    }
    return nil
}

func (m *mockContext) Respond(resp ...*tele.CallbackResponse) error {
    if len(resp) > 0 && resp[0] != nil {
        m.respondCalls = append(m.respondCalls, *resp[0])
    }
    return nil
}

func (m *mockContext) Callback() *tele.Callback {
    if m.callbackFn != nil {
        return m.callbackFn()
    }
    return nil
}
```

### Pattern 3: mockStore with Function-Field Pattern (D-05)

**What:** Each method delegates to a function field, allowing per-test-case behavior injection
**When to use:** All handler and callback tests

```go
type mockStore struct {
    addEndpointFn          func(ctx context.Context, name, url string, interval int) (storage.Endpoint, error)
    getEndpointFn          func(ctx context.Context, id int64) (storage.Endpoint, error)
    getEndpointByURLFn     func(ctx context.Context, url string) (storage.Endpoint, error)
    getEndpointByNameFn    func(ctx context.Context, name string) (storage.Endpoint, error)
    deleteEndpointFn       func(ctx context.Context, id int64) error
    listEndpointsFn        func(ctx context.Context) ([]storage.Endpoint, error)
    updateEndpointIntervalFn func(ctx context.Context, id int64, interval int) error
}

func (m *mockStore) AddEndpoint(ctx context.Context, name, url string, interval int) (storage.Endpoint, error) {
    if m.addEndpointFn != nil {
        return m.addEndpointFn(ctx, name, url, interval)
    }
    return storage.Endpoint{}, nil
}
// ... same pattern for all methods
```

### Pattern 4: Simulating c.Message().Payload for Command Arguments

**What:** `Payload` field on `tele.Message` contains the text after the command. Handlers parse it with `strings.Fields(c.Message().Payload)`.
**When to use:** All command handler tests (`/add`, `/delete`, `/interval`)

```go
// For "/add prod-api https://example.com 30s"
mc := &mockContext{
    messageFn: func() *tele.Message {
        return &tele.Message{Payload: "prod-api https://example.com 30s"}
    },
    chatFn: func() *tele.Chat {
        return &tele.Chat{ID: 123}
    },
}
```

### Pattern 5: Simulating c.Callback().Data for Callback Arguments

**What:** `Data` field on `tele.Callback` contains pipe-separated values set by inline keyboard buttons.
**When to use:** All callback handler tests

```go
// For detail callback with endpoint ID 42
mc := &mockContext{
    callbackFn: func() *tele.Callback {
        return &tele.Callback{Data: "42"}
    },
    chatFn: func() *tele.Chat {
        return &tele.Chat{ID: 123}
    },
}

// For set-interval callback with endpoint ID 42 and 300 seconds
mc := &mockContext{
    callbackFn: func() *tele.Callback {
        return &tele.Callback{Data: "42|300"}
    },
}
```

### Pattern 6: Table-Driven Handler Tests (D-06, D-07)

**What:** Each handler gets one test function with a table of test cases
**When to use:** All handlers

```go
func TestHandleAdd(t *testing.T) {
    tests := []struct {
        name          string
        payload       string
        chatID        int64
        store         *mockStore
        scheduler     *mockScheduler
        wantContains  []string // substring checks per D-06
        wantNoSend    bool     // for guarded() tests
    }{
        {
            name:    "happy path with default interval",
            payload: "prod-api https://example.com",
            chatID:  123,
            store: &mockStore{
                addEndpointFn: func(_ context.Context, name, url string, interval int) (storage.Endpoint, error) {
                    return storage.Endpoint{ID: 1, Name: name, URL: url, IntervalSeconds: interval}, nil
                },
            },
            scheduler:    &mockScheduler{},
            wantContains: []string{"Added endpoint", "prod-api", "https://example.com"},
        },
        // ... more cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            b := newTestBot(tt.store, tt.scheduler)
            if tt.chatID != 0 {
                b.chatID = tt.chatID
            }
            mc := &mockContext{
                messageFn: func() *tele.Message {
                    return &tele.Message{Payload: tt.payload}
                },
                chatFn: func() *tele.Chat {
                    return &tele.Chat{ID: tt.chatID}
                },
            }
            err := b.handleAdd(mc)
            // assertions...
        })
    }
}
```

### Anti-Patterns to Avoid
- **Testing through `guarded()` wrapper for every test:** Call handler methods directly (e.g., `b.handleAdd(mc)`) for most tests. Only test guarded behavior for a few specific cases per D-11.
- **Exact string matching on bot responses:** Use `strings.Contains` per D-06. Bot messages include emoji and HTML that change easily.
- **Rewriting existing scheduler httptest tests:** Per D-08, add new mock-based tests alongside. Existing tests provide integration coverage.
- **Mocking `tele.Context` with a full implementation:** The struct embedding approach satisfies the interface automatically; unused methods panic (which is actually desirable -- if a test triggers an unmocked method, the panic reveals a gap).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Full `tele.Context` mock | 40+ method implementation | Struct embedding of `tele.Context` interface | Embedded nil interface panics on unused methods, which is a feature -- reveals test gaps |
| Complex assertion library | Custom `assertContains` helpers | Direct `strings.Contains` + `t.Errorf` | CLAUDE.md forbids testify; stdlib assertions are simple enough |

## Common Pitfalls

### Pitfall 1: Nil Panic from Embedded tele.Context
**What goes wrong:** The embedded `tele.Context` is nil by default (zero value). Calling any un-overridden method panics with nil pointer dereference instead of a clear "method not implemented" message.
**Why it happens:** Struct embedding of an interface type means the embedded field starts as nil.
**How to avoid:** This is actually desirable for tests -- a panic means the handler is calling a method the mock doesn't support, revealing a gap. If a clearer error is needed, consider initializing the embedded field with a stub that has named panics.
**Warning signs:** Test panics with `nil pointer dereference` instead of clear assertion failure.

### Pitfall 2: c.Respond() Variadic Signature
**What goes wrong:** `Respond(resp ...*tele.CallbackResponse) error` can be called with zero arguments (just `c.Respond()` to acknowledge), one argument (with text), or multiple. The mock must handle all cases.
**Why it happens:** Callbacks typically call `c.Respond()` (no args) to acknowledge + separately call `c.Edit()` for content, OR call `c.Respond(&tele.CallbackResponse{Text: "..."})` for toast-style messages.
**How to avoid:** In the mock, check `len(resp) > 0 && resp[0] != nil` before capturing. Empty responds are common (acknowledge only) and should not be recorded as assertions.
**Warning signs:** Tests fail because `respondCalls` has unexpected empty entries.

### Pitfall 3: Callback Data Format is Handler-Specific
**What goes wrong:** Different callbacks expect different `Data` formats. Detail/delete/interval expect a plain int64 ID (`"42"`), but `handleSetIntervalCallback` expects pipe-separated `"42|300"`.
**Why it happens:** Telebot encodes inline button data as `unique|data` where `data` is what `c.Callback().Data` returns. The code in `handleSetIntervalCallback` does `strings.Split(c.Callback().Data, "|")` to get two parts.
**How to avoid:** Match the exact format each handler expects. Document data format per handler.
**Warning signs:** Tests for `handleSetIntervalCallback` pass wrong-format data and get "Invalid data" response instead of the expected behavior.

### Pitfall 4: Bot Struct Construction Must Set rootCtx
**What goes wrong:** Handlers call `b.store.AddEndpoint(b.rootCtx, ...)` -- if `rootCtx` is nil, all store calls receive nil context which may cause subtle issues.
**Why it happens:** Constructing `Bot` directly (not via `NewBot`) might forget to set `rootCtx`.
**How to avoid:** Always set `rootCtx: context.Background()` in the test helper.
**Warning signs:** Store mock receives nil context.

### Pitfall 5: Scheduler Nil Check in Handlers
**What goes wrong:** Handlers check `if b.scheduler != nil` before calling scheduler methods. Tests must verify both paths: with scheduler set and with scheduler nil.
**Why it happens:** The bot-scheduler circular dependency means scheduler is set after construction.
**How to avoid:** Include test cases with nil scheduler to verify handlers work without scheduler (graceful degradation). Include test cases with scheduler to verify scheduler calls happen.
**Warning signs:** Missing coverage of the `scheduler == nil` branches.

### Pitfall 6: mockStore for findEndpoint Cascade
**What goes wrong:** `findEndpoint(arg)` tries: (1) parse as int64 -> `GetEndpoint`, (2) `GetEndpointByName`, (3) `GetEndpointByURL`. If testing name lookup, `GetEndpointByName` must return the endpoint. If testing "not found", ALL three must return `ErrNotFound`.
**Why it happens:** The cascade means multiple mock functions are called in sequence.
**How to avoid:** For "not found" tests, set all three functions to return `ErrNotFound`. For name lookup tests, ensure `GetEndpointByName` returns success. For ID lookup tests, provide a numeric arg so `GetEndpoint` is called directly.
**Warning signs:** Unexpected "Endpoint not found" when testing name-based lookup because `GetEndpointByName` was not set.

## Code Examples

### Handler Method Coverage Map

Each handler and the `tele.Context` methods it calls:

| Handler | Context Methods | Store Methods | Scheduler Methods | Error Paths |
|---------|----------------|---------------|-------------------|-------------|
| `handleAdd` | `Message().Payload`, `Send` | `AddEndpoint` | `Add` | too few args, invalid name, invalid URL, invalid interval, interval < 10s, `ErrDuplicate`, store error, scheduler error |
| `handleDelete` | `Message().Payload`, `Send` | `GetEndpoint`/`GetEndpointByName`/`GetEndpointByURL` (via `findEndpoint`), `DeleteEndpoint` | `Remove` | empty arg, `ErrNotFound`, store error on find, store error on delete |
| `handleList` | `Send` | `ListEndpoints` | (none) | store error, empty list, non-empty list |
| `handleInterval` | `Message().Payload`, `Send` | `GetEndpoint`/`GetEndpointByName`/`GetEndpointByURL` (via `findEndpoint`), `UpdateEndpointInterval` | `Remove`, `Add` | too few args, `ErrNotFound`, invalid interval, interval < 10s, store error, scheduler error |
| `handleHelp` | `Send` | (none) | (none) | (none -- always succeeds) |
| `handleDetailCallback` | `Callback().Data`, `Edit`, `Respond` | `GetEndpoint` | (none) | invalid ID, not found |
| `handleDeleteCallback` | `Callback().Data`, `Edit`, `Respond` | `GetEndpoint` | (none) | invalid ID, not found |
| `handleConfirmDeleteCallback` | `Callback().Data`, `Respond`, `Edit` (via `editEndpointList`) | `GetEndpoint`, `DeleteEndpoint`, `ListEndpoints` | `Remove` | invalid ID, not found, delete error |
| `handleBackCallback` | `Respond`, `Edit` (via `editEndpointList`) | `ListEndpoints` | (none) | store error |
| `handleIntervalCallback` | `Callback().Data`, `Edit`, `Respond` | `GetEndpoint` | (none) | invalid ID, not found |
| `handleSetIntervalCallback` | `Callback().Data`, `Respond`, `Edit` (via `editEndpointList`) | `GetEndpoint`, `UpdateEndpointInterval`, `ListEndpoints` | `Remove`, `Add` | invalid data format, invalid ID, invalid seconds, not found, update error |
| `handleRefreshCallback` | `Respond`, `Edit` (via `editEndpointList`) | `ListEndpoints` | (none) | store error |

### mockScheduler Pattern

```go
type mockScheduler struct {
    addFn    func(ctx context.Context, ep storage.Endpoint) error
    removeFn func(endpointID int64) error
    addCalls    int
    removeCalls int
}

func (m *mockScheduler) Add(ctx context.Context, ep storage.Endpoint) error {
    m.addCalls++
    if m.addFn != nil {
        return m.addFn(ctx, ep)
    }
    return nil
}

func (m *mockScheduler) Remove(endpointID int64) error {
    m.removeCalls++
    if m.removeFn != nil {
        return m.removeFn(endpointID)
    }
    return nil
}
```

### mockChecker Pattern for TEST-04 (in scheduler_test.go)

```go
type mockChecker struct {
    checkFn func(ctx context.Context, url string) (int, error)
}

func (m *mockChecker) Check(ctx context.Context, url string) (int, error) {
    return m.checkFn(ctx, url)
}
```

### Guarded Middleware Test (D-11)

```go
{
    name:   "wrong chat ID ignored",
    chatID: 999,  // authorized ID is 123
    // ... setup ...
    wantNoSend: true,  // guarded() returns nil, no Send called
}
```

Note: To test guarded behavior, call the guarded wrapper (`b.guarded(b.handleAdd)`) not the raw handler. The test needs to verify that the handler func returned by `guarded()` returns nil without calling Send when chat ID doesn't match.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` |
| Config file | none (stdlib, no config needed) |
| Quick run command | `go test ./internal/bot/ ./internal/monitor/` |
| Full suite command | `go test ./...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TEST-01 | Shared mock infrastructure compiles and is usable | unit (compilation test via usage) | `go test ./internal/bot/ -run TestHandle` | No -- Wave 0 |
| TEST-02 | All 5 command handlers tested (happy + error) | unit | `go test ./internal/bot/ -run "TestHandle(Add\|Delete\|List\|Interval\|Help)" -v` | No -- Wave 0 |
| TEST-03 | All 7 callback handlers tested (happy + error) | unit | `go test ./internal/bot/ -run "TestHandle.*Callback" -v` | No -- Wave 0 |
| TEST-04 | Mock checker scheduler tests | unit | `go test ./internal/monitor/ -run "TestCheckAndNotifyMock" -v` | No -- Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/bot/ ./internal/monitor/`
- **Per wave merge:** `go test ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/bot/mock_test.go` -- shared mock infrastructure (TEST-01)
- [ ] `internal/bot/handlers_test.go` -- command handler tests (TEST-02)
- [ ] `internal/bot/callbacks_test.go` -- callback handler tests (TEST-03)
- [ ] `internal/monitor/scheduler_test.go` -- add mockChecker + mock-based tests (TEST-04, modify existing file)

## Open Questions

1. **Should mockContext track Send options (tele.NoPreview, tele.Silent)?**
   - What we know: Handlers pass `tele.NoPreview` to many `Send` calls. Current decision (D-02) captures message text only.
   - What's unclear: Whether tests should assert on options or just message content.
   - Recommendation: Capture options in a separate `sendOpts [][]interface{}` field for future use, but don't assert on them initially. Focus on message content assertions per D-07. This is within Claude's discretion.

2. **Should mockContext helper constructors be created?**
   - What we know: Decision D-03 mandates function-field pattern. CONTEXT.md leaves "helper constructors vs inline struct literals" to Claude's discretion.
   - What's unclear: Which approach produces more readable test tables.
   - Recommendation: Create a minimal `newMockContext` helper that sets up the default authorized chat ID and accepts optional functional options. Inline overrides work well for simple cases. Test tables should remain readable without excessive setup.

## Project Constraints (from CLAUDE.md)

These directives MUST be followed in all implementation:

- **Testing framework:** stdlib `testing` only -- no testify, no gomock
- **Table-driven tests:** Required where applicable (all handler tests qualify)
- **No new dependencies:** Nothing outside the mandatory library table
- **context.Background():** Acceptable in tests (explicitly noted in conventions)
- **Interfaces at point of use:** mockStore/mockScheduler implement the bot-package interfaces, not the storage/monitor-package interfaces
- **go test ./... must pass** before every commit
- **go vet ./... must pass** before every commit
- **CGO_ENABLED=0 go build ./cmd/monitor/** must pass before every commit

## Sources

### Primary (HIGH confidence)
- `internal/bot/bot.go` -- Bot struct, Store/Scheduler interfaces (7 Store methods, 2 Scheduler methods), guarded middleware, field layout
- `internal/bot/handlers.go` -- All 5 command handlers + findEndpoint + sendEndpointList, exact parameter usage
- `internal/bot/callbacks.go` -- All 7 callback handlers + editEndpointList, callback data formats
- `internal/bot/format.go` -- FormatEndpointList, FormatEndpointDetail return types (string + *tele.ReplyMarkup)
- `internal/monitor/scheduler.go` -- Checker interface (`Check(ctx, url) (int, error)`), checkAndNotify logic
- `internal/monitor/scheduler_test.go` -- Existing mock patterns (state-based with mutex), existing httptest tests
- `internal/bot/format_test.go` -- Existing substring assertion pattern for HTML messages
- `internal/bot/validate_test.go` -- Existing table-driven test pattern
- `internal/storage/models.go` -- Endpoint struct fields
- `go doc gopkg.in/telebot.v4.Context` -- Full interface definition (40+ methods)
- `go doc gopkg.in/telebot.v4.Message` -- Payload field for command arguments
- `go doc gopkg.in/telebot.v4.Callback` -- Data field for callback arguments
- `go doc gopkg.in/telebot.v4.CallbackResponse` -- Response struct for Respond calls

### Secondary (MEDIUM confidence)
- `.planning/codebase/TESTING.md` -- Project-wide testing patterns and conventions analysis
- `.planning/phases/02-bot-handler-tests/02-CONTEXT.md` -- All locked decisions (D-01 through D-13)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All stdlib + existing project deps, no new libraries needed
- Architecture: HIGH - All source code read directly, handler method signatures and dependencies fully mapped
- Pitfalls: HIGH - Based on direct code analysis of handler implementations and interface contracts

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable -- stdlib testing, no external changes expected)

# Phase 2: Bot Handler Tests - Context

**Gathered:** 2026-03-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Build shared mock infrastructure and comprehensive table-driven tests for all bot command handlers and callback handlers. Every handler has tests covering success and error paths. Scheduler tests gain mock Checker support for deterministic testing.

</domain>

<decisions>
## Implementation Decisions

### Mock tele.Context (TEST-01)
- **D-01:** Use struct embedding pattern — define a `baseMockContext` struct that embeds `tele.Context` (satisfies the interface with panicking defaults), then override only the methods handlers actually call
- **D-02:** Capture outputs in slice/value fields: `sentMessages []string`, `editedMessages []string`, `respondCalls []tele.CallbackResponse` for assertion
- **D-03:** Use function-field pattern for input methods: `messageFn func() *tele.Message`, `callbackFn func() *tele.Callback`, `chatFn func() *tele.Chat` — allows each test case to inject different inputs
- **D-04:** All three mock types (`mockContext`, `mockStore`, `mockScheduler`) live in `internal/bot/mock_test.go` as shared infrastructure
- **D-05:** `mockStore` and `mockScheduler` use function-field pattern (e.g., `addEndpointFn func(...) (...)`) so each test case can inject different behavior inline

### Test assertion style (TEST-02, TEST-03)
- **D-06:** Use substring checks (`strings.Contains`) for bot response text, not exact string matching — follows the existing pattern in `format_test.go` and is resilient to cosmetic formatting changes
- **D-07:** Assert on key content markers: endpoint names, URLs, error messages, emoji indicators — not on full HTML structure

### Scheduler mock tests (TEST-04)
- **D-08:** Add new mock-based tests alongside existing httptest-based tests — don't rewrite the existing tests, they provide valuable integration-level coverage
- **D-09:** Create a `mockChecker` with function-field pattern (`checkFn func(ctx, url) (int, error)`) in `scheduler_test.go`
- **D-10:** New mock-based tests cover the same scenarios (OK, failure, failure cap, recovery) but run faster and are fully deterministic

### Coverage boundary
- **D-11:** Test `guarded()` implicitly through handler tests — include test cases with wrong chat ID to verify unauthorized messages are ignored
- **D-12:** Test `findEndpoint()` implicitly through `handleDelete` and `handleInterval` tests — include test cases that look up by ID, by name, and by URL
- **D-13:** Test `editEndpointList()` implicitly through callback tests (back, confirm-delete, refresh all call it) — no dedicated unit test needed

### Claude's Discretion
- Exact test case names and descriptions within the table-driven pattern
- Order of test file creation (mock infrastructure first is implied)
- Whether `mockContext` needs helper constructors or if inline struct literals suffice
- How to simulate `c.Message().Payload` (likely via a real `tele.Message` struct stored in mock)
- How to handle `c.Callback().Data` (likely via a real `tele.Callback` struct stored in mock)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements and constraints
- `.planning/REQUIREMENTS.md` — TEST-01 through TEST-04 acceptance criteria
- `.planning/ROADMAP.md` §Phase 2 — Success criteria (4 items)
- `CLAUDE.md` — Testing requirements (stdlib only, table-driven, no testify/gomock)

### Codebase analysis
- `.planning/codebase/TESTING.md` — Full testing patterns, mock conventions, coverage map
- `.planning/codebase/CONVENTIONS.md` — Code organization, interface placement, naming patterns

### Source files to test
- `internal/bot/bot.go` — Bot struct, Store/Scheduler interfaces, guarded middleware, SendMessage/SendSilentMessage
- `internal/bot/handlers.go` — 5 command handlers (add, delete, list, interval, help) + findEndpoint + sendEndpointList
- `internal/bot/callbacks.go` — 7 callback handlers (detail, delete, confirm-delete, back, interval, set-interval, refresh) + editEndpointList

### Existing mock patterns
- `internal/monitor/scheduler_test.go` — mockStore, mockNotifier patterns (function-field + mutex + counter methods)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/monitor/scheduler_test.go`: Established mock patterns (state-based with mutex, counter methods) — same style for bot mocks
- `internal/bot/format_test.go`: Substring assertion pattern for HTML messages — reuse in handler tests
- `internal/bot/validate_test.go`: Table-driven test pattern — same structure for handler tests

### Established Patterns
- Mocks implement consumer-side interfaces, not producing-side structs
- Table-driven tests with `name`, input fields, and `want`/`wantErr` fields
- `t.Helper()` on setup functions, `t.Cleanup()` for teardown
- `context.Background()` acceptable in tests

### Integration Points
- `tele.Context` interface — the primary mock target; handlers call Send, Edit, Respond, Message, Chat, Callback
- `storage.Endpoint` — used as return types from mockStore; construct inline with test data
- `apperror` sentinels — mockStore returns these to test error branches (ErrNotFound, ErrDuplicate)

</code_context>

<specifics>
## Specific Ideas

No specific requirements — user deferred all decisions to Claude. Standard approaches apply throughout.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 02-bot-handler-tests*
*Context gathered: 2026-03-26*

# Phase 2: Bot Handler Tests - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-26
**Phase:** 02-bot-handler-tests
**Areas discussed:** Mock tele.Context design, Assertion granularity, Scheduler test migration, Coverage boundary

---

## Gray Area Selection

| Option | Description | Selected |
|--------|-------------|----------|
| Mock tele.Context design | How to mock telebot's Context interface — struct embedding vs function-field pattern | |
| Assertion granularity | Exact bot response strings vs key substrings | |
| Scheduler test migration | Rewrite existing tests vs add mock-based alongside | |
| Coverage boundary | Dedicated tests for helpers vs implicit coverage through handlers | |

**User's choice:** "you make the decisions"
**Notes:** User deferred all gray areas to Claude's discretion. All four areas resolved with Claude's recommended approaches.

---

## Mock tele.Context design

| Option | Description | Selected |
|--------|-------------|----------|
| Struct embedding + function fields | Embed tele.Context for defaults, override with function fields for inputs, capture slices for outputs | ✓ |
| Full interface re-implementation | Implement every tele.Context method manually | |
| Thin wrapper with real tele.Bot | Use a real bot instance with test server | |

**User's choice:** Deferred to Claude
**Notes:** Struct embedding chosen — matches TEST-01 requirement and minimizes boilerplate. Function fields for inputs allow per-test-case customization.

## Assertion granularity

| Option | Description | Selected |
|--------|-------------|----------|
| Substring checks | Use strings.Contains for key content markers | ✓ |
| Exact string match | Compare full response text exactly | |
| Regex patterns | Use regexp for flexible matching | |

**User's choice:** Deferred to Claude
**Notes:** Substring checks chosen — follows existing pattern in format_test.go, resilient to cosmetic changes.

## Scheduler test migration

| Option | Description | Selected |
|--------|-------------|----------|
| Add alongside | New mock-based tests added next to existing httptest tests | ✓ |
| Rewrite existing | Replace httptest tests with mock-based versions | |
| Replace selectively | Keep some httptest, replace others | |

**User's choice:** Deferred to Claude
**Notes:** Adding alongside preserves integration coverage while gaining deterministic mock-based tests.

## Coverage boundary

| Option | Description | Selected |
|--------|-------------|----------|
| Implicit coverage | Test helpers through the handlers that call them | ✓ |
| Dedicated unit tests | Separate test functions for guarded, findEndpoint, editEndpointList | |
| Mixed approach | Dedicated for complex helpers, implicit for simple ones | |

**User's choice:** Deferred to Claude
**Notes:** Implicit coverage chosen — guarded tested via wrong-chat-ID cases, findEndpoint via delete/interval lookup variants, editEndpointList via callback tests.

## Claude's Discretion

All four areas were deferred to Claude. Decisions captured in CONTEXT.md D-01 through D-13.

## Deferred Ideas

None — discussion stayed within phase scope.

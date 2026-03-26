# Phase 1: Interface Cleanup and Tech Debt - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-26
**Phase:** 01-interface-cleanup-and-tech-debt
**Areas discussed:** Status code strategy, Name validation rules, Checker interface design, Dead code audit scope

---

## Area Selection

| Option | Description | Selected |
|--------|-------------|----------|
| Status code strategy | QUAL-02 + QUAL-03: persist statusCode to DB or remove; notification wiring | ✓ |
| Name validation rules | QUAL-05: exact char set, case, leading/trailing chars, error UX | ✓ |
| Checker interface design | QUAL-01: interface at point of use, method signature | ✓ |
| Dead code audit scope | QUAL-04: focused sweep vs broad audit | ✓ |

**User's choice:** "Actually for now just choose sensible solutions for this. If unsure ask"
**Notes:** User delegated all decisions to Claude with instruction to ask if uncertain.

---

## Status Code Strategy

**Claude's decision:** Persist statusCode to new `last_status_code` DB column (goose migration). Formatter reads from endpoint model — shows "HTTP: 503" for non-zero, "connection error" for zero. No Notifier interface change.

**Alternatives considered:**
- Remove statusCode parameter entirely (simpler but loses useful diagnostic info)
- Pass statusCode through Notifier interface (requires interface change, less clean)

---

## Name Validation Rules

**Claude's decision:** `[a-zA-Z0-9_-]`, 1-50 chars, no leading/trailing hyphens, case-preserving.

**Alternatives considered:**
- Allow dots (rejected: URL-like, could cause confusion)
- Force lowercase (rejected: users may want readable names like "MyAPI")
- Allow spaces (rejected: complicates command parsing)

---

## Checker Interface Design

**Claude's decision:** `Checker` interface in `scheduler.go` with `Check(ctx, url) (int, error)`. Same signature as existing method.

**Alternatives considered:**
- Richer return type (e.g., `CheckResult` struct) — rejected: over-engineering for current needs, can evolve later
- Define in separate file — rejected: point-of-use convention is established

---

## Dead Code Audit Scope

**Claude's decision:** Focused sweep of exported symbols. `FormatFailureWithCode` becomes live after wiring. Remove anything else found unused.

**Alternatives considered:**
- Broad refactoring sweep — rejected: scope creep, save for future
- Skip audit entirely — rejected: QUAL-04 requires it

---

## Claude's Discretion

- Unification of FormatFailure/FormatFailureWithCode
- Migration file numbering
- Implementation order within phase
- Handling of any additional dead code found during sweep

# Roadmap: Noroshi

## Overview

Noroshi is a working, deployed uptime monitor with core functionality complete. This milestone transforms it from "works" to "portfolio-grade" by resolving tech debt that blocks testability, adding comprehensive test coverage, establishing CI, completing missing features, and finishing with professional documentation. The dependency chain is strict: clean interfaces before tests, deterministic tests before CI, CI before badges, features after validation infrastructure exists, docs last because they reference everything.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Interface Cleanup and Tech Debt** - Fix interface violations, wire dead code, remove unused parameters, add input validation
- [ ] **Phase 2: Bot Handler Tests** - Build mock infrastructure and comprehensive tests for all bot handlers and callbacks
- [ ] **Phase 3: CI Pipeline** - GitHub Actions workflow with parallel build/test and lint jobs, golangci-lint configuration
- [ ] **Phase 4: Feature Completion** - Live status checks, immediate check on add, structured logging with component-scoped loggers
- [ ] **Phase 5: Documentation and README** - Updated DESIGN.md, professional README with badges and architecture diagram, cleanup of stale files

## Phase Details

### Phase 1: Interface Cleanup and Tech Debt
**Goal**: Every interface follows point-of-use convention, every mock uses canonical error types, no exported symbols are dead code, and all user input is validated
**Depends on**: Nothing (first phase)
**Requirements**: QUAL-01, QUAL-02, QUAL-03, QUAL-04, QUAL-05
**Success Criteria** (what must be TRUE):
  1. Scheduler accepts any implementation of a Checker interface, not just *HTTPChecker
  2. Failure notifications display the HTTP status code when a check fails with a non-zero status
  3. The statusCode parameter is either persisted to a database column or removed from all store method signatures
  4. No exported function or type exists that is unused by any caller in the codebase
  5. Endpoint names are validated on creation: only alphanumeric characters, hyphens, and underscores are accepted, with a maximum length of 50
**Plans:** 2 plans
Plans:
- [x] 01-01-PLAN.md -- Checker interface extraction and last_status_code storage persistence
- [x] 01-02-PLAN.md -- Status code notification wiring, dead code sweep, name validation

### Phase 2: Bot Handler Tests
**Goal**: Every bot command handler and callback handler has table-driven tests covering success and error paths, built on shared mock infrastructure
**Depends on**: Phase 1
**Requirements**: TEST-01, TEST-02, TEST-03, TEST-04
**Success Criteria** (what must be TRUE):
  1. A shared mock_test.go exists in internal/bot/ with mockContext (struct-embedding tele.Context), mockStore, and mockScheduler using function-field pattern
  2. Every bot command handler (/add, /delete, /list, /interval, /help) has table-driven tests covering both happy path and error paths
  3. Every bot callback handler (detail view, refresh, back, delete confirmation) has table-driven tests covering both happy path and error paths
  4. Scheduler tests use mock Checker interface instead of real HTTP servers, making all tests deterministic
**Plans:** 2 plans
Plans:
- [x] 02-01-PLAN.md -- Mock infrastructure and command handler tests (TEST-01, TEST-02)
- [x] 02-02-PLAN.md -- Callback handler tests and mock-based scheduler tests (TEST-03, TEST-04)

### Phase 3: CI Pipeline
**Goal**: Every push to develop and main is automatically built, vetted, tested, and linted, with all checks passing green
**Depends on**: Phase 2
**Requirements**: CICD-01, CICD-02, CICD-03, CICD-04
**Success Criteria** (what must be TRUE):
  1. A GitHub Actions workflow runs on every push to develop and main with parallel lint and build-and-test jobs
  2. golangci-lint v2 runs with Go-specific linters (contextcheck, bodyclose, sqlclosecheck, errorlint, sloglint) and the existing codebase passes clean
  3. CGO_ENABLED=0 go build ./cmd/monitor/ succeeds in CI
  4. go vet ./... passes in CI with zero warnings
**Plans:** 1/2 plans executed
Plans:
- [x] 03-01-PLAN.md -- Fix errorlint violations and create golangci-lint v2 config (CICD-02)
- [x] 03-02-PLAN.md -- GitHub Actions CI workflow with parallel jobs (CICD-01, CICD-03, CICD-04)

### Phase 4: Feature Completion
**Goal**: Users can trigger live health checks on demand, see immediate results when adding endpoints, and the codebase uses consistent structured logging
**Depends on**: Phase 3
**Requirements**: FEAT-01, FEAT-02, FEAT-03, FEAT-04
**Success Criteria** (what must be TRUE):
  1. User can send /status and receive a reply showing live health check results for every monitored endpoint
  2. When a user adds a new endpoint via /add, an immediate health check runs and the result is shown in the reply
  3. Every component (Scheduler, Bot, HTTPChecker) logs with a component-scoped logger injected via its constructor
  4. All slog calls across the codebase use consistent typed fields (endpoint_id, endpoint_url, endpoint_name, command, chat_id)
**Plans**: TBD

### Phase 5: Documentation and README
**Goal**: A portfolio visitor can understand the project's architecture, set it up, and deploy it from the README alone, with all documentation reflecting the current state of the code
**Depends on**: Phase 4
**Requirements**: DOCS-01, DOCS-02, DOCS-03, DOCS-04, DOCS-05, DOCS-06
**Success Criteria** (what must be TRUE):
  1. DESIGN.md accurately reflects current project structure, command syntax, Store interface, and message formats
  2. README displays CI status badge, Go Report Card badge, Go Reference badge, and license badge -- all resolving correctly with CI green
  3. README includes a Mermaid architecture diagram showing component relationships that renders on GitHub
  4. README contains clear setup instructions (docker run), configuration table (env vars), command reference, and deployment docs (Docker, Compose, Coolify)
  5. TODO.md is either deleted or cleaned to contain only accurate, current items; .env.example reflects current /add syntax
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4 -> 5

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Interface Cleanup and Tech Debt | 0/2 | Planning complete | - |
| 2. Bot Handler Tests | 0/2 | Planning complete | - |
| 3. CI Pipeline | 1/2 | In Progress|  |
| 4. Feature Completion | 0/? | Not started | - |
| 5. Documentation and README | 0/? | Not started | - |

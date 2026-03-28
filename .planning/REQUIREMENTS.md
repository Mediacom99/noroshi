# Requirements: Noroshi

**Defined:** 2026-03-26
**Core Value:** Reliable uptime monitoring with zero-friction setup — one Docker container, one Telegram bot, no dashboards to maintain.

## v1 Requirements

### Code Quality

- [x] **QUAL-01**: Scheduler depends on a Checker interface, not concrete `*HTTPChecker`
- [x] **QUAL-02**: HTTP status code flows through notification pipeline — `NotifyFailure` receives status code and uses `FormatFailureWithCode` for non-zero codes
- [x] **QUAL-03**: Store methods either persist `statusCode` to a `last_status_code` DB column or remove the unused parameter from signatures
- [x] **QUAL-04**: Dead code removed — unused functions, stale references cleaned up
- [x] **QUAL-05**: Endpoint name validation enforced (alphanumeric, hyphens, underscores, max 50 chars)

### Testing

- [ ] **TEST-01**: Shared mock infrastructure for `tele.Context` interface using struct embedding pattern in `internal/bot/mock_test.go`
- [ ] **TEST-02**: Table-driven tests for all bot command handlers (`/add`, `/delete`, `/list`, `/interval`, `/help`)
- [x] **TEST-03**: Tests for bot callback handlers (detail view, refresh, back, delete confirmation)
- [x] **TEST-04**: Scheduler tests use mock Checker interface instead of real HTTP servers

### CI/CD

- [ ] **CICD-01**: GitHub Actions workflow with parallel lint and test jobs
- [x] **CICD-02**: golangci-lint v2 configuration with Go-specific linters (contextcheck, bodyclose, sqlclosecheck, errorlint, sloglint)
- [ ] **CICD-03**: Build verification with `CGO_ENABLED=0 go build ./cmd/monitor/`
- [ ] **CICD-04**: `go vet ./...` passes in CI

### Features

- [ ] **FEAT-01**: `/status` command triggers live health checks on all endpoints and replies with results
- [ ] **FEAT-02**: Adding an endpoint via `/add` performs an immediate health check and shows the result to the user
- [ ] **FEAT-03**: Structured logging with component-scoped loggers injected via constructors (`slog.With("component", "...")`)
- [ ] **FEAT-04**: Consistent slog fields across all packages (endpoint_id, endpoint_url, endpoint_name, command, chat_id)

### Documentation

- [ ] **DOCS-01**: DESIGN.md updated to match current implementation (project structure, command syntax, Store interface, message formats)
- [ ] **DOCS-02**: Professional README with CI status badge, Go Report Card badge, Go Reference badge, license badge
- [ ] **DOCS-03**: README includes Mermaid architecture diagram showing component relationships
- [ ] **DOCS-04**: README has clear setup, configuration, and deployment sections
- [ ] **DOCS-05**: TODO.md cleaned up — completed items removed, remaining items accurate
- [ ] **DOCS-06**: .env.example and command documentation in README reflect current `/add <name> <url> [interval]` syntax

## v2 Requirements

Deferred to future milestone. Tracked but not in current roadmap.

### Monitoring Enhancements

- **FEAT-10**: Pause/resume endpoint monitoring for maintenance windows
- **FEAT-11**: SSL certificate expiry monitoring with configurable alert thresholds
- **FEAT-12**: Check history table (`check_log`) storing status, response time, and timestamp per check
- **FEAT-13**: Uptime percentage calculation over 24h/7d/30d windows
- **FEAT-14**: Response time tracking and display in detail view
- **FEAT-15**: Expected status code configuration per endpoint (not all healthy endpoints return 200)
- **FEAT-16**: Response time threshold alerts

### Advanced Features

- **FEAT-20**: Daily/weekly uptime digest message via scheduled gocron job
- **FEAT-21**: Response time sparkline in Telegram detail view using Unicode block characters
- **FEAT-22**: Keyword/body content monitoring (verify response body contains expected string)
- **FEAT-23**: Incident timeline (`/incidents <name>` showing state transitions)
- **FEAT-24**: Config export/backup via `/export` command (JSON document via Telegram)

## Out of Scope

| Feature | Reason |
|---------|--------|
| Web dashboard / status page | Contradicts "Telegram-only" core identity — the project stands out by NOT having one |
| Multi-user support / RBAC | Chat ID guard is appropriate for personal/small-team use |
| Non-HTTP monitor types (TCP, DNS, ICMP) | Stay focused on HTTP/HTTPS — depth over breadth |
| PostgreSQL / MySQL support | SQLite handles the expected scale; adding DB drivers violates no-new-deps constraint |
| OAuth / advanced authentication | Single chat ID is sufficient |
| Coverage badge in CI | Overhead not justified for this project size |
| Rate limiting on commands | Low risk for single-user/small-group use |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| QUAL-01 | Phase 1 | Complete |
| QUAL-02 | Phase 1 | Complete |
| QUAL-03 | Phase 1 | Complete |
| QUAL-04 | Phase 1 | Complete |
| QUAL-05 | Phase 1 | Complete |
| TEST-01 | Phase 2 | Pending |
| TEST-02 | Phase 2 | Pending |
| TEST-03 | Phase 2 | Complete |
| TEST-04 | Phase 2 | Complete |
| CICD-01 | Phase 3 | Pending |
| CICD-02 | Phase 3 | Complete |
| CICD-03 | Phase 3 | Pending |
| CICD-04 | Phase 3 | Pending |
| FEAT-01 | Phase 4 | Pending |
| FEAT-02 | Phase 4 | Pending |
| FEAT-03 | Phase 4 | Pending |
| FEAT-04 | Phase 4 | Pending |
| DOCS-01 | Phase 5 | Pending |
| DOCS-02 | Phase 5 | Pending |
| DOCS-03 | Phase 5 | Pending |
| DOCS-04 | Phase 5 | Pending |
| DOCS-05 | Phase 5 | Pending |
| DOCS-06 | Phase 5 | Pending |

**Coverage:**
- v1 requirements: 23 total
- Mapped to phases: 23
- Unmapped: 0

---
*Requirements defined: 2026-03-26*
*Last updated: 2026-03-26 after roadmap creation*

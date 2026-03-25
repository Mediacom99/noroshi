# Testing Patterns

**Analysis Date:** 2026-03-26

## Test Framework

**Runner:**
- Go stdlib `testing` package only
- No testify, gomock, or other third-party test libraries

**Assertion Library:**
- None -- use `t.Errorf`, `t.Fatalf`, `t.Fatal`, `t.Error` directly
- Use `t.Fatalf` when failure should stop the test (e.g., setup failures, nil checks before dereference)
- Use `t.Errorf` for non-fatal assertions that should continue

**Run Commands:**
```bash
go test ./...                  # Run all tests
go test ./internal/storage/    # Run tests for a specific package
go test -v ./...               # Verbose output
go test -run TestName ./...    # Run specific test
```

**Pre-commit requirement:** `go test ./...` must pass before every commit (alongside `go vet ./...` and `CGO_ENABLED=0 go build ./cmd/monitor/`).

## Test File Organization

**Location:** Co-located with source files in the same package

**Naming:** `{source_file}_test.go` -- test file mirrors the source file name
- `apperror.go` -> `apperror_test.go`
- `store.go` -> `store_test.go`
- `checker.go` -> `checker_test.go`
- `scheduler.go` -> `scheduler_test.go`
- `format.go` -> `format_test.go`
- `validate.go` -> `validate_test.go`
- `config.go` -> `config_test.go`

**Package declaration:** Tests use the same package (not `_test` suffix), allowing access to unexported functions.

## Test Structure

**Individual Test Functions:**
- Named `Test{FunctionName}{Scenario}` (e.g., `TestCheckerOK`, `TestChecker503`, `TestGetEndpointNotFound`)
- Setup at top, action in middle, assertions at bottom

```go
func TestCheckerOK(t *testing.T) {
    // Setup
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    defer srv.Close()

    checker := NewHTTPChecker(5 * time.Second)

    // Action
    code, err := checker.Check(context.Background(), srv.URL)

    // Assert
    if err != nil {
        t.Fatalf("Check: %v", err)
    }
    if code != 200 {
        t.Errorf("code = %d, want 200", code)
    }
}
```

**Table-Driven Tests:**
- Used for functions with multiple input/output combinations
- Anonymous struct slice with `name`, input fields, and `want`/`wantErr` fields
- Iterate with `for _, tt := range tests` and `t.Run(tt.name, ...)`

```go
func TestFormatDuration(t *testing.T) {
    tests := []struct {
        name string
        d    time.Duration
        want string
    }{
        {"zero", 0, "0s"},
        {"seconds only", 45 * time.Second, "45s"},
        {"minutes and seconds", 12*time.Minute + 34*time.Second, "12m 34s"},
        // ...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := FormatDuration(tt.d)
            if got != tt.want {
                t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
            }
        })
    }
}
```

```go
func TestValidateURL(t *testing.T) {
    tests := []struct {
        name    string
        url     string
        wantErr bool
    }{
        {"valid https", "https://example.com", false},
        {"empty string", "", true},
        {"ftp scheme", "ftp://example.com", true},
        // ...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateURL(tt.url)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
            }
        })
    }
}
```

**Substring Checks for Formatted Output:**
- For testing HTML-formatted messages, use `strings.Contains` checks with labeled assertions

```go
checks := []struct {
    label    string
    contains string
}{
    {"header", "<b>Endpoint Down</b>"},
    {"name", "prod-api"},
    {"url", "<code>https://api.example.com/health</code>"},
}
for _, c := range checks {
    if !strings.Contains(msg, c.contains) {
        t.Errorf("should contain %s: %q", c.label, c.contains)
    }
}
```

## Mocking

**Framework:** Hand-written mocks (no gomock, no mockgen)

**Pattern:** Define mock structs in test files that implement the consumer-side interfaces.

**Mock Store** (in `internal/monitor/scheduler_test.go`):
```go
type mockStore struct {
    mu        sync.Mutex
    endpoints map[int64]storage.Endpoint
}

func newMockStore() *mockStore {
    return &mockStore{endpoints: make(map[int64]storage.Endpoint)}
}

func (m *mockStore) SetEndpoint(ep storage.Endpoint) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.endpoints[ep.ID] = ep
}

func (m *mockStore) GetEndpoint(_ context.Context, id int64) (storage.Endpoint, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    ep, ok := m.endpoints[id]
    if !ok {
        return storage.Endpoint{}, &notFoundError{}
    }
    return ep, nil
}
// ... additional methods matching the Store interface
```

**Mock Notifier** (in `internal/monitor/scheduler_test.go`):
```go
type mockNotifier struct {
    mu         sync.Mutex
    failures   []storage.Endpoint
    recoveries []recoveryCall
}

func (n *mockNotifier) NotifyFailure(_ context.Context, ep storage.Endpoint) error {
    n.mu.Lock()
    defer n.mu.Unlock()
    n.failures = append(n.failures, ep)
    return nil
}

// Counter methods for assertions
func (n *mockNotifier) failureCount() int {
    n.mu.Lock()
    defer n.mu.Unlock()
    return len(n.failures)
}
```

**Key Mock Patterns:**
- Mocks use `sync.Mutex` for goroutine safety (scheduler runs async)
- Helper method `SetEndpoint` to seed test data
- Counter methods (`failureCount()`, `recoveryCount()`) for assertion readability
- Context parameter ignored with `_` in mock methods
- Mocks implement only the interface methods needed by the consumer

**What to Mock:**
- Store interfaces when testing business logic (scheduler, bot handlers)
- Notifier interface when testing scheduling logic

**What NOT to Mock:**
- The HTTP layer in checker tests -- use `httptest.NewServer` instead
- The actual SQLite database in store tests -- use in-memory SQLite

## Fixtures and Factories

**Test Database Helper** (in `internal/storage/store_test.go`):
```go
func testDB(t *testing.T) *sql.DB {
    t.Helper()
    db, err := sql.Open("sqlite", ":memory:")
    if err != nil {
        t.Fatalf("open in-memory db: %v", err)
    }
    t.Cleanup(func() { db.Close() })

    if err := RunMigrations(db); err != nil {
        t.Fatalf("run migrations: %v", err)
    }
    return db
}
```

**Environment Helper** (in `internal/config/config_test.go`):
```go
func setEnv(t *testing.T, vars map[string]string) {
    t.Helper()
    for k, v := range vars {
        t.Setenv(k, v)
    }
}
```

**Test Data:**
- Endpoint structs constructed inline with meaningful field values
- Use recognizable names: `"prod-api"`, `"site-a"`, `"site-b"`
- Use realistic URLs: `"https://example.com"`, `"https://api.example.com/health"`

**HTTP Test Servers:**
```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
}))
defer srv.Close()
```

## Coverage

**Requirements:** No formal coverage threshold enforced. All non-main packages must have test files (per `CLAUDE.md`).

**Exception:** `internal/bot/` handlers and callbacks have no tests (requires Telegram API). Only `format.go` and `validate.go` within `bot` are tested since they are pure functions.

**View Coverage:**
```bash
go test -cover ./...
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

## Test Types

**Unit Tests:**
- All tests are unit tests
- Pure function tests: `format_test.go`, `validate_test.go`, `apperror_test.go`, `config_test.go`
- Mock-based logic tests: `scheduler_test.go` (mocks store and notifier, uses real HTTP server)
- Real-database tests: `store_test.go` (in-memory SQLite with real migrations)
- Real-HTTP tests: `checker_test.go` (httptest servers)

**Integration Tests:**
- `store_test.go` acts as an integration test -- real SQLite + real goose migrations
- `scheduler_test.go` is a partial integration test -- real HTTP checker + real httptest servers + mock store/notifier

**E2E Tests:**
- Not used

## Coverage Map

| Package | Test File | What's Tested | Gaps |
|---------|-----------|---------------|------|
| `internal/apperror` | `apperror_test.go` | Wrap, Is, Unwrap, Error formatting, sentinel comparison | Full coverage |
| `internal/bot` | `format_test.go` | FormatDuration, FormatFailure, FormatFailureWithCode, FormatRecovery, FormatEndpointList, FormatEndpointDetail, FormatHelp, HTML escaping | Bot handlers (`handlers.go`), callbacks (`callbacks.go`), `bot.go` lifecycle, `SendMessage`, `SendSilentMessage`, `TelegramNotifier` |
| `internal/bot` | `validate_test.go` | ValidateURL with valid/invalid inputs | Full coverage of validate.go |
| `internal/config` | `config_test.go` | Full config load, defaults, missing required vars, invalid values | Missing: invalid `MAX_FAILURE_NOTIFICATIONS`, invalid `HEALTH_PORT` |
| `internal/monitor` | `checker_test.go` | HTTP 200, HTTP 503, unreachable server, cancelled context | Full coverage of checker.go |
| `internal/monitor` | `scheduler_test.go` | checkAndNotify OK, failure, failure cap, recovery, no-recovery-when-ok | `Add`, `Remove`, `Start`, `Shutdown` methods; NewScheduler error path |
| `internal/storage` | `store_test.go` | CRUD operations, duplicate detection, not-found errors, migrations, RecordFailure/Recovery, interval update, lookup by URL/name | Full coverage of store.go |
| `cmd/monitor` | (none) | N/A | `main.go` not tested (acceptable -- wiring only) |

## Common Patterns

**Async/Concurrent Testing:**
- Mocks use `sync.Mutex` because `checkAndNotify` may run in goroutines
- Tests call `checkAndNotify` directly (synchronous) to avoid timing issues
- No `time.Sleep` in tests

**Error Testing:**
```go
func TestGetEndpointNotFound(t *testing.T) {
    db := testDB(t)
    store := NewSQLiteStore(db)
    ctx := context.Background()

    _, err := store.GetEndpoint(ctx, 999)
    if !errors.Is(err, apperror.ErrNotFound) {
        t.Fatalf("expected ErrNotFound, got: %v", err)
    }
}
```

**Stateful Sequence Testing:**
- Test state transitions by calling methods in sequence and asserting intermediate states
```go
// Failure then recovery
sched.checkAndNotify(1)  // fails
isDown = false
sched.checkAndNotify(1)  // recovers

if notifier.recoveryCount() != 1 {
    t.Errorf("recovery notifications = %d, want 1", notifier.recoveryCount())
}
```

**Toggle Server Behavior:**
```go
isDown := true
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if isDown {
        w.WriteHeader(http.StatusServiceUnavailable)
    } else {
        w.WriteHeader(http.StatusOK)
    }
}))
```

**Cleanup:**
- Use `defer srv.Close()` for httptest servers
- Use `t.Cleanup(func() { db.Close() })` for database connections
- Use `t.Setenv()` for environment variables (auto-restored after test)

---

*Testing analysis: 2026-03-26*

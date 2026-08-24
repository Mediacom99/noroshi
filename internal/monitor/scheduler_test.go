package monitor

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"noroshi/internal/storage"
)

// mockStore implements Store for testing.
type mockStore struct {
	mu             sync.Mutex
	endpoints      map[int64]storage.Endpoint
	recordedChecks int
	inMaintenance  bool
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

func (m *mockStore) UpdateEndpointStatus(_ context.Context, id int64, o storage.CheckOutcome) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return &notFoundError{}
	}
	ep.Status = o.Status
	ep.LastStatusCode = o.StatusCode
	ep.LastLatencyMs = o.LatencyMs
	ep.LastCheckError = o.Reason
	ep.LastCheckedAt = sql.NullTime{Time: time.Now(), Valid: true}
	m.endpoints[id] = ep
	return nil
}

func (m *mockStore) RecordFailure(_ context.Context, id int64, _ storage.CheckOutcome, maxNotifications int, failureThreshold int) (storage.Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return storage.Endpoint{}, &notFoundError{}
	}
	ep.ConsecutiveFailures++
	if ep.FailureNotificationsSent < maxNotifications && ep.ConsecutiveFailures >= failureThreshold {
		ep.FailureNotificationsSent++
		ep.LastNotifiedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}
	ep.Status = "not_ok"
	if ep.ConsecutiveFailures == 1 {
		ep.LastFailureAt = sql.NullTime{Time: time.Now(), Valid: true}
	}
	ep.LastCheckedAt = sql.NullTime{Time: time.Now(), Valid: true}
	m.endpoints[id] = ep
	return ep, nil
}

func (m *mockStore) RecordRecovery(_ context.Context, id int64, _ storage.CheckOutcome) (storage.Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return storage.Endpoint{}, &notFoundError{}
	}
	result := ep // preserve LastFailureAt
	ep.Status = "ok"
	ep.ConsecutiveFailures = 0
	ep.FailureNotificationsSent = 0
	ep.LastFailureAt = sql.NullTime{}
	ep.LastCheckedAt = sql.NullTime{Time: time.Now(), Valid: true}
	m.endpoints[id] = ep
	result.Status = "ok"
	return result, nil
}

func (m *mockStore) ListExpiredPauses(_ context.Context, now time.Time) ([]storage.Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []storage.Endpoint
	for _, ep := range m.endpoints {
		if ep.Paused && ep.PausedUntil.Valid && !ep.PausedUntil.Time.After(now) {
			out = append(out, ep)
		}
	}
	return out, nil
}

func (m *mockStore) SetEndpointPaused(_ context.Context, id int64, paused bool, until sql.NullTime) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return &notFoundError{}
	}
	ep.Paused = paused
	ep.PausedUntil = until
	m.endpoints[id] = ep
	return nil
}

func (m *mockStore) SetAlertMessageID(_ context.Context, id int64, messageID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return &notFoundError{}
	}
	ep.AlertMessageID = messageID
	m.endpoints[id] = ep
	return nil
}

func (m *mockStore) RecordCheck(_ context.Context, endpointID int64, up bool, statusCode int, latencyMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordedChecks++
	return nil
}

func (m *mockStore) PruneChecks(_ context.Context, olderThan time.Time) (int64, error) {
	return 0, nil
}

func (m *mockStore) IsInMaintenance(_ context.Context, _ int64, _ time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inMaintenance, nil
}

func (m *mockStore) ListEndpoints(_ context.Context) ([]storage.Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	endpoints := make([]storage.Endpoint, 0, len(m.endpoints))
	for _, ep := range m.endpoints {
		endpoints = append(endpoints, ep)
	}
	return endpoints, nil
}

func (m *mockStore) GetCheckStats(_ context.Context, endpointID int64, since time.Time) (storage.WindowStats, error) {
	return storage.WindowStats{Total: 10, Up: 9, AvgLatencyMs: 120, Incidents: 1}, nil
}

func (m *mockStore) TouchCertWarning(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return &notFoundError{}
	}
	ep.LastCertWarningAt = sql.NullTime{Time: time.Now(), Valid: true}
	m.endpoints[id] = ep
	return nil
}

func (m *mockStore) TouchLastNotified(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return &notFoundError{}
	}
	ep.LastNotifiedAt = sql.NullTime{Time: time.Now(), Valid: true}
	m.endpoints[id] = ep
	return nil
}

type notFoundError struct{}

func (e *notFoundError) Error() string { return "not found" }

// mockNotifier records notification calls.
type mockNotifier struct {
	mu           sync.Mutex
	failures     []storage.Endpoint
	recoveries   []recoveryCall
	certWarnings int
	digests      []string
}

type recoveryCall struct {
	Endpoint storage.Endpoint
	Downtime time.Duration
}

func (n *mockNotifier) NotifyFailure(_ context.Context, ep storage.Endpoint) (int64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failures = append(n.failures, ep)
	return int64(len(n.failures)), nil
}

func (n *mockNotifier) NotifyCertExpiry(_ context.Context, ep storage.Endpoint, daysLeft int) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.certWarnings++
	return nil
}

func (n *mockNotifier) NotifyRecovery(_ context.Context, ep storage.Endpoint, downtime time.Duration) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.recoveries = append(n.recoveries, recoveryCall{ep, downtime})
	return nil
}

func (n *mockNotifier) NotifyDigest(_ context.Context, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.digests = append(n.digests, text)
	return nil
}

func (n *mockNotifier) digestCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.digests)
}

func (n *mockNotifier) failureCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.failures)
}

func (n *mockNotifier) recoveryCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.recoveries)
}

// mockChecker implements Checker for deterministic testing without HTTP.
type mockChecker struct {
	checkFn func(ctx context.Context, url string, opts CheckOptions) CheckResult
}

func (m *mockChecker) Check(ctx context.Context, url string, opts CheckOptions) CheckResult {
	return m.checkFn(ctx, url, opts)
}

func newMockScheduler(t *testing.T, store *mockStore, checker *mockChecker, notifier *mockNotifier, maxFail int) *Scheduler {
	t.Helper()
	sched, err := NewScheduler(context.Background(), store, checker, notifier, maxFail, 1, 0, DigestConfig{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return sched
}

func TestCheckAndNotifyMockOK(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{Up: true, StatusCode: 200, Latency: 10 * time.Millisecond}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)
	sched.checkAndNotify(1)

	if notifier.failureCount() != 0 {
		t.Error("should not notify failure when endpoint stays OK")
	}
	if notifier.recoveryCount() != 0 {
		t.Error("should not notify recovery when endpoint was already OK")
	}

	ep, _ := store.GetEndpoint(context.Background(), 1)
	if ep.Status != "ok" {
		t.Errorf("status = %q, want ok", ep.Status)
	}
}

func TestCheckAndNotifyMockFailure(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{Up: false, StatusCode: 503, Latency: 10 * time.Millisecond, Reason: "HTTP 503"}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)
	sched.checkAndNotify(1)

	if notifier.failureCount() != 1 {
		t.Errorf("failure notifications = %d, want 1", notifier.failureCount())
	}
}

func TestCheckAndNotifyMockConnectionError(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{Up: false, Reason: "connection error", Err: fmt.Errorf("connection refused")}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)
	sched.checkAndNotify(1)

	if notifier.failureCount() != 1 {
		t.Errorf("failure notifications = %d, want 1 (connection error is a failure)", notifier.failureCount())
	}
}

func TestCheckAndNotifyMockFailureCap(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{Up: false, StatusCode: 500, Latency: 10 * time.Millisecond, Reason: "HTTP 500"}
		},
	}

	maxNotifications := 3
	sched := newMockScheduler(t, store, checker, notifier, maxNotifications)

	for range 5 {
		sched.checkAndNotify(1)
	}

	if notifier.failureCount() != maxNotifications {
		t.Errorf("failure notifications = %d, want %d (capped)", notifier.failureCount(), maxNotifications)
	}
}

func TestCheckAndNotifyMockRecovery(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}

	returnFailure := true
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			if returnFailure {
				return CheckResult{Up: false, StatusCode: 503, Latency: 10 * time.Millisecond, Reason: "HTTP 503"}
			}
			return CheckResult{Up: true, StatusCode: 200, Latency: 10 * time.Millisecond}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)

	// First call: failure
	sched.checkAndNotify(1)

	// Second call: recovery
	returnFailure = false
	sched.checkAndNotify(1)

	if notifier.recoveryCount() != 1 {
		t.Errorf("recovery notifications = %d, want 1", notifier.recoveryCount())
	}
}

func TestCheckAndNotifyOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: srv.URL, IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := NewChecker(5 * time.Second)

	sched, err := NewScheduler(context.Background(), store, checker, notifier, 3, 1, 0, DigestConfig{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	sched.checkAndNotify(1)

	if notifier.failureCount() != 0 {
		t.Error("should not notify when endpoint stays OK")
	}
	if notifier.recoveryCount() != 0 {
		t.Error("should not notify recovery when endpoint was already OK")
	}

	ep, _ := store.GetEndpoint(context.Background(), 1)
	if ep.Status != "ok" {
		t.Errorf("status = %q, want ok", ep.Status)
	}
}

func TestCheckAndNotifyFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: srv.URL, IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := NewChecker(5 * time.Second)

	sched, err := NewScheduler(context.Background(), store, checker, notifier, 3, 1, 0, DigestConfig{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	sched.checkAndNotify(1)

	if notifier.failureCount() != 1 {
		t.Errorf("failure notifications = %d, want 1", notifier.failureCount())
	}
}

func TestCheckAndNotifyFailureCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: srv.URL, IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := NewChecker(5 * time.Second)

	maxNotifications := 3
	sched, err := NewScheduler(context.Background(), store, checker, notifier, maxNotifications, 1, 0, DigestConfig{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	for range 5 {
		sched.checkAndNotify(1)
	}

	if notifier.failureCount() != maxNotifications {
		t.Errorf("failure notifications = %d, want %d (capped)", notifier.failureCount(), maxNotifications)
	}
}

func TestCheckAndNotifyRecovery(t *testing.T) {
	isDown := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isDown {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: srv.URL, IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := NewChecker(5 * time.Second)

	sched, err := NewScheduler(context.Background(), store, checker, notifier, 3, 1, 0, DigestConfig{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	sched.checkAndNotify(1)

	isDown = false
	sched.checkAndNotify(1)

	if notifier.recoveryCount() != 1 {
		t.Errorf("recovery notifications = %d, want 1", notifier.recoveryCount())
	}
}

func TestCheckAndNotifyNoRecoveryWhenAlreadyOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: srv.URL, IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := NewChecker(5 * time.Second)

	sched, err := NewScheduler(context.Background(), store, checker, notifier, 3, 1, 0, DigestConfig{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	sched.checkAndNotify(1)
	sched.checkAndNotify(1)

	if notifier.recoveryCount() != 0 {
		t.Error("should not send recovery when endpoint was already OK")
	}
}

func TestCheckAndNotifyMock2xxIsOK(t *testing.T) {
	for _, code := range []int{200, 201, 204, 299} {
		store := newMockStore()
		store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
		notifier := &mockNotifier{}
		checker := &mockChecker{
			checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
				return CheckResult{Up: true, StatusCode: code, Latency: 10 * time.Millisecond}
			},
		}

		sched := newMockScheduler(t, store, checker, notifier, 3)
		sched.checkAndNotify(1)

		if notifier.failureCount() != 0 {
			t.Errorf("status %d: should not notify failure for 2xx", code)
		}
		ep, _ := store.GetEndpoint(context.Background(), 1)
		if ep.Status != "ok" {
			t.Errorf("status %d: endpoint status = %q, want ok", code, ep.Status)
		}
	}
}

func TestCheckAndNotifyMockRedirectIsFailure(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{Up: false, StatusCode: 301, Latency: 10 * time.Millisecond, Reason: "HTTP 301"}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)
	sched.checkAndNotify(1)

	if notifier.failureCount() != 1 {
		t.Errorf("failure notifications = %d, want 1 (3xx is not a success)", notifier.failureCount())
	}
}

func TestSchedulerSingletonMode(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 1, Status: "ok"})
	notifier := &mockNotifier{}

	var mu sync.Mutex
	running := 0
	maxConcurrent := 0
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			mu.Lock()
			running++
			if running > maxConcurrent {
				maxConcurrent = running
			}
			mu.Unlock()

			// Slower than the 1s job interval — without singleton mode the
			// next run would start while this one is still in flight.
			time.Sleep(1200 * time.Millisecond)

			mu.Lock()
			running--
			mu.Unlock()
			return CheckResult{Up: true, StatusCode: 200, Latency: 10 * time.Millisecond}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)
	if err := sched.Add(context.Background(), storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 1}); err != nil {
		t.Fatal(err)
	}
	sched.Start()
	time.Sleep(2500 * time.Millisecond)
	if err := sched.Shutdown(); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if maxConcurrent > 1 {
		t.Errorf("max concurrent checks = %d, want 1 (singleton mode)", maxConcurrent)
	}
}

func TestCheckNow(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}

	fail := false
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			if fail {
				return CheckResult{Up: false, StatusCode: 503, Latency: 10 * time.Millisecond, Reason: "HTTP 503"}
			}
			return CheckResult{Up: true, StatusCode: 200, Latency: 10 * time.Millisecond}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)

	// OK check: status stays ok, no notifications, no counter changes.
	ep, err := sched.CheckNow(context.Background(), 1)
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if ep.Status != "ok" {
		t.Errorf("status = %q, want ok", ep.Status)
	}
	if notifier.failureCount() != 0 || notifier.recoveryCount() != 0 {
		t.Error("CheckNow must not send notifications")
	}

	// Failing ad-hoc check: status flips to not_ok but the failure/recovery
	// state machine (counters) is left to the scheduled jobs.
	fail = true
	ep, err = sched.CheckNow(context.Background(), 1)
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if ep.Status != "not_ok" {
		t.Errorf("status = %q, want not_ok", ep.Status)
	}
	if ep.ConsecutiveFailures != 0 || ep.FailureNotificationsSent != 0 {
		t.Errorf("CheckNow must not touch failure counters, got consecutive=%d notifications=%d",
			ep.ConsecutiveFailures, ep.FailureNotificationsSent)
	}
	if notifier.failureCount() != 0 {
		t.Error("CheckNow must not send failure notifications")
	}

	// Recovery via ad-hoc check resets failure state.
	fail = false
	ep, err = sched.CheckNow(context.Background(), 1)
	if err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if ep.Status != "ok" {
		t.Errorf("status = %q, want ok", ep.Status)
	}
	if notifier.recoveryCount() != 0 {
		t.Error("CheckNow must not send recovery notifications")
	}
}

func TestCheckAndNotifyFailureThreshold(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{Up: false, StatusCode: 500, Latency: 10 * time.Millisecond, Reason: "HTTP 500"}
		},
	}

	sched, err := NewScheduler(context.Background(), store, checker, notifier, 3, 3, 0, DigestConfig{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	// Failures 1 and 2: below threshold, no notification.
	sched.checkAndNotify(1)
	sched.checkAndNotify(1)
	if notifier.failureCount() != 0 {
		t.Errorf("failure notifications = %d, want 0 (below threshold)", notifier.failureCount())
	}

	// Failure 3: threshold reached, first notification.
	sched.checkAndNotify(1)
	if notifier.failureCount() != 1 {
		t.Errorf("failure notifications = %d, want 1 (threshold reached)", notifier.failureCount())
	}

	// Failures 4 and 5: notifications 2 and 3 (cap).
	sched.checkAndNotify(1)
	sched.checkAndNotify(1)
	if notifier.failureCount() != 3 {
		t.Errorf("failure notifications = %d, want 3 (cap)", notifier.failureCount())
	}

	// Failure 6: cap reached, no more notifications.
	sched.checkAndNotify(1)
	if notifier.failureCount() != 3 {
		t.Errorf("failure notifications = %d, want 3 (capped)", notifier.failureCount())
	}
}

func TestCheckAndNotifyPaused(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok", Paused: true})
	notifier := &mockNotifier{}

	called := false
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			called = true
			return CheckResult{Up: false, StatusCode: 500, Latency: 10 * time.Millisecond, Reason: "HTTP 500"}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)
	sched.checkAndNotify(1)

	if called {
		t.Error("checker must not be called for a paused endpoint")
	}
	ep, _ := store.GetEndpoint(context.Background(), 1)
	if ep.ConsecutiveFailures != 0 {
		t.Error("paused endpoint must not record failures")
	}
}

func TestCheckAndNotifyReminder(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{Up: false, StatusCode: 500, Latency: 10 * time.Millisecond, Reason: "HTTP 500"}
		},
	}

	// max 1 notification, reminder every nanosecond (fires on next check).
	sched, err := NewScheduler(context.Background(), store, checker, notifier, 1, 1, time.Nanosecond, DigestConfig{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	sched.checkAndNotify(1) // first alert (cap reached)
	if notifier.failureCount() != 1 {
		t.Fatalf("failure notifications = %d, want 1", notifier.failureCount())
	}

	sched.checkAndNotify(1) // cap reached → reminder
	if notifier.failureCount() != 2 {
		t.Errorf("failure notifications = %d, want 2 (reminder sent)", notifier.failureCount())
	}
}

func TestCheckAndNotifyNoReminderWhenDisabled(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{Up: false, StatusCode: 500, Latency: 10 * time.Millisecond, Reason: "HTTP 500"}
		},
	}

	sched, err := NewScheduler(context.Background(), store, checker, notifier, 1, 1, 0, DigestConfig{}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	for range 3 {
		sched.checkAndNotify(1)
	}
	if notifier.failureCount() != 1 {
		t.Errorf("failure notifications = %d, want 1 (reminders disabled)", notifier.failureCount())
	}
}

func TestCheckAndNotifyStoresAlertMessageID(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{Up: false, StatusCode: 500, Latency: 10 * time.Millisecond, Reason: "HTTP 500"}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)
	sched.checkAndNotify(1)

	ep, _ := store.GetEndpoint(context.Background(), 1)
	if ep.AlertMessageID == 0 {
		t.Error("AlertMessageID should be stored after a failure notification")
	}
}

func TestResumeExpiredPauses(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{
		ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok",
		Paused:      true,
		PausedUntil: sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true},
	})
	store.SetEndpoint(storage.Endpoint{
		ID: 2, URL: "https://other.example.com", IntervalSeconds: 30, Status: "ok",
		Paused:      true,
		PausedUntil: sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true},
	})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{Up: true, StatusCode: 200, Latency: 10 * time.Millisecond}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)
	sched.resumeExpiredPauses()

	ep1, _ := store.GetEndpoint(context.Background(), 1)
	if ep1.Paused {
		t.Error("endpoint 1 should have been resumed (expired pause)")
	}
	ep2, _ := store.GetEndpoint(context.Background(), 2)
	if !ep2.Paused {
		t.Error("endpoint 2 should still be paused (not yet expired)")
	}
}

func TestCertExpiryWarning(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{
				Up: true, StatusCode: 200, Latency: 10 * time.Millisecond,
				CertExpiry: time.Now().Add(7 * 24 * time.Hour), // 7 days left
			}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)

	sched.checkAndNotify(1)
	if notifier.certWarnings != 1 {
		t.Errorf("cert warnings = %d, want 1", notifier.certWarnings)
	}

	// Second check within cooldown: no new warning.
	sched.checkAndNotify(1)
	if notifier.certWarnings != 1 {
		t.Errorf("cert warnings = %d, want 1 (cooldown)", notifier.certWarnings)
	}
}

func TestCertExpiryNoWarningWhenFar(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{
				Up: true, StatusCode: 200, Latency: 10 * time.Millisecond,
				CertExpiry: time.Now().Add(90 * 24 * time.Hour),
			}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)
	sched.checkAndNotify(1)
	if notifier.certWarnings != 0 {
		t.Errorf("cert warnings = %d, want 0 (90 days left)", notifier.certWarnings)
	}
}

func TestCheckAndNotifySkipsDuringMaintenance(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	store.inMaintenance = true
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{Up: false, StatusCode: 503, Latency: 10 * time.Millisecond, Reason: "HTTP 503"}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)
	sched.checkAndNotify(1)

	if notifier.failureCount() != 0 {
		t.Error("should not notify during a maintenance window")
	}
	if store.recordedChecks != 0 {
		t.Error("should not record checks during a maintenance window")
	}
	ep, _ := store.GetEndpoint(context.Background(), 1)
	if ep.Status != "ok" {
		t.Errorf("status = %q, want ok (untouched during maintenance)", ep.Status)
	}
}

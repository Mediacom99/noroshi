package monitor

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"noroshi/internal/storage"
)

// mockStore implements Store for testing.
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

func (m *mockStore) UpdateEndpointStatus(_ context.Context, id int64, status string, _ int, _ int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return &notFoundError{}
	}
	ep.Status = status
	ep.LastCheckedAt = sql.NullTime{Time: time.Now(), Valid: true}
	m.endpoints[id] = ep
	return nil
}

func (m *mockStore) RecordFailure(_ context.Context, id int64, _ int, _ int64, maxNotifications int) (storage.Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return storage.Endpoint{}, &notFoundError{}
	}
	ep.ConsecutiveFailures++
	if ep.FailureNotificationsSent < maxNotifications {
		ep.FailureNotificationsSent++
	}
	ep.Status = "not_ok"
	if ep.ConsecutiveFailures == 1 {
		ep.LastFailureAt = sql.NullTime{Time: time.Now(), Valid: true}
	}
	ep.LastCheckedAt = sql.NullTime{Time: time.Now(), Valid: true}
	m.endpoints[id] = ep
	return ep, nil
}

func (m *mockStore) RecordRecovery(_ context.Context, id int64, _ int, _ int64) (storage.Endpoint, error) {
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

type notFoundError struct{}

func (e *notFoundError) Error() string { return "not found" }

// mockNotifier records notification calls.
type mockNotifier struct {
	mu         sync.Mutex
	failures   []storage.Endpoint
	recoveries []recoveryCall
}

type recoveryCall struct {
	Endpoint storage.Endpoint
	Downtime time.Duration
}

func (n *mockNotifier) NotifyFailure(_ context.Context, ep storage.Endpoint) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failures = append(n.failures, ep)
	return nil
}

func (n *mockNotifier) NotifyRecovery(_ context.Context, ep storage.Endpoint, downtime time.Duration) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.recoveries = append(n.recoveries, recoveryCall{ep, downtime})
	return nil
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
	checkFn func(ctx context.Context, url string) (int, time.Duration, error)
}

func (m *mockChecker) Check(ctx context.Context, url string) (int, time.Duration, error) {
	return m.checkFn(ctx, url)
}

func newMockScheduler(t *testing.T, store *mockStore, checker *mockChecker, notifier *mockNotifier, maxFail int) *Scheduler {
	t.Helper()
	sched, err := NewScheduler(context.Background(), store, checker, notifier, maxFail)
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
		checkFn: func(_ context.Context, _ string) (int, time.Duration, error) {
			return 200, 10 * time.Millisecond, nil
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
		checkFn: func(_ context.Context, _ string) (int, time.Duration, error) {
			return 503, 10 * time.Millisecond, nil
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
		checkFn: func(_ context.Context, _ string) (int, time.Duration, error) {
			return 0, 0, fmt.Errorf("connection refused")
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
		checkFn: func(_ context.Context, _ string) (int, time.Duration, error) {
			return 500, 10 * time.Millisecond, nil
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
		checkFn: func(_ context.Context, _ string) (int, time.Duration, error) {
			if returnFailure {
				return 503, 10 * time.Millisecond, nil
			}
			return 200, 10 * time.Millisecond, nil
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
	checker := NewHTTPChecker(5 * time.Second)

	sched, err := NewScheduler(context.Background(), store, checker, notifier, 3)
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
	checker := NewHTTPChecker(5 * time.Second)

	sched, err := NewScheduler(context.Background(), store, checker, notifier, 3)
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
	checker := NewHTTPChecker(5 * time.Second)

	maxNotifications := 3
	sched, err := NewScheduler(context.Background(), store, checker, notifier, maxNotifications)
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
	checker := NewHTTPChecker(5 * time.Second)

	sched, err := NewScheduler(context.Background(), store, checker, notifier, 3)
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
	checker := NewHTTPChecker(5 * time.Second)

	sched, err := NewScheduler(context.Background(), store, checker, notifier, 3)
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
			checkFn: func(_ context.Context, _ string) (int, time.Duration, error) {
				return code, 10 * time.Millisecond, nil
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
		checkFn: func(_ context.Context, _ string) (int, time.Duration, error) {
			return 301, 10 * time.Millisecond, nil
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
		checkFn: func(_ context.Context, _ string) (int, time.Duration, error) {
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
			return 200, 10 * time.Millisecond, nil
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
		checkFn: func(_ context.Context, _ string) (int, time.Duration, error) {
			if fail {
				return 503, 10 * time.Millisecond, nil
			}
			return 200, 10 * time.Millisecond, nil
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

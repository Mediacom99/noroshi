package monitor

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// mockStore implements Store for testing.
type mockStore struct {
	mu        sync.Mutex
	endpoints map[int64]Endpoint
}

func newMockStore() *mockStore {
	return &mockStore{endpoints: make(map[int64]Endpoint)}
}

func (m *mockStore) SetEndpoint(ep Endpoint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.endpoints[ep.ID] = ep
}

func (m *mockStore) GetEndpoint(_ context.Context, id int64) (Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return Endpoint{}, &notFoundError{}
	}
	return ep, nil
}

func (m *mockStore) UpdateEndpointStatus(_ context.Context, id int64, status string, _ int) error {
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

func (m *mockStore) RecordFailure(_ context.Context, id int64, _ int) (Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return Endpoint{}, &notFoundError{}
	}
	ep.ConsecutiveFailures++
	ep.FailureNotificationsSent++
	ep.Status = "not_ok"
	if ep.ConsecutiveFailures == 1 {
		ep.LastFailureAt = sql.NullTime{Time: time.Now(), Valid: true}
	}
	ep.LastCheckedAt = sql.NullTime{Time: time.Now(), Valid: true}
	m.endpoints[id] = ep
	return ep, nil
}

func (m *mockStore) RecordRecovery(_ context.Context, id int64, _ int) (Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.endpoints[id]
	if !ok {
		return Endpoint{}, &notFoundError{}
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
	failures   []Endpoint
	recoveries []recoveryCall
}

type recoveryCall struct {
	Endpoint Endpoint
	Downtime time.Duration
}

func (n *mockNotifier) NotifyFailure(_ context.Context, ep Endpoint) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failures = append(n.failures, ep)
	return nil
}

func (n *mockNotifier) NotifyRecovery(_ context.Context, ep Endpoint, downtime time.Duration) error {
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

func TestCheckAndNotifyOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := newMockStore()
	store.SetEndpoint(Endpoint{ID: 1, URL: srv.URL, IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := NewChecker(5 * time.Second)

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
	store.SetEndpoint(Endpoint{ID: 1, URL: srv.URL, IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := NewChecker(5 * time.Second)

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
	store.SetEndpoint(Endpoint{ID: 1, URL: srv.URL, IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := NewChecker(5 * time.Second)

	maxNotifications := 3
	sched, err := NewScheduler(context.Background(), store, checker, notifier, maxNotifications)
	if err != nil {
		t.Fatal(err)
	}

	// Run more failures than the cap
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
	store.SetEndpoint(Endpoint{ID: 1, URL: srv.URL, IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := NewChecker(5 * time.Second)

	sched, err := NewScheduler(context.Background(), store, checker, notifier, 3)
	if err != nil {
		t.Fatal(err)
	}

	// Cause a failure
	sched.checkAndNotify(1)

	// Recover
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
	store.SetEndpoint(Endpoint{ID: 1, URL: srv.URL, IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := NewChecker(5 * time.Second)

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

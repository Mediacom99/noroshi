package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"noroshi/internal/apperror"
	"noroshi/internal/storage"
)

const testToken = "test-token"

// mockStore implements api.Store with function-field delegation.
type mockStore struct {
	addEndpointFn          func(ctx context.Context, name, url string, interval int) (storage.Endpoint, error)
	getEndpointFn          func(ctx context.Context, id int64) (storage.Endpoint, error)
	deleteEndpointFn       func(ctx context.Context, id int64) error
	listEndpointsFn        func(ctx context.Context) ([]storage.Endpoint, error)
	updateIntervalFn       func(ctx context.Context, id int64, interval int) error
	setPausedFn            func(ctx context.Context, id int64, paused bool, until sql.NullTime) error
	setExpectedStatusFn    func(ctx context.Context, id int64, code int) error
	setExpectedKeywordFn   func(ctx context.Context, id int64, keyword string) error
	renameEndpointFn       func(ctx context.Context, id int64, newName string) error
	getCheckStatsFn        func(ctx context.Context, endpointID int64, since time.Time) (storage.WindowStats, error)
	getRecentTransitionsFn func(ctx context.Context, endpointID int64, limit int) ([]storage.CheckTransition, error)
	getRecentChecksFn      func(ctx context.Context, endpointID int64, since time.Time) ([]storage.Check, error)
	getDailyStatsFn        func(ctx context.Context, endpointID int64, since time.Time) ([]storage.DayStat, error)
	addMaintenanceFn       func(ctx context.Context, endpointID sql.NullInt64, days string, startMinutes, endMinutes int) (storage.MaintenanceWindow, error)
	listMaintenanceFn      func(ctx context.Context) ([]storage.MaintenanceWindow, error)
	deleteMaintenanceFn    func(ctx context.Context, id int64) error
}

func (m *mockStore) AddEndpoint(ctx context.Context, name, url string, interval int) (storage.Endpoint, error) {
	return m.addEndpointFn(ctx, name, url, interval)
}
func (m *mockStore) GetEndpoint(ctx context.Context, id int64) (storage.Endpoint, error) {
	return m.getEndpointFn(ctx, id)
}
func (m *mockStore) DeleteEndpoint(ctx context.Context, id int64) error {
	return m.deleteEndpointFn(ctx, id)
}
func (m *mockStore) ListEndpoints(ctx context.Context) ([]storage.Endpoint, error) {
	return m.listEndpointsFn(ctx)
}
func (m *mockStore) UpdateEndpointInterval(ctx context.Context, id int64, interval int) error {
	return m.updateIntervalFn(ctx, id, interval)
}
func (m *mockStore) SetEndpointPaused(ctx context.Context, id int64, paused bool, until sql.NullTime) error {
	return m.setPausedFn(ctx, id, paused, until)
}
func (m *mockStore) SetExpectedStatus(ctx context.Context, id int64, code int) error {
	return m.setExpectedStatusFn(ctx, id, code)
}
func (m *mockStore) SetExpectedKeyword(ctx context.Context, id int64, keyword string) error {
	return m.setExpectedKeywordFn(ctx, id, keyword)
}
func (m *mockStore) RenameEndpoint(ctx context.Context, id int64, newName string) error {
	return m.renameEndpointFn(ctx, id, newName)
}
func (m *mockStore) GetCheckStats(ctx context.Context, endpointID int64, since time.Time) (storage.WindowStats, error) {
	return m.getCheckStatsFn(ctx, endpointID, since)
}
func (m *mockStore) GetRecentTransitions(ctx context.Context, endpointID int64, limit int) ([]storage.CheckTransition, error) {
	return m.getRecentTransitionsFn(ctx, endpointID, limit)
}
func (m *mockStore) GetRecentChecks(ctx context.Context, endpointID int64, since time.Time) ([]storage.Check, error) {
	return m.getRecentChecksFn(ctx, endpointID, since)
}
func (m *mockStore) GetDailyStats(ctx context.Context, endpointID int64, since time.Time) ([]storage.DayStat, error) {
	return m.getDailyStatsFn(ctx, endpointID, since)
}
func (m *mockStore) AddMaintenanceWindow(ctx context.Context, endpointID sql.NullInt64, days string, startMinutes, endMinutes int) (storage.MaintenanceWindow, error) {
	return m.addMaintenanceFn(ctx, endpointID, days, startMinutes, endMinutes)
}
func (m *mockStore) ListMaintenanceWindows(ctx context.Context) ([]storage.MaintenanceWindow, error) {
	return m.listMaintenanceFn(ctx)
}
func (m *mockStore) DeleteMaintenanceWindow(ctx context.Context, id int64) error {
	return m.deleteMaintenanceFn(ctx, id)
}

// mockScheduler implements api.Scheduler with function-field delegation.
type mockScheduler struct {
	addFn      func(ctx context.Context, ep storage.Endpoint) error
	removeFn   func(endpointID int64) error
	checkNowFn func(ctx context.Context, endpointID int64) (storage.Endpoint, error)
}

func (m *mockScheduler) Add(ctx context.Context, ep storage.Endpoint) error {
	if m.addFn != nil {
		return m.addFn(ctx, ep)
	}
	return nil
}
func (m *mockScheduler) Remove(endpointID int64) error {
	if m.removeFn != nil {
		return m.removeFn(endpointID)
	}
	return nil
}
func (m *mockScheduler) CheckNow(ctx context.Context, endpointID int64) (storage.Endpoint, error) {
	if m.checkNowFn != nil {
		return m.checkNowFn(ctx, endpointID)
	}
	return storage.Endpoint{ID: endpointID, Status: "ok"}, nil
}

func testEndpoint() storage.Endpoint {
	return storage.Endpoint{
		ID:              1,
		Name:            "prod-api",
		URL:             "https://example.com",
		IntervalSeconds: 60,
		Status:          "ok",
		LastStatusCode:  200,
		LastLatencyMs:   42,
		CreatedAt:       time.Now().UTC(),
	}
}

// newTestServer wires a Server with a mock store holding one endpoint.
func newTestServer(store *mockStore, sched *mockScheduler) *Server {
	return NewServer(store, sched, testToken, []string{"https://status.example.com"}, nil)
}

func defaultMockStore() *mockStore {
	ep := testEndpoint()
	return &mockStore{
		addEndpointFn: func(_ context.Context, name, url string, interval int) (storage.Endpoint, error) {
			return storage.Endpoint{ID: 2, Name: name, URL: url, IntervalSeconds: interval}, nil
		},
		getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
			if id != ep.ID {
				return storage.Endpoint{}, apperror.Wrap(apperror.ErrNotFound, nil)
			}
			return ep, nil
		},
		deleteEndpointFn: func(context.Context, int64) error { return nil },
		listEndpointsFn: func(context.Context) ([]storage.Endpoint, error) {
			return []storage.Endpoint{ep}, nil
		},
		updateIntervalFn: func(_ context.Context, _ int64, interval int) error {
			ep.IntervalSeconds = interval
			return nil
		},
		setPausedFn: func(_ context.Context, _ int64, paused bool, until sql.NullTime) error {
			ep.Paused = paused
			ep.PausedUntil = until
			return nil
		},
		setExpectedStatusFn:  func(context.Context, int64, int) error { return nil },
		setExpectedKeywordFn: func(context.Context, int64, string) error { return nil },
		renameEndpointFn: func(_ context.Context, _ int64, newName string) error {
			ep.Name = newName
			return nil
		},
		getCheckStatsFn: func(context.Context, int64, time.Time) (storage.WindowStats, error) {
			return storage.WindowStats{Total: 10, Up: 9}, nil
		},
		getRecentTransitionsFn: func(context.Context, int64, int) ([]storage.CheckTransition, error) {
			return []storage.CheckTransition{
				{CheckedAt: time.Now().Add(-2 * time.Hour), Up: false, StatusCode: 503},
				{CheckedAt: time.Now().Add(-time.Hour), Up: true, StatusCode: 200},
			}, nil
		},
		getRecentChecksFn: func(context.Context, int64, time.Time) ([]storage.Check, error) {
			return []storage.Check{{CheckedAt: time.Now(), Up: true, StatusCode: 200, LatencyMs: 42}}, nil
		},
		getDailyStatsFn: func(context.Context, int64, time.Time) ([]storage.DayStat, error) {
			return []storage.DayStat{{Date: "2026-09-02", Total: 100, Up: 98, AvgLatencyMs: 55}}, nil
		},
		addMaintenanceFn: func(_ context.Context, endpointID sql.NullInt64, days string, startMinutes, endMinutes int) (storage.MaintenanceWindow, error) {
			return storage.MaintenanceWindow{ID: 1, EndpointID: endpointID, Days: days, StartMinutes: startMinutes, EndMinutes: endMinutes}, nil
		},
		listMaintenanceFn: func(context.Context) ([]storage.MaintenanceWindow, error) {
			return []storage.MaintenanceWindow{{ID: 1, Days: "sat,sun", StartMinutes: 120, EndMinutes: 240}}, nil
		},
		deleteMaintenanceFn: func(_ context.Context, id int64) error {
			if id != 1 {
				return apperror.Wrap(apperror.ErrNotFound, nil)
			}
			return nil
		},
	}
}

// do performs an authenticated request against the server handler.
func do(t *testing.T, srv *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rec.Body.String())
	}
	return out
}

func TestAuthRequired(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})

	for _, tc := range []struct {
		name   string
		header string
	}{
		{"missing header", ""},
		{"wrong token", "Bearer wrong"},
		{"malformed scheme", "Basic abc"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/endpoints", nil)
		if tc.header != "" {
			req.Header.Set("Authorization", tc.header)
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", tc.name, rec.Code)
		}
	}

	// Correct token passes.
	rec := do(t, srv, http.MethodGet, "/api/endpoints", "")
	if rec.Code != http.StatusOK {
		t.Errorf("valid token: status = %d, want 200", rec.Code)
	}
}

func TestCORS(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})

	// Preflight from an allowed origin.
	req := httptest.NewRequest(http.MethodOptions, "/api/endpoints", nil)
	req.Header.Set("Origin", "https://status.example.com")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight: status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://status.example.com" {
		t.Errorf("preflight: ACAO = %q, want https://status.example.com", got)
	}

	// Preflight from a disallowed origin gets no CORS headers.
	req = httptest.NewRequest(http.MethodOptions, "/api/endpoints", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("disallowed origin: ACAO = %q, want empty", got)
	}
}

func TestListEndpoints(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})
	rec := do(t, srv, http.MethodGet, "/api/endpoints", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := decodeBody(t, rec)
	eps, ok := body["endpoints"].([]any)
	if !ok || len(eps) != 1 {
		t.Fatalf("endpoints = %v, want 1 entry", body["endpoints"])
	}
	ep := eps[0].(map[string]any)
	if ep["name"] != "prod-api" || ep["type"] != "https" {
		t.Errorf("unexpected endpoint: %v", ep)
	}
}

func TestAddEndpoint(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})

	for _, tc := range []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"valid", `{"name":"web","url":"https://example.org","interval_seconds":30}`, http.StatusCreated},
		{"default interval", `{"name":"web","url":"https://example.org"}`, http.StatusCreated},
		{"invalid name", `{"name":"123","url":"https://example.org"}`, http.StatusBadRequest},
		{"invalid url", `{"name":"web","url":"ftp://example.org"}`, http.StatusBadRequest},
		{"interval too small", `{"name":"web","url":"https://example.org","interval_seconds":5}`, http.StatusBadRequest},
		{"bad json", `{`, http.StatusBadRequest},
	} {
		rec := do(t, srv, http.MethodPost, "/api/endpoints", tc.body)
		if rec.Code != tc.wantStatus {
			t.Errorf("%s: status = %d, want %d (body: %s)", tc.name, rec.Code, tc.wantStatus, rec.Body.String())
		}
	}
}

func TestAddEndpointDuplicate(t *testing.T) {
	store := defaultMockStore()
	store.addEndpointFn = func(context.Context, string, string, int) (storage.Endpoint, error) {
		return storage.Endpoint{}, apperror.Wrap(apperror.ErrDuplicate, nil)
	}
	srv := newTestServer(store, &mockScheduler{})

	rec := do(t, srv, http.MethodPost, "/api/endpoints", `{"name":"web","url":"https://example.org"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestGetEndpointWithStats(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})
	rec := do(t, srv, http.MethodGet, "/api/endpoints/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := decodeBody(t, rec)
	stats, ok := body["stats"].(map[string]any)
	if !ok {
		t.Fatalf("missing stats: %v", body)
	}
	for _, window := range []string{"24h", "7d", "30d"} {
		if _, ok := stats[window]; !ok {
			t.Errorf("missing stats window %q", window)
		}
	}
}

func TestGetEndpointNotFound(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})
	rec := do(t, srv, http.MethodGet, "/api/endpoints/99", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestUpdateEndpoint(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})

	for _, tc := range []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"rename", `{"name":"new-name"}`, http.StatusOK},
		{"interval", `{"interval_seconds":120}`, http.StatusOK},
		{"interval too small", `{"interval_seconds":5}`, http.StatusBadRequest},
		{"expected status", `{"expected_status":200}`, http.StatusOK},
		{"expected status any", `{"expected_status":0}`, http.StatusOK},
		{"expected status invalid", `{"expected_status":99}`, http.StatusBadRequest},
		{"keyword", `{"expected_keyword":"ok"}`, http.StatusOK},
		{"keyword clear", `{"expected_keyword":""}`, http.StatusOK},
		{"keyword bad regex", `{"expected_keyword":"re:["}`, http.StatusBadRequest},
	} {
		rec := do(t, srv, http.MethodPatch, "/api/endpoints/1", tc.body)
		if rec.Code != tc.wantStatus {
			t.Errorf("%s: status = %d, want %d (body: %s)", tc.name, rec.Code, tc.wantStatus, rec.Body.String())
		}
	}
}

func TestDeleteEndpoint(t *testing.T) {
	var removed int64
	sched := &mockScheduler{removeFn: func(id int64) error { removed = id; return nil }}
	srv := newTestServer(defaultMockStore(), sched)

	rec := do(t, srv, http.MethodDelete, "/api/endpoints/1", "")
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if removed != 1 {
		t.Errorf("scheduler.Remove called with %d, want 1", removed)
	}
}

func TestPauseAndResume(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})

	rec := do(t, srv, http.MethodPost, "/api/endpoints/1/pause", `{"duration":"2h"}`)
	if rec.Code != http.StatusOK {
		t.Errorf("pause: status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	ep := decodeBody(t, rec)["endpoint"].(map[string]any)
	if ep["paused_until"] == nil {
		t.Errorf("timed pause should set paused_until: %v", ep)
	}

	rec = do(t, srv, http.MethodPost, "/api/endpoints/1/pause", `{"duration":"nope"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad duration: status = %d, want 400", rec.Code)
	}

	rec = do(t, srv, http.MethodPost, "/api/endpoints/1/resume", "")
	if rec.Code != http.StatusOK {
		t.Errorf("resume: status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestCheckNow(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})
	rec := do(t, srv, http.MethodPost, "/api/endpoints/1/check", "")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestListIncidents(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})
	rec := do(t, srv, http.MethodGet, "/api/endpoints/1/incidents", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	incidents := decodeBody(t, rec)["incidents"].([]any)
	if len(incidents) != 1 {
		t.Fatalf("incidents = %d, want 1", len(incidents))
	}
	inc := incidents[0].(map[string]any)
	if inc["status_code"].(float64) != 503 || inc["duration_seconds"].(float64) != 3600 {
		t.Errorf("unexpected incident: %v", inc)
	}
}

func TestListChecks(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})

	rec := do(t, srv, http.MethodGet, "/api/endpoints/1/checks?window=7d", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	checks := decodeBody(t, rec)["checks"].([]any)
	if len(checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(checks))
	}

	rec = do(t, srv, http.MethodGet, "/api/endpoints/1/checks?window=bogus", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad window: status = %d, want 400", rec.Code)
	}
}

func TestListDailyStats(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})

	rec := do(t, srv, http.MethodGet, "/api/endpoints/1/daily", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	days := decodeBody(t, rec)["days"].([]any)
	if len(days) != 1 {
		t.Fatalf("days = %d, want 1", len(days))
	}
	day := days[0].(map[string]any)
	if day["date"] != "2026-09-02" || day["uptime"].(float64) != 98 {
		t.Errorf("unexpected day stat: %v", day)
	}

	rec = do(t, srv, http.MethodGet, "/api/endpoints/1/daily?days=0", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("days=0: status = %d, want 400", rec.Code)
	}
	rec = do(t, srv, http.MethodGet, "/api/endpoints/1/daily?days=90", "")
	if rec.Code != http.StatusOK {
		t.Errorf("days=90 should clamp to retention, status = %d, want 200", rec.Code)
	}
}

func TestMaintenanceWindows(t *testing.T) {
	srv := newTestServer(defaultMockStore(), &mockScheduler{})

	// List.
	rec := do(t, srv, http.MethodGet, "/api/maintenance", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status = %d, want 200", rec.Code)
	}
	windows := decodeBody(t, rec)["maintenance"].([]any)
	if len(windows) != 1 {
		t.Fatalf("maintenance = %d, want 1", len(windows))
	}
	mw := windows[0].(map[string]any)
	if mw["days"] != "sat,sun" || mw["start_minutes"].(float64) != 120 {
		t.Errorf("unexpected window: %v", mw)
	}
	if _, ok := mw["active"].(bool); !ok {
		t.Errorf("window should carry an active flag: %v", mw)
	}

	// Add: valid (endpoint-scoped), global, and invalid inputs.
	for _, tc := range []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"endpoint scoped", `{"endpoint_id":1,"days":"mon,wed","start":"02:00","end":"04:00"}`, http.StatusCreated},
		{"global", `{"days":"all","start":"22:00","end":"02:00"}`, http.StatusCreated},
		{"unknown endpoint", `{"endpoint_id":99,"days":"all","start":"02:00","end":"04:00"}`, http.StatusNotFound},
		{"bad days", `{"days":"monday","start":"02:00","end":"04:00"}`, http.StatusBadRequest},
		{"bad time", `{"days":"all","start":"2am","end":"04:00"}`, http.StatusBadRequest},
		{"equal start/end", `{"days":"all","start":"02:00","end":"02:00"}`, http.StatusBadRequest},
	} {
		rec := do(t, srv, http.MethodPost, "/api/maintenance", tc.body)
		if rec.Code != tc.wantStatus {
			t.Errorf("%s: status = %d, want %d (body: %s)", tc.name, rec.Code, tc.wantStatus, rec.Body.String())
		}
	}

	// Delete: existing and missing.
	rec = do(t, srv, http.MethodDelete, "/api/maintenance/1", "")
	if rec.Code != http.StatusNoContent {
		t.Errorf("delete: status = %d, want 204", rec.Code)
	}
	rec = do(t, srv, http.MethodDelete, "/api/maintenance/99", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("delete missing: status = %d, want 404", rec.Code)
	}
}

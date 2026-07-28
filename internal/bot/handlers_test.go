package bot

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"noroshi/internal/apperror"
	"noroshi/internal/storage"

	tele "gopkg.in/telebot.v4"
)

func TestHandleAdd(t *testing.T) {
	tests := []struct {
		name         string
		payload      string
		chatID       int64
		store        *mockStore
		scheduler    *mockScheduler
		useGuarded   bool
		wantContains []string
		wantNoSend   bool
		wantAddCalls int
	}{
		{
			name:    "happy path with default interval",
			payload: "prod-api https://example.com",
			store: &mockStore{
				addEndpointFn: func(_ context.Context, name, url string, interval int) (storage.Endpoint, error) {
					return storage.Endpoint{ID: 1, Name: name, URL: url, IntervalSeconds: interval}, nil
				},
			},
			scheduler:    &mockScheduler{},
			wantContains: []string{"Added endpoint", "prod-api", "https://example.com"},
			wantAddCalls: 1,
		},
		{
			name:    "happy path with custom interval",
			payload: "prod-api https://example.com 30s",
			store: &mockStore{
				addEndpointFn: func(_ context.Context, name, url string, interval int) (storage.Endpoint, error) {
					return storage.Endpoint{ID: 1, Name: name, URL: url, IntervalSeconds: interval}, nil
				},
			},
			scheduler:    &mockScheduler{},
			wantContains: []string{"Added endpoint", "30s"},
			wantAddCalls: 1,
		},
		{
			name:         "too few args",
			payload:      "only-one-arg",
			store:        &mockStore{},
			wantContains: []string{"/add"},
		},
		{
			name:         "empty payload",
			payload:      "",
			store:        &mockStore{},
			wantContains: []string{"/add"},
		},
		{
			name:         "invalid URL",
			payload:      "good-name notaurl",
			store:        &mockStore{},
			wantContains: []string{"Invalid URL"},
		},
		{
			name:         "invalid interval format",
			payload:      "prod-api https://example.com notaduration",
			store:        &mockStore{},
			wantContains: []string{"Invalid interval"},
		},
		{
			name:         "interval too short",
			payload:      "prod-api https://example.com 5s",
			store:        &mockStore{},
			wantContains: []string{"at least 10s"},
		},
		{
			name:    "duplicate endpoint",
			payload: "prod-api https://example.com",
			store: &mockStore{
				addEndpointFn: func(_ context.Context, _, _ string, _ int) (storage.Endpoint, error) {
					return storage.Endpoint{}, apperror.Wrap(apperror.ErrDuplicate, fmt.Errorf("UNIQUE constraint"))
				},
			},
			wantContains: []string{"already being monitored"},
		},
		{
			name:    "store error",
			payload: "prod-api https://example.com",
			store: &mockStore{
				addEndpointFn: func(_ context.Context, _, _ string, _ int) (storage.Endpoint, error) {
					return storage.Endpoint{}, fmt.Errorf("db error")
				},
			},
			wantContains: []string{"Internal error"},
		},
		{
			name:    "nil scheduler",
			payload: "prod-api https://example.com",
			store: &mockStore{
				addEndpointFn: func(_ context.Context, name, url string, interval int) (storage.Endpoint, error) {
					return storage.Endpoint{ID: 1, Name: name, URL: url, IntervalSeconds: interval}, nil
				},
			},
			scheduler:    nil,
			wantContains: []string{"Added endpoint"},
		},
		{
			name:       "wrong chat ID",
			payload:    "prod-api https://example.com",
			chatID:     999,
			store:      &mockStore{},
			useGuarded: true,
			wantNoSend: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sched Scheduler
			if tt.scheduler != nil {
				sched = tt.scheduler
			}
			b := newTestBot(tt.store, sched)

			chatID := int64(123)
			if tt.chatID != 0 {
				chatID = tt.chatID
			}
			mc := &mockContext{
				messageFn: func() *tele.Message {
					return &tele.Message{Payload: tt.payload}
				},
				chatFn: func() *tele.Chat {
					return &tele.Chat{ID: chatID}
				},
			}

			var err error
			if tt.useGuarded {
				err = b.guarded(b.handleAdd)(mc)
			} else {
				err = b.handleAdd(mc)
			}
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if tt.wantNoSend {
				if len(mc.sentMessages) != 0 {
					t.Errorf("expected no messages, got %d: %v", len(mc.sentMessages), mc.sentMessages)
				}
				return
			}
			if len(mc.sentMessages) == 0 {
				t.Fatal("expected a sent message, got none")
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(mc.sentMessages[0], want) {
					t.Errorf("message should contain %q, got: %s", want, mc.sentMessages[0])
				}
			}
			if tt.scheduler != nil && tt.wantAddCalls > 0 {
				if tt.scheduler.addCalls != tt.wantAddCalls {
					t.Errorf("expected scheduler.Add called %d times, got %d", tt.wantAddCalls, tt.scheduler.addCalls)
				}
			}
		})
	}
}

func TestHandleDelete(t *testing.T) {
	testEndpoint := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", IntervalSeconds: 60}
	notFoundErr := apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("not found"))

	tests := []struct {
		name            string
		payload         string
		store           *mockStore
		scheduler       *mockScheduler
		wantContains    []string
		wantRemoveCalls int
	}{
		{
			name:    "delete by numeric ID",
			payload: "1",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
			},
			scheduler:       &mockScheduler{},
			wantContains:    []string{"Deleted", "prod-api"},
			wantRemoveCalls: 1,
		},
		{
			name:    "delete by name",
			payload: "prod-api",
			store: &mockStore{
				// "prod-api" is non-numeric, so GetEndpoint is not called.
				// findEndpoint tries GetEndpointByName next.
				getEndpointByNameFn: func(_ context.Context, name string) (storage.Endpoint, error) {
					if name == "prod-api" {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
			},
			scheduler:       &mockScheduler{},
			wantContains:    []string{"Deleted", "prod-api"},
			wantRemoveCalls: 1,
		},
		{
			name:    "delete by URL",
			payload: "https://example.com",
			store: &mockStore{
				// "https://example.com" is non-numeric, so GetEndpoint is not called.
				// GetEndpointByName returns not found.
				getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
					return storage.Endpoint{}, notFoundErr
				},
				// GetEndpointByURL returns the endpoint.
				getEndpointByURLFn: func(_ context.Context, url string) (storage.Endpoint, error) {
					if url == "https://example.com" {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
			},
			scheduler:       &mockScheduler{},
			wantContains:    []string{"Deleted", "prod-api"},
			wantRemoveCalls: 1,
		},
		{
			name:         "empty arg",
			payload:      "",
			store:        &mockStore{},
			wantContains: []string{"Usage:"},
		},
		{
			name:    "not found",
			payload: "nonexistent",
			store: &mockStore{
				getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
					return storage.Endpoint{}, notFoundErr
				},
				getEndpointByURLFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
					return storage.Endpoint{}, notFoundErr
				},
			},
			wantContains: []string{"Endpoint not found"},
		},
		{
			name:    "store error on delete",
			payload: "1",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, _ int64) (storage.Endpoint, error) {
					return testEndpoint, nil
				},
				deleteEndpointFn: func(_ context.Context, _ int64) error {
					return fmt.Errorf("db error")
				},
			},
			scheduler:    &mockScheduler{},
			wantContains: []string{"Internal error"},
		},
		{
			name:    "nil scheduler on delete",
			payload: "1",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, _ int64) (storage.Endpoint, error) {
					return testEndpoint, nil
				},
			},
			scheduler:    nil,
			wantContains: []string{"Deleted", "prod-api"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sched Scheduler
			if tt.scheduler != nil {
				sched = tt.scheduler
			}
			b := newTestBot(tt.store, sched)

			mc := &mockContext{
				messageFn: func() *tele.Message {
					return &tele.Message{Payload: tt.payload}
				},
				chatFn: func() *tele.Chat {
					return &tele.Chat{ID: 123}
				},
			}

			err := b.handleDelete(mc)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if len(mc.sentMessages) == 0 {
				t.Fatal("expected a sent message, got none")
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(mc.sentMessages[0], want) {
					t.Errorf("message should contain %q, got: %s", want, mc.sentMessages[0])
				}
			}
			if tt.scheduler != nil && tt.wantRemoveCalls > 0 {
				if tt.scheduler.removeCalls != tt.wantRemoveCalls {
					t.Errorf("expected scheduler.Remove called %d times, got %d", tt.wantRemoveCalls, tt.scheduler.removeCalls)
				}
			}
		})
	}
}

func TestHandleList(t *testing.T) {
	tests := []struct {
		name         string
		store        *mockStore
		wantContains []string
	}{
		{
			name:         "empty list",
			store:        &mockStore{},
			wantContains: []string{"No endpoints"},
		},
		{
			name: "list with endpoints",
			store: &mockStore{
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return []storage.Endpoint{
						{ID: 1, Name: "site-a", URL: "https://a.com", IntervalSeconds: 30, Status: "ok"},
						{ID: 2, Name: "site-b", URL: "https://b.com", IntervalSeconds: 60, Status: "not_ok"},
					}, nil
				},
			},
			wantContains: []string{"endpoints healthy"},
		},
		{
			name: "store error",
			store: &mockStore{
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return nil, fmt.Errorf("db error")
				},
			},
			wantContains: []string{"Internal error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBot(tt.store, nil)

			mc := &mockContext{
				chatFn: func() *tele.Chat {
					return &tele.Chat{ID: 123}
				},
			}

			err := b.handleList(mc)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if len(mc.sentMessages) == 0 {
				t.Fatal("expected a sent message, got none")
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(mc.sentMessages[0], want) {
					t.Errorf("message should contain %q, got: %s", want, mc.sentMessages[0])
				}
			}
		})
	}
}

func TestHandleInterval(t *testing.T) {
	testEndpoint := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", IntervalSeconds: 60}
	notFoundErr := apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("not found"))

	tests := []struct {
		name            string
		payload         string
		store           *mockStore
		scheduler       *mockScheduler
		wantContains    []string
		wantRemoveCalls int
		wantAddCalls    int
	}{
		{
			name:    "happy path by name",
			payload: "prod-api 5m",
			store: &mockStore{
				getEndpointByNameFn: func(_ context.Context, name string) (storage.Endpoint, error) {
					if name == "prod-api" {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
			},
			scheduler:       &mockScheduler{},
			wantContains:    []string{"Updated interval", "5m"},
			wantRemoveCalls: 1,
			wantAddCalls:    1,
		},
		{
			name:    "happy path by ID",
			payload: "1 30s",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
			},
			scheduler:       &mockScheduler{},
			wantContains:    []string{"Updated interval", "30s"},
			wantRemoveCalls: 1,
			wantAddCalls:    1,
		},
		{
			name:         "too few args",
			payload:      "prod-api",
			store:        &mockStore{},
			wantContains: []string{"/interval"},
		},
		{
			name:    "not found",
			payload: "nonexistent 5m",
			store: &mockStore{
				getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
					return storage.Endpoint{}, notFoundErr
				},
				getEndpointByURLFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
					return storage.Endpoint{}, notFoundErr
				},
			},
			wantContains: []string{"Endpoint not found"},
		},
		{
			name:    "invalid interval",
			payload: "prod-api notaduration",
			store: &mockStore{
				getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
					return testEndpoint, nil
				},
			},
			wantContains: []string{"Invalid interval"},
		},
		{
			name:    "interval too short",
			payload: "prod-api 5s",
			store: &mockStore{
				getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
					return testEndpoint, nil
				},
			},
			wantContains: []string{"at least 10s"},
		},
		{
			name:    "store error on update",
			payload: "prod-api 5m",
			store: &mockStore{
				getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
					return testEndpoint, nil
				},
				updateEndpointIntervalFn: func(_ context.Context, _ int64, _ int) error {
					return fmt.Errorf("db error")
				},
			},
			wantContains: []string{"Internal error"},
		},
		{
			name:    "nil scheduler",
			payload: "prod-api 5m",
			store: &mockStore{
				getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
					return testEndpoint, nil
				},
			},
			scheduler:    nil,
			wantContains: []string{"Updated interval"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sched Scheduler
			if tt.scheduler != nil {
				sched = tt.scheduler
			}
			b := newTestBot(tt.store, sched)

			mc := &mockContext{
				messageFn: func() *tele.Message {
					return &tele.Message{Payload: tt.payload}
				},
				chatFn: func() *tele.Chat {
					return &tele.Chat{ID: 123}
				},
			}

			err := b.handleInterval(mc)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if len(mc.sentMessages) == 0 {
				t.Fatal("expected a sent message, got none")
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(mc.sentMessages[0], want) {
					t.Errorf("message should contain %q, got: %s", want, mc.sentMessages[0])
				}
			}
			if tt.scheduler != nil {
				if tt.wantRemoveCalls > 0 && tt.scheduler.removeCalls != tt.wantRemoveCalls {
					t.Errorf("expected scheduler.Remove called %d times, got %d", tt.wantRemoveCalls, tt.scheduler.removeCalls)
				}
				if tt.wantAddCalls > 0 && tt.scheduler.addCalls != tt.wantAddCalls {
					t.Errorf("expected scheduler.Add called %d times, got %d", tt.wantAddCalls, tt.scheduler.addCalls)
				}
			}
		})
	}
}

func TestHandleHelp(t *testing.T) {
	tests := []struct {
		name         string
		wantContains []string
	}{
		{
			name:         "returns help text",
			wantContains: []string{"/add", "/delete", "/list", "/interval", "/help", "Noroshi"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBot(&mockStore{}, nil)

			mc := &mockContext{
				chatFn: func() *tele.Chat {
					return &tele.Chat{ID: 123}
				},
			}

			err := b.handleHelp(mc)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if len(mc.sentMessages) == 0 {
				t.Fatal("expected a sent message, got none")
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(mc.sentMessages[0], want) {
					t.Errorf("message should contain %q, got: %s", want, mc.sentMessages[0])
				}
			}
		})
	}
}

func TestHandleStatus(t *testing.T) {
	tests := []struct {
		name         string
		store        *mockStore
		scheduler    *mockScheduler
		wantContains []string
	}{
		{
			name:         "empty list",
			store:        &mockStore{},
			scheduler:    &mockScheduler{},
			wantContains: []string{"No endpoints"},
		},
		{
			name: "checks all endpoints",
			store: &mockStore{
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return []storage.Endpoint{
						{ID: 1, Name: "site-a", URL: "https://a.com", IntervalSeconds: 30, Status: "unknown"},
						{ID: 2, Name: "site-b", URL: "https://b.com", IntervalSeconds: 60, Status: "unknown"},
					}, nil
				},
			},
			scheduler: &mockScheduler{
				checkNowFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return storage.Endpoint{ID: 1, Name: "site-a", Status: "ok", LastStatusCode: 200, LastLatencyMs: 42}, nil
					}
					return storage.Endpoint{ID: 2, Name: "site-b", Status: "not_ok", LastStatusCode: 503, LastLatencyMs: 120}, nil
				},
			},
			wantContains: []string{"1/2 healthy", "site-a", "HTTP 200", "site-b", "HTTP 503"},
		},
		{
			name: "store error",
			store: &mockStore{
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return nil, fmt.Errorf("db error")
				},
			},
			scheduler:    &mockScheduler{},
			wantContains: []string{"Internal error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBot(tt.store, tt.scheduler)

			mc := &mockContext{
				chatFn: func() *tele.Chat {
					return &tele.Chat{ID: 123}
				},
			}

			err := b.handleStatus(mc)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if len(mc.sentMessages) == 0 {
				t.Fatal("expected a sent message, got none")
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(mc.sentMessages[0], want) {
					t.Errorf("message should contain %q, got: %s", want, mc.sentMessages[0])
				}
			}
		})
	}
}

func TestHandlePauseResume(t *testing.T) {
	notFoundErr := apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("not found"))
	active := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "ok"}

	tests := []struct {
		name          string
		handler       func(*Bot, tele.Context) error
		payload       string
		paused        bool
		wantContains  string
		wantSchedAdd  int
		wantSchedRm   int
		wantStoreCall bool
	}{
		{name: "pause active", handler: (*Bot).handlePause, payload: "prod-api", wantContains: "Paused", wantSchedRm: 1, wantStoreCall: true},
		{name: "resume paused", handler: (*Bot).handleResume, payload: "prod-api", paused: true, wantContains: "Resumed", wantSchedAdd: 1, wantStoreCall: true},
		{name: "pause already paused", handler: (*Bot).handlePause, payload: "prod-api", paused: true, wantContains: "already paused"},
		{name: "resume already active", handler: (*Bot).handleResume, payload: "prod-api", wantContains: "already resumed"},
		{name: "missing argument", handler: (*Bot).handlePause, payload: "", wantContains: "Usage: /pause"},
		{name: "not found", handler: (*Bot).handlePause, payload: "nope", wantContains: "Endpoint not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := active
			ep.Paused = tt.paused

			storeCalled := false
			store := &mockStore{
				getEndpointByNameFn: func(_ context.Context, name string) (storage.Endpoint, error) {
					if name == "prod-api" {
						return ep, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
				getEndpointByURLFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
					return storage.Endpoint{}, notFoundErr
				},
				setEndpointPausedFn: func(_ context.Context, _ int64, _ bool) error {
					storeCalled = true
					return nil
				},
			}
			sched := &mockScheduler{}
			b := newTestBot(store, sched)

			mc := &mockContext{
				messageFn: func() *tele.Message {
					return &tele.Message{Payload: tt.payload}
				},
			}

			if err := tt.handler(b, mc); err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
			if len(mc.sentMessages) == 0 {
				t.Fatal("expected a sent message, got none")
			}
			if !strings.Contains(mc.sentMessages[0], tt.wantContains) {
				t.Errorf("message should contain %q, got: %s", tt.wantContains, mc.sentMessages[0])
			}
			if sched.addCalls != tt.wantSchedAdd {
				t.Errorf("scheduler Add calls = %d, want %d", sched.addCalls, tt.wantSchedAdd)
			}
			if sched.removeCalls != tt.wantSchedRm {
				t.Errorf("scheduler Remove calls = %d, want %d", sched.removeCalls, tt.wantSchedRm)
			}
			if storeCalled != tt.wantStoreCall {
				t.Errorf("SetEndpointPaused called = %v, want %v", storeCalled, tt.wantStoreCall)
			}
		})
	}
}

func TestHandleIntervalPausedEndpoint(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "ok", Paused: true}

	var updatedTo int
	store := &mockStore{
		getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
			return ep, nil
		},
		updateEndpointIntervalFn: func(_ context.Context, _ int64, interval int) error {
			updatedTo = interval
			return nil
		},
	}
	sched := &mockScheduler{}
	b := newTestBot(store, sched)

	mc := &mockContext{
		messageFn: func() *tele.Message {
			return &tele.Message{Payload: "prod-api 5m"}
		},
	}

	if err := b.handleInterval(mc); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if updatedTo != 300 {
		t.Errorf("interval updated to %d, want 300", updatedTo)
	}
	if sched.addCalls != 0 {
		t.Errorf("scheduler Add calls = %d, want 0 (paused endpoints must not get a job)", sched.addCalls)
	}
	if !strings.Contains(mc.sentMessages[0], "Updated interval") {
		t.Errorf("expected confirmation, got: %s", mc.sentMessages[0])
	}
}

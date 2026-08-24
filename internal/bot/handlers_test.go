package bot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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
			wantContains: []string{"healthy"},
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
				setEndpointPausedFn: func(_ context.Context, _ int64, _ bool, _ sql.NullTime) error {
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

func TestHandlePauseWithDuration(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "ok"}

	var untilSet sql.NullTime
	store := &mockStore{
		getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
			return ep, nil
		},
		setEndpointPausedFn: func(_ context.Context, _ int64, paused bool, until sql.NullTime) error {
			untilSet = until
			return nil
		},
	}
	sched := &mockScheduler{}
	b := newTestBot(store, sched)

	mc := &mockContext{
		messageFn: func() *tele.Message {
			return &tele.Message{Payload: "prod-api 2h"}
		},
	}

	if err := b.handlePause(mc); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !untilSet.Valid {
		t.Fatal("expected paused_until to be set for a timed pause")
	}
	if d := time.Until(untilSet.Time); d < time.Hour || d > 2*time.Hour {
		t.Errorf("paused_until should be ~2h from now, got %v", d)
	}
	if !strings.Contains(mc.sentMessages[0], "resumes automatically") {
		t.Errorf("expected timed-pause confirmation, got: %s", mc.sentMessages[0])
	}
}

func TestHandlePauseInvalidDuration(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "ok"}
	store := &mockStore{
		getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
			return ep, nil
		},
	}
	b := newTestBot(store, &mockScheduler{})

	mc := &mockContext{
		messageFn: func() *tele.Message {
			return &tele.Message{Payload: "prod-api xyz"}
		},
	}

	if err := b.handlePause(mc); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !strings.Contains(mc.sentMessages[0], "Invalid duration") {
		t.Errorf("expected invalid-duration message, got: %s", mc.sentMessages[0])
	}
}

func TestHandleExpect(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "ok"}

	var setTo int
	store := &mockStore{
		getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
			return ep, nil
		},
		setExpectedStatusFn: func(_ context.Context, _ int64, code int) error {
			setTo = code
			return nil
		},
	}
	b := newTestBot(store, &mockScheduler{})

	tests := []struct {
		payload      string
		wantCode     int
		wantContains string
	}{
		{"prod-api 200", 200, "exactly <b>HTTP 200</b>"},
		{"prod-api any", 0, "any 2xx"},
		{"prod-api 99", 0, "Invalid status code"},
		{"", 0, "Usage: /expect"},
	}
	for _, tt := range tests {
		setTo = -1
		mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: tt.payload} }}
		if err := b.handleExpect(mc); err != nil {
			t.Fatalf("payload %q: %v", tt.payload, err)
		}
		if !strings.Contains(mc.sentMessages[0], tt.wantContains) {
			t.Errorf("payload %q: message should contain %q, got: %s", tt.payload, tt.wantContains, mc.sentMessages[0])
		}
		if tt.wantContains == "exactly <b>HTTP 200</b>" && setTo != 200 {
			t.Errorf("payload %q: SetExpectedStatus called with %d, want 200", tt.payload, setTo)
		}
	}
}

func TestHandleKeyword(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "ok"}

	var setTo string
	store := &mockStore{
		getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
			return ep, nil
		},
		setExpectedKeywordFn: func(_ context.Context, _ int64, kw string) error {
			setTo = kw
			return nil
		},
	}
	b := newTestBot(store, &mockScheduler{})

	mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: `prod-api "status":"ok"`} }}
	if err := b.handleKeyword(mc); err != nil {
		t.Fatalf("handleKeyword: %v", err)
	}
	if setTo != `"status":"ok"` {
		t.Errorf("keyword set to %q", setTo)
	}
	if !strings.Contains(mc.sentMessages[0], "Keyword check") {
		t.Errorf("got: %s", mc.sentMessages[0])
	}

	// Regex specs are accepted when they compile.
	mc = &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: `prod-api re:version-[0-9]+`} }}
	if err := b.handleKeyword(mc); err != nil {
		t.Fatalf("handleKeyword regex: %v", err)
	}
	if setTo != `re:version-[0-9]+` {
		t.Errorf("keyword set to %q", setTo)
	}

	// Invalid regex is rejected before persisting.
	setTo = ""
	mc = &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: `prod-api re:[unclosed`} }}
	if err := b.handleKeyword(mc); err != nil {
		t.Fatalf("handleKeyword invalid regex: %v", err)
	}
	if setTo != "" {
		t.Errorf("invalid regex should not be persisted, got %q", setTo)
	}
	if !strings.Contains(mc.sentMessages[0], "Invalid regex") {
		t.Errorf("got: %s", mc.sentMessages[0])
	}

	// Negated substring is accepted.
	mc = &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: `prod-api !fatal error`} }}
	if err := b.handleKeyword(mc); err != nil {
		t.Fatalf("handleKeyword negated: %v", err)
	}
	if setTo != "!fatal error" {
		t.Errorf("keyword set to %q", setTo)
	}

	mc = &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: "prod-api off"} }}
	if err := b.handleKeyword(mc); err != nil {
		t.Fatalf("handleKeyword off: %v", err)
	}
	if setTo != "" {
		t.Errorf("keyword should be cleared, got %q", setTo)
	}
	if !strings.Contains(mc.sentMessages[0], "disabled") {
		t.Errorf("got: %s", mc.sentMessages[0])
	}
}

func TestHandleRename(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "ok"}

	var newName string
	store := &mockStore{
		getEndpointByNameFn: func(_ context.Context, name string) (storage.Endpoint, error) {
			if name == "prod-api" {
				return ep, nil
			}
			return storage.Endpoint{}, apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("not found"))
		},
		renameEndpointFn: func(_ context.Context, _ int64, name string) error {
			newName = name
			return nil
		},
	}
	b := newTestBot(store, &mockScheduler{})

	mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: "prod-api api-v2"} }}
	if err := b.handleRename(mc); err != nil {
		t.Fatalf("handleRename: %v", err)
	}
	if newName != "api-v2" {
		t.Errorf("renamed to %q, want api-v2", newName)
	}
	if !strings.Contains(mc.sentMessages[0], "Renamed") {
		t.Errorf("got: %s", mc.sentMessages[0])
	}

	// Invalid new name rejected
	mc = &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: "prod-api bad!name"} }}
	if err := b.handleRename(mc); err != nil {
		t.Fatalf("handleRename: %v", err)
	}
	if !strings.Contains(mc.sentMessages[0], "Name must") {
		t.Errorf("expected validation error, got: %s", mc.sentMessages[0])
	}
}

func TestHandlePauseAll(t *testing.T) {
	eps := []storage.Endpoint{
		{ID: 1, Name: "a", URL: "https://a.com", Status: "ok"},
		{ID: 2, Name: "b", URL: "https://b.com", Status: "ok"},
		{ID: 3, Name: "c", URL: "https://c.com", Status: "ok", Paused: true},
	}
	pausedIDs := map[int64]bool{}
	store := &mockStore{
		listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
			return eps, nil
		},
		setEndpointPausedFn: func(_ context.Context, id int64, paused bool, _ sql.NullTime) error {
			pausedIDs[id] = paused
			return nil
		},
	}
	sched := &mockScheduler{}
	b := newTestBot(store, sched)

	mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: "all"} }}
	if err := b.handlePause(mc); err != nil {
		t.Fatalf("handlePause all: %v", err)
	}
	if len(pausedIDs) != 2 {
		t.Errorf("paused %d endpoints, want 2 (one was already paused)", len(pausedIDs))
	}
	if sched.removeCalls != 2 {
		t.Errorf("scheduler Remove calls = %d, want 2", sched.removeCalls)
	}
	if !strings.Contains(mc.sentMessages[0], "paused 2 endpoint(s)") {
		t.Errorf("got: %s", mc.sentMessages[0])
	}
}

func TestHandleUptime(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "ok"}
	store := &mockStore{
		getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
			return ep, nil
		},
		getCheckStatsFn: func(_ context.Context, _ int64, since time.Time) (storage.WindowStats, error) {
			return storage.WindowStats{Total: 100, Up: 99, AvgLatencyMs: 45, P95LatencyMs: 120, Incidents: 1}, nil
		},
	}
	b := newTestBot(store, &mockScheduler{})

	mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: "prod-api"} }}
	if err := b.handleUptime(mc); err != nil {
		t.Fatalf("handleUptime: %v", err)
	}
	for _, want := range []string{"prod-api", "99.00%", "p95 120ms", "1 incident"} {
		if !strings.Contains(mc.sentMessages[0], want) {
			t.Errorf("uptime message should contain %q, got: %s", want, mc.sentMessages[0])
		}
	}
}

func TestHandleIncidents(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "ok"}
	now := time.Now()
	store := &mockStore{
		getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) {
			return ep, nil
		},
		getRecentTransitionsFn: func(_ context.Context, _ int64, _ int) ([]storage.CheckTransition, error) {
			return []storage.CheckTransition{
				{CheckedAt: now.Add(-2 * time.Hour), Up: true, StatusCode: 200},
				{CheckedAt: now.Add(-time.Hour), Up: false, StatusCode: 503},
				{CheckedAt: now.Add(-30 * time.Minute), Up: true, StatusCode: 200},
			}, nil
		},
	}
	b := newTestBot(store, &mockScheduler{})

	mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: "prod-api"} }}
	if err := b.handleIncidents(mc); err != nil {
		t.Fatalf("handleIncidents: %v", err)
	}
	for _, want := range []string{"prod-api", "30m", "HTTP 503"} {
		if !strings.Contains(mc.sentMessages[0], want) {
			t.Errorf("incidents message should contain %q, got: %s", want, mc.sentMessages[0])
		}
	}
}

func TestHandleMaint(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "ok"}

	t.Run("add per-endpoint", func(t *testing.T) {
		var gotDays string
		var gotStart, gotEnd int
		var gotEndpointID sql.NullInt64
		store := &mockStore{
			getEndpointByNameFn: func(_ context.Context, _ string) (storage.Endpoint, error) { return ep, nil },
			addMaintenanceWindowFn: func(_ context.Context, endpointID sql.NullInt64, days string, start, end int) (storage.MaintenanceWindow, error) {
				gotEndpointID, gotDays, gotStart, gotEnd = endpointID, days, start, end
				return storage.MaintenanceWindow{ID: 1, EndpointID: endpointID, Days: days, StartMinutes: start, EndMinutes: end}, nil
			},
		}
		b := newTestBot(store, &mockScheduler{})
		mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: "add prod-api sat,sun 02:00-04:00"} }}
		if err := b.handleMaint(mc); err != nil {
			t.Fatalf("handleMaint: %v", err)
		}
		if !gotEndpointID.Valid || gotEndpointID.Int64 != 1 {
			t.Errorf("endpointID = %+v, want {1 true}", gotEndpointID)
		}
		if gotDays != "sat,sun" || gotStart != 120 || gotEnd != 240 {
			t.Errorf("got days=%q start=%d end=%d", gotDays, gotStart, gotEnd)
		}
		if !strings.Contains(mc.sentMessages[0], "Maintenance window #1") {
			t.Errorf("got: %s", mc.sentMessages[0])
		}
	})

	t.Run("add global", func(t *testing.T) {
		var gotEndpointID sql.NullInt64
		store := &mockStore{
			addMaintenanceWindowFn: func(_ context.Context, endpointID sql.NullInt64, days string, start, end int) (storage.MaintenanceWindow, error) {
				gotEndpointID = endpointID
				return storage.MaintenanceWindow{ID: 2, EndpointID: endpointID, Days: days, StartMinutes: start, EndMinutes: end}, nil
			},
		}
		b := newTestBot(store, &mockScheduler{})
		mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: "add all all 22:00-02:00"} }}
		if err := b.handleMaint(mc); err != nil {
			t.Fatalf("handleMaint: %v", err)
		}
		if gotEndpointID.Valid {
			t.Errorf("endpointID = %+v, want NULL for 'all'", gotEndpointID)
		}
		if !strings.Contains(mc.sentMessages[0], "all endpoints") {
			t.Errorf("got: %s", mc.sentMessages[0])
		}
	})

	t.Run("add invalid args", func(t *testing.T) {
		store := &mockStore{}
		b := newTestBot(store, &mockScheduler{})
		for _, payload := range []string{
			"add prod-api funday 02:00-04:00",   // invalid day
			"add prod-api sat 02:00-02:00",      // zero-length window
			"add prod-api sat 25:00-04:00",      // invalid hour
			"add prod-api sat 0200-0400",        // bad format
			"add prod-api sat",                  // missing time range
		} {
			mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: payload} }}
			if err := b.handleMaint(mc); err != nil {
				t.Fatalf("handleMaint(%q): %v", payload, err)
			}
			if len(mc.sentMessages) == 0 {
				t.Errorf("handleMaint(%q): expected an error reply", payload)
			}
		}
	})

	t.Run("list", func(t *testing.T) {
		store := &mockStore{
			listMaintenanceWindowsFn: func(_ context.Context) ([]storage.MaintenanceWindow, error) {
				return []storage.MaintenanceWindow{
					{ID: 1, Days: "all", StartMinutes: 60, EndMinutes: 120},
					{ID: 2, EndpointID: sql.NullInt64{Int64: 1, Valid: true}, Days: "sat,sun", StartMinutes: 1320, EndMinutes: 240},
				}, nil
			},
			listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
				return []storage.Endpoint{ep}, nil
			},
		}
		b := newTestBot(store, &mockScheduler{})
		mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: "list"} }}
		if err := b.handleMaint(mc); err != nil {
			t.Fatalf("handleMaint: %v", err)
		}
		got := mc.sentMessages[0]
		for _, want := range []string{"#1", "all endpoints", "#2", "prod-api", "22:00–04:00"} {
			if !strings.Contains(got, want) {
				t.Errorf("list output missing %q: %s", want, got)
			}
		}
	})

	t.Run("del", func(t *testing.T) {
		var deletedID int64
		store := &mockStore{
			deleteMaintenanceWindowFn: func(_ context.Context, id int64) error {
				deletedID = id
				return nil
			},
		}
		b := newTestBot(store, &mockScheduler{})
		mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: "del 5"} }}
		if err := b.handleMaint(mc); err != nil {
			t.Fatalf("handleMaint: %v", err)
		}
		if deletedID != 5 {
			t.Errorf("deletedID = %d, want 5", deletedID)
		}
	})

	t.Run("del not found", func(t *testing.T) {
		store := &mockStore{
			deleteMaintenanceWindowFn: func(_ context.Context, _ int64) error {
				return apperror.Wrap(apperror.ErrNotFound, errors.New("no row"))
			},
		}
		b := newTestBot(store, &mockScheduler{})
		mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{Payload: "del 99"} }}
		if err := b.handleMaint(mc); err != nil {
			t.Fatalf("handleMaint: %v", err)
		}
		if !strings.Contains(mc.sentMessages[0], "not found") {
			t.Errorf("got: %s", mc.sentMessages[0])
		}
	})
}

func TestHandleDigest(t *testing.T) {
	t.Run("with endpoints", func(t *testing.T) {
		store := &mockStore{
			listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
				return []storage.Endpoint{{ID: 1, Name: "prod-api", Status: "ok"}}, nil
			},
			getCheckStatsFn: func(_ context.Context, _ int64, _ time.Time) (storage.WindowStats, error) {
				return storage.WindowStats{Total: 10, Up: 10, AvgLatencyMs: 100}, nil
			},
		}
		b := newTestBot(store, &mockScheduler{})
		mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{} }}
		if err := b.handleDigest(mc); err != nil {
			t.Fatalf("handleDigest: %v", err)
		}
		if !strings.Contains(mc.sentMessages[0], "Noroshi digest") || !strings.Contains(mc.sentMessages[0], "prod-api") {
			t.Errorf("got: %s", mc.sentMessages[0])
		}
	})

	t.Run("no endpoints", func(t *testing.T) {
		store := &mockStore{
			listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) { return nil, nil },
		}
		b := newTestBot(store, &mockScheduler{})
		mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{} }}
		if err := b.handleDigest(mc); err != nil {
			t.Fatalf("handleDigest: %v", err)
		}
		if !strings.Contains(mc.sentMessages[0], "No active endpoints") {
			t.Errorf("got: %s", mc.sentMessages[0])
		}
	})
}

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

func TestHandleDetailCallback(t *testing.T) {
	testEndpoint := storage.Endpoint{
		ID:              1,
		Name:            "prod-api",
		URL:             "https://example.com",
		IntervalSeconds: 60,
		Status:          "ok",
	}
	notFoundErr := apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("not found"))

	tests := []struct {
		name             string
		callbackData     string
		store            *mockStore
		wantEditContains []string
		wantRespondText  string
		wantNoEdit       bool
	}{
		{
			name:         "happy path",
			callbackData: "1",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
			},
			wantEditContains: []string{"prod-api", "https://example.com"},
		},
		{
			name:            "invalid ID",
			callbackData:    "notanumber",
			store:           &mockStore{},
			wantRespondText: "Invalid endpoint",
			wantNoEdit:      true,
		},
		{
			name:         "not found",
			callbackData: "999",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, _ int64) (storage.Endpoint, error) {
					return storage.Endpoint{}, notFoundErr
				},
			},
			wantRespondText: "not found",
			wantNoEdit:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBot(tt.store, nil)
			mc := &mockContext{
				callbackFn: func() *tele.Callback {
					return &tele.Callback{Data: tt.callbackData}
				},
			}

			err := b.handleDetailCallback(mc)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if tt.wantNoEdit {
				if len(mc.editedMessages) != 0 {
					t.Errorf("expected no edits, got %d: %v", len(mc.editedMessages), mc.editedMessages)
				}
			} else {
				if len(mc.editedMessages) == 0 {
					t.Fatal("expected an edited message, got none")
				}
				for _, want := range tt.wantEditContains {
					if !strings.Contains(mc.editedMessages[0], want) {
						t.Errorf("edited message should contain %q, got: %s", want, mc.editedMessages[0])
					}
				}
			}

			if tt.wantRespondText != "" {
				if len(mc.respondCalls) == 0 {
					t.Fatal("expected a respond call, got none")
				}
				if !strings.Contains(mc.respondCalls[0].Text, tt.wantRespondText) {
					t.Errorf("respond text should contain %q, got: %s", tt.wantRespondText, mc.respondCalls[0].Text)
				}
			}
		})
	}
}

func TestHandleDeleteCallback(t *testing.T) {
	testEndpoint := storage.Endpoint{
		ID:              1,
		Name:            "prod-api",
		URL:             "https://example.com",
		IntervalSeconds: 60,
		Status:          "ok",
	}
	notFoundErr := apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("not found"))

	tests := []struct {
		name             string
		callbackData     string
		store            *mockStore
		wantEditContains []string
		wantRespondText  string
		wantNoEdit       bool
	}{
		{
			name:         "happy path shows confirmation",
			callbackData: "1",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
			},
			wantEditContains: []string{"Delete endpoint?", "prod-api"},
		},
		{
			name:            "invalid ID",
			callbackData:    "abc",
			store:           &mockStore{},
			wantRespondText: "Invalid endpoint",
			wantNoEdit:      true,
		},
		{
			name:         "not found",
			callbackData: "999",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, _ int64) (storage.Endpoint, error) {
					return storage.Endpoint{}, notFoundErr
				},
			},
			wantRespondText: "not found",
			wantNoEdit:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBot(tt.store, nil)
			mc := &mockContext{
				callbackFn: func() *tele.Callback {
					return &tele.Callback{Data: tt.callbackData}
				},
			}

			err := b.handleDeleteCallback(mc)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if tt.wantNoEdit {
				if len(mc.editedMessages) != 0 {
					t.Errorf("expected no edits, got %d: %v", len(mc.editedMessages), mc.editedMessages)
				}
			} else {
				if len(mc.editedMessages) == 0 {
					t.Fatal("expected an edited message, got none")
				}
				for _, want := range tt.wantEditContains {
					if !strings.Contains(mc.editedMessages[0], want) {
						t.Errorf("edited message should contain %q, got: %s", want, mc.editedMessages[0])
					}
				}
			}

			if tt.wantRespondText != "" {
				if len(mc.respondCalls) == 0 {
					t.Fatal("expected a respond call, got none")
				}
				if !strings.Contains(mc.respondCalls[0].Text, tt.wantRespondText) {
					t.Errorf("respond text should contain %q, got: %s", tt.wantRespondText, mc.respondCalls[0].Text)
				}
			}
		})
	}
}

func TestHandleConfirmDeleteCallback(t *testing.T) {
	testEndpoint := storage.Endpoint{
		ID:              1,
		Name:            "prod-api",
		URL:             "https://example.com",
		IntervalSeconds: 60,
		Status:          "ok",
	}
	remainingEndpoint := storage.Endpoint{
		ID:              2,
		Name:            "staging-api",
		URL:             "https://staging.example.com",
		IntervalSeconds: 120,
		Status:          "ok",
	}
	notFoundErr := apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("not found"))

	tests := []struct {
		name             string
		callbackData     string
		store            *mockStore
		scheduler        *mockScheduler
		wantEditContains []string
		wantRespondText  string
		wantNoEdit       bool
		wantRemoveCalls  int
	}{
		{
			name:         "happy path deletes and shows empty list",
			callbackData: "1",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return nil, nil
				},
			},
			wantRespondText:  "Deleted!",
			wantEditContains: []string{"No endpoints"},
		},
		{
			name:         "happy path with remaining endpoints",
			callbackData: "1",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return []storage.Endpoint{remainingEndpoint}, nil
				},
			},
			wantRespondText:  "Deleted!",
			wantEditContains: []string{"healthy"},
		},
		{
			name:            "invalid ID",
			callbackData:    "xyz",
			store:           &mockStore{},
			wantRespondText: "Invalid endpoint",
			wantNoEdit:      true,
		},
		{
			name:         "not found",
			callbackData: "999",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, _ int64) (storage.Endpoint, error) {
					return storage.Endpoint{}, notFoundErr
				},
			},
			wantRespondText: "not found",
			wantNoEdit:      true,
		},
		{
			name:         "delete error",
			callbackData: "1",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
				deleteEndpointFn: func(_ context.Context, _ int64) error {
					return fmt.Errorf("db error")
				},
			},
			wantRespondText: "Error deleting",
			wantNoEdit:      true,
		},
		{
			name:         "with scheduler",
			callbackData: "1",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return nil, nil
				},
			},
			scheduler:        &mockScheduler{},
			wantRespondText:  "Deleted!",
			wantEditContains: []string{"No endpoints"},
			wantRemoveCalls:  1,
		},
		{
			name:         "nil scheduler",
			callbackData: "1",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return nil, nil
				},
			},
			scheduler:        nil,
			wantRespondText:  "Deleted!",
			wantEditContains: []string{"No endpoints"},
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
				callbackFn: func() *tele.Callback {
					return &tele.Callback{Data: tt.callbackData}
				},
			}

			err := b.handleConfirmDeleteCallback(mc)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if tt.wantNoEdit {
				if len(mc.editedMessages) != 0 {
					t.Errorf("expected no edits, got %d: %v", len(mc.editedMessages), mc.editedMessages)
				}
			} else {
				if len(mc.editedMessages) == 0 {
					t.Fatal("expected an edited message, got none")
				}
				for _, want := range tt.wantEditContains {
					if !strings.Contains(mc.editedMessages[0], want) {
						t.Errorf("edited message should contain %q, got: %s", want, mc.editedMessages[0])
					}
				}
			}

			if tt.wantRespondText != "" {
				found := false
				for _, rc := range mc.respondCalls {
					if strings.Contains(rc.Text, tt.wantRespondText) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected respond call containing %q, got: %v", tt.wantRespondText, mc.respondCalls)
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

func TestHandleBackCallback(t *testing.T) {
	tests := []struct {
		name             string
		store            *mockStore
		wantEditContains []string
	}{
		{
			name: "returns to list",
			store: &mockStore{
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return []storage.Endpoint{
						{ID: 1, Name: "site-a", URL: "https://a.com", Status: "ok"},
						{ID: 2, Name: "site-b", URL: "https://b.com", Status: "ok"},
					}, nil
				},
			},
			wantEditContains: []string{"healthy"},
		},
		{
			name:             "empty list",
			store:            &mockStore{},
			wantEditContains: []string{"No endpoints"},
		},
		{
			name: "store error",
			store: &mockStore{
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return nil, fmt.Errorf("db error")
				},
			},
			wantEditContains: []string{"Internal error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBot(tt.store, nil)
			mc := &mockContext{
				callbackFn: func() *tele.Callback {
					return &tele.Callback{}
				},
			}

			err := b.handleBackCallback(mc)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if len(mc.editedMessages) == 0 {
				t.Fatal("expected an edited message, got none")
			}
			for _, want := range tt.wantEditContains {
				if !strings.Contains(mc.editedMessages[0], want) {
					t.Errorf("edited message should contain %q, got: %s", want, mc.editedMessages[0])
				}
			}
		})
	}
}

func TestHandleIntervalCallback(t *testing.T) {
	testEndpoint := storage.Endpoint{
		ID:              1,
		Name:            "prod-api",
		URL:             "https://example.com",
		IntervalSeconds: 60,
		Status:          "ok",
	}
	notFoundErr := apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("not found"))

	tests := []struct {
		name             string
		callbackData     string
		store            *mockStore
		wantEditContains []string
		wantRespondText  string
		wantNoEdit       bool
	}{
		{
			name:         "happy path shows interval buttons",
			callbackData: "1",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
			},
			wantEditContains: []string{"Change interval", "prod-api"},
		},
		{
			name:            "invalid ID",
			callbackData:    "abc",
			store:           &mockStore{},
			wantRespondText: "Invalid endpoint",
			wantNoEdit:      true,
		},
		{
			name:         "not found",
			callbackData: "999",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, _ int64) (storage.Endpoint, error) {
					return storage.Endpoint{}, notFoundErr
				},
			},
			wantRespondText: "not found",
			wantNoEdit:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBot(tt.store, nil)
			mc := &mockContext{
				callbackFn: func() *tele.Callback {
					return &tele.Callback{Data: tt.callbackData}
				},
			}

			err := b.handleIntervalCallback(mc)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if tt.wantNoEdit {
				if len(mc.editedMessages) != 0 {
					t.Errorf("expected no edits, got %d: %v", len(mc.editedMessages), mc.editedMessages)
				}
			} else {
				if len(mc.editedMessages) == 0 {
					t.Fatal("expected an edited message, got none")
				}
				for _, want := range tt.wantEditContains {
					if !strings.Contains(mc.editedMessages[0], want) {
						t.Errorf("edited message should contain %q, got: %s", want, mc.editedMessages[0])
					}
				}
			}

			if tt.wantRespondText != "" {
				if len(mc.respondCalls) == 0 {
					t.Fatal("expected a respond call, got none")
				}
				if !strings.Contains(mc.respondCalls[0].Text, tt.wantRespondText) {
					t.Errorf("respond text should contain %q, got: %s", tt.wantRespondText, mc.respondCalls[0].Text)
				}
			}
		})
	}
}

func TestHandleSetIntervalCallback(t *testing.T) {
	testEndpoint := storage.Endpoint{
		ID:              1,
		Name:            "prod-api",
		URL:             "https://example.com",
		IntervalSeconds: 60,
		Status:          "ok",
	}
	notFoundErr := apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("not found"))

	tests := []struct {
		name             string
		callbackData     string
		store            *mockStore
		scheduler        *mockScheduler
		wantEditContains []string
		wantRespondText  string
		wantNoEdit       bool
		wantRemoveCalls  int
		wantAddCalls     int
	}{
		{
			name:         "happy path updates and shows list",
			callbackData: "1|300",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return []storage.Endpoint{testEndpoint}, nil
				},
			},
			wantRespondText:  "5m",
			wantEditContains: []string{"healthy"},
		},
		{
			name:            "invalid data format",
			callbackData:    "nopipe",
			store:           &mockStore{},
			wantRespondText: "Invalid data",
			wantNoEdit:      true,
		},
		{
			name:            "invalid endpoint ID",
			callbackData:    "abc|300",
			store:           &mockStore{},
			wantRespondText: "Invalid endpoint",
			wantNoEdit:      true,
		},
		{
			name:            "invalid seconds",
			callbackData:    "1|notanumber",
			store:           &mockStore{},
			wantRespondText: "Invalid interval",
			wantNoEdit:      true,
		},
		{
			name:         "not found",
			callbackData: "999|300",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, _ int64) (storage.Endpoint, error) {
					return storage.Endpoint{}, notFoundErr
				},
			},
			wantRespondText: "not found",
			wantNoEdit:      true,
		},
		{
			name:         "update error",
			callbackData: "1|300",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
				updateEndpointIntervalFn: func(_ context.Context, _ int64, _ int) error {
					return fmt.Errorf("db error")
				},
			},
			wantRespondText: "Error updating",
			wantNoEdit:      true,
		},
		{
			name:         "with scheduler",
			callbackData: "1|300",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return []storage.Endpoint{testEndpoint}, nil
				},
			},
			scheduler:        &mockScheduler{},
			wantRespondText:  "5m",
			wantEditContains: []string{"healthy"},
			wantRemoveCalls:  1,
			wantAddCalls:     1,
		},
		{
			name:         "nil scheduler",
			callbackData: "1|300",
			store: &mockStore{
				getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
					if id == 1 {
						return testEndpoint, nil
					}
					return storage.Endpoint{}, notFoundErr
				},
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return []storage.Endpoint{testEndpoint}, nil
				},
			},
			scheduler:        nil,
			wantRespondText:  "5m",
			wantEditContains: []string{"healthy"},
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
				callbackFn: func() *tele.Callback {
					return &tele.Callback{Data: tt.callbackData}
				},
			}

			err := b.handleSetIntervalCallback(mc)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if tt.wantNoEdit {
				if len(mc.editedMessages) != 0 {
					t.Errorf("expected no edits, got %d: %v", len(mc.editedMessages), mc.editedMessages)
				}
			} else {
				if len(mc.editedMessages) == 0 {
					t.Fatal("expected an edited message, got none")
				}
				for _, want := range tt.wantEditContains {
					if !strings.Contains(mc.editedMessages[0], want) {
						t.Errorf("edited message should contain %q, got: %s", want, mc.editedMessages[0])
					}
				}
			}

			if tt.wantRespondText != "" {
				found := false
				for _, rc := range mc.respondCalls {
					if strings.Contains(rc.Text, tt.wantRespondText) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected respond call containing %q, got: %v", tt.wantRespondText, mc.respondCalls)
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

func TestHandleRefreshCallback(t *testing.T) {
	tests := []struct {
		name             string
		store            *mockStore
		wantEditContains []string
	}{
		{
			name: "refreshes list",
			store: &mockStore{
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return []storage.Endpoint{
						{ID: 1, Name: "site-a", URL: "https://a.com", Status: "ok"},
						{ID: 2, Name: "site-b", URL: "https://b.com", Status: "ok"},
					}, nil
				},
			},
			wantEditContains: []string{"healthy"},
		},
		{
			name:             "empty list",
			store:            &mockStore{},
			wantEditContains: []string{"No endpoints"},
		},
		{
			name: "store error",
			store: &mockStore{
				listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
					return nil, fmt.Errorf("db error")
				},
			},
			wantEditContains: []string{"Internal error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBot(tt.store, nil)
			mc := &mockContext{
				callbackFn: func() *tele.Callback {
					return &tele.Callback{}
				},
			}

			err := b.handleRefreshCallback(mc)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if len(mc.editedMessages) == 0 {
				t.Fatal("expected an edited message, got none")
			}
			for _, want := range tt.wantEditContains {
				if !strings.Contains(mc.editedMessages[0], want) {
					t.Errorf("edited message should contain %q, got: %s", want, mc.editedMessages[0])
				}
			}
		})
	}
}

func TestHandleCheckNowCallback(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "unknown"}
	checked := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "ok", LastStatusCode: 200, LastLatencyMs: 42}

	store := &mockStore{
		getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
			return ep, nil
		},
	}
	sched := &mockScheduler{
		checkNowFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
			return checked, nil
		},
	}
	b := newTestBot(store, sched)

	mc := &mockContext{
		callbackFn: func() *tele.Callback {
			return &tele.Callback{Data: "1"}
		},
	}

	if err := b.handleCheckNowCallback(mc); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if len(mc.editedMessages) == 0 {
		t.Fatal("expected the detail view to be edited with fresh results")
	}
	if !strings.Contains(mc.editedMessages[0], "prod-api") {
		t.Errorf("edited message should contain endpoint name, got: %s", mc.editedMessages[0])
	}
	if len(mc.respondCalls) == 0 || mc.respondCalls[0].Text != "Checking..." {
		t.Errorf("expected a 'Checking...' response, got: %+v", mc.respondCalls)
	}
}

func TestHandlePauseCallback(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", Status: "ok"}

	var pausedSet bool
	store := &mockStore{
		getEndpointFn: func(_ context.Context, id int64) (storage.Endpoint, error) {
			return ep, nil
		},
		setEndpointPausedFn: func(_ context.Context, _ int64, paused bool) error {
			pausedSet = paused
			return nil
		},
	}
	sched := &mockScheduler{}
	b := newTestBot(store, sched)

	mc := &mockContext{
		callbackFn: func() *tele.Callback {
			return &tele.Callback{Data: "1"}
		},
	}

	if err := b.handlePauseCallback(mc); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !pausedSet {
		t.Error("expected SetEndpointPaused(true)")
	}
	if sched.removeCalls != 1 {
		t.Errorf("scheduler Remove calls = %d, want 1 (job stopped on pause)", sched.removeCalls)
	}
	if len(mc.editedMessages) == 0 {
		t.Fatal("expected the detail view to be re-rendered")
	}
}

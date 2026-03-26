package bot

import (
	"context"

	"noroshi/internal/storage"

	tele "gopkg.in/telebot.v4"
)

// newTestBot creates a Bot for testing without a real Telegram connection.
// Handlers never reference b.bot, so it can be nil.
func newTestBot(store Store, scheduler Scheduler) *Bot {
	return &Bot{
		store:     store,
		scheduler: scheduler,
		chatID:    123,
		rootCtx:   context.Background(),
	}
}

// mockContext implements tele.Context for handler testing.
// Input functions control what the handler reads; output slices capture what it writes.
type mockContext struct {
	tele.Context // embedded — panics on any unimplemented method

	// Input function fields.
	messageFn  func() *tele.Message
	callbackFn func() *tele.Callback
	chatFn     func() *tele.Chat

	// Output capture fields.
	sentMessages   []string
	editedMessages []string
	respondCalls   []tele.CallbackResponse
	sendOpts       [][]interface{}
}

func (m *mockContext) Message() *tele.Message {
	if m.messageFn != nil {
		return m.messageFn()
	}
	return nil
}

func (m *mockContext) Chat() *tele.Chat {
	if m.chatFn != nil {
		return m.chatFn()
	}
	return &tele.Chat{ID: 123}
}

func (m *mockContext) Callback() *tele.Callback {
	if m.callbackFn != nil {
		return m.callbackFn()
	}
	return nil
}

func (m *mockContext) Send(what interface{}, opts ...interface{}) error {
	if s, ok := what.(string); ok {
		m.sentMessages = append(m.sentMessages, s)
	}
	m.sendOpts = append(m.sendOpts, opts)
	return nil
}

func (m *mockContext) Edit(what interface{}, opts ...interface{}) error {
	if s, ok := what.(string); ok {
		m.editedMessages = append(m.editedMessages, s)
	}
	return nil
}

func (m *mockContext) Respond(resp ...*tele.CallbackResponse) error {
	if len(resp) > 0 && resp[0] != nil {
		m.respondCalls = append(m.respondCalls, *resp[0])
	}
	return nil
}

// mockStore implements bot.Store with function-field delegation.
// Each method delegates to its function field if set, otherwise returns the zero value.
type mockStore struct {
	addEndpointFn            func(ctx context.Context, name, url string, interval int) (storage.Endpoint, error)
	getEndpointFn            func(ctx context.Context, id int64) (storage.Endpoint, error)
	getEndpointByURLFn       func(ctx context.Context, url string) (storage.Endpoint, error)
	getEndpointByNameFn      func(ctx context.Context, name string) (storage.Endpoint, error)
	deleteEndpointFn         func(ctx context.Context, id int64) error
	listEndpointsFn          func(ctx context.Context) ([]storage.Endpoint, error)
	updateEndpointIntervalFn func(ctx context.Context, id int64, interval int) error
}

func (m *mockStore) AddEndpoint(ctx context.Context, name, url string, interval int) (storage.Endpoint, error) {
	if m.addEndpointFn != nil {
		return m.addEndpointFn(ctx, name, url, interval)
	}
	return storage.Endpoint{}, nil
}

func (m *mockStore) GetEndpoint(ctx context.Context, id int64) (storage.Endpoint, error) {
	if m.getEndpointFn != nil {
		return m.getEndpointFn(ctx, id)
	}
	return storage.Endpoint{}, nil
}

func (m *mockStore) GetEndpointByURL(ctx context.Context, url string) (storage.Endpoint, error) {
	if m.getEndpointByURLFn != nil {
		return m.getEndpointByURLFn(ctx, url)
	}
	return storage.Endpoint{}, nil
}

func (m *mockStore) GetEndpointByName(ctx context.Context, name string) (storage.Endpoint, error) {
	if m.getEndpointByNameFn != nil {
		return m.getEndpointByNameFn(ctx, name)
	}
	return storage.Endpoint{}, nil
}

func (m *mockStore) DeleteEndpoint(ctx context.Context, id int64) error {
	if m.deleteEndpointFn != nil {
		return m.deleteEndpointFn(ctx, id)
	}
	return nil
}

func (m *mockStore) ListEndpoints(ctx context.Context) ([]storage.Endpoint, error) {
	if m.listEndpointsFn != nil {
		return m.listEndpointsFn(ctx)
	}
	return nil, nil
}

func (m *mockStore) UpdateEndpointInterval(ctx context.Context, id int64, interval int) error {
	if m.updateEndpointIntervalFn != nil {
		return m.updateEndpointIntervalFn(ctx, id, interval)
	}
	return nil
}

// mockScheduler implements bot.Scheduler with function-field delegation and call counters.
type mockScheduler struct {
	addFn       func(ctx context.Context, ep storage.Endpoint) error
	removeFn    func(endpointID int64) error
	addCalls    int
	removeCalls int
}

func (m *mockScheduler) Add(ctx context.Context, ep storage.Endpoint) error {
	m.addCalls++
	if m.addFn != nil {
		return m.addFn(ctx, ep)
	}
	return nil
}

func (m *mockScheduler) Remove(endpointID int64) error {
	m.removeCalls++
	if m.removeFn != nil {
		return m.removeFn(endpointID)
	}
	return nil
}

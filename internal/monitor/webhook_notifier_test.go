package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"noroshi/internal/storage"
)

// webhookCapture records the last request received by the test server.
type webhookCapture struct {
	mu        sync.Mutex
	payload   alertPayload
	auth      string
	count     int
	failWith  int // respond with this status when > 0
}

func (c *webhookCapture) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.count++
		c.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&c.payload)
		if c.failWith > 0 {
			w.WriteHeader(c.failWith)
		}
	})
}

func (c *webhookCapture) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

func TestWebhookNotifier(t *testing.T) {
	ep := storage.Endpoint{
		ID: 7, Name: "prod-api", URL: "https://example.com",
		LastStatusCode: 503, LastLatencyMs: 42, LastCheckError: "HTTP 503",
	}

	t.Run("failure payload with auth", func(t *testing.T) {
		cap := &webhookCapture{}
		srv := httptest.NewServer(cap.handler())
		defer srv.Close()

		n := NewWebhookNotifier(srv.URL, "secret")
		msgID, err := n.NotifyFailure(context.Background(), ep)
		if err != nil {
			t.Fatalf("NotifyFailure: %v", err)
		}
		if msgID != 0 {
			t.Errorf("msgID = %d, want 0 (webhook has no message IDs)", msgID)
		}
		if cap.auth != "Bearer secret" {
			t.Errorf("Authorization = %q", cap.auth)
		}
		p := cap.payload
		if p.Event != "failure" || p.Endpoint == nil {
			t.Fatalf("payload: %+v", p)
		}
		if p.Endpoint.ID != 7 || p.Endpoint.Name != "prod-api" || p.Endpoint.StatusCode != 503 ||
			p.Endpoint.LatencyMs != 42 || p.Endpoint.Reason != "HTTP 503" {
			t.Errorf("endpoint payload: %+v", p.Endpoint)
		}
		if p.Timestamp.IsZero() {
			t.Error("timestamp should be set")
		}
	})

	t.Run("recovery payload", func(t *testing.T) {
		cap := &webhookCapture{}
		srv := httptest.NewServer(cap.handler())
		defer srv.Close()

		n := NewWebhookNotifier(srv.URL, "")
		if err := n.NotifyRecovery(context.Background(), ep, 5*time.Minute); err != nil {
			t.Fatalf("NotifyRecovery: %v", err)
		}
		if cap.payload.Event != "recovery" || cap.payload.DowntimeSeconds != 300 {
			t.Errorf("payload: %+v", cap.payload)
		}
		if cap.auth != "" {
			t.Errorf("Authorization should be empty without token, got %q", cap.auth)
		}
	})

	t.Run("cert expiry payload", func(t *testing.T) {
		cap := &webhookCapture{}
		srv := httptest.NewServer(cap.handler())
		defer srv.Close()

		n := NewWebhookNotifier(srv.URL, "")
		if err := n.NotifyCertExpiry(context.Background(), ep, 10); err != nil {
			t.Fatalf("NotifyCertExpiry: %v", err)
		}
		if cap.payload.Event != "cert_expiry" || cap.payload.DaysLeft != 10 {
			t.Errorf("payload: %+v", cap.payload)
		}
	})

	t.Run("digest payload", func(t *testing.T) {
		cap := &webhookCapture{}
		srv := httptest.NewServer(cap.handler())
		defer srv.Close()

		n := NewWebhookNotifier(srv.URL, "")
		if err := n.NotifyDigest(context.Background(), "digest text"); err != nil {
			t.Fatalf("NotifyDigest: %v", err)
		}
		if cap.payload.Event != "digest" || cap.payload.Text != "digest text" {
			t.Errorf("payload: %+v", cap.payload)
		}
	})

	t.Run("http error status fails", func(t *testing.T) {
		cap := &webhookCapture{failWith: 500}
		srv := httptest.NewServer(cap.handler())
		defer srv.Close()

		n := NewWebhookNotifier(srv.URL, "")
		if err := n.NotifyDigest(context.Background(), "x"); err == nil {
			t.Error("expected error for HTTP 500")
		}
	})

	t.Run("unreachable fails", func(t *testing.T) {
		n := NewWebhookNotifier("http://127.0.0.1:1", "")
		if err := n.NotifyDigest(context.Background(), "x"); err == nil {
			t.Error("expected error for unreachable webhook")
		}
	})
}

type stubNotifier struct {
	msgID int64
	err   error
	calls *int
}

func (s *stubNotifier) NotifyFailure(_ context.Context, _ storage.Endpoint) (int64, error) {
	*s.calls++
	return s.msgID, s.err
}
func (s *stubNotifier) NotifyRecovery(_ context.Context, _ storage.Endpoint, _ time.Duration) error {
	*s.calls++
	return s.err
}
func (s *stubNotifier) NotifyCertExpiry(_ context.Context, _ storage.Endpoint, _ int) error {
	*s.calls++
	return s.err
}
func (s *stubNotifier) NotifyDigest(_ context.Context, _ string) error {
	*s.calls++
	return s.err
}

func TestMultiNotifier(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "api"}

	t.Run("fans out and returns first message id", func(t *testing.T) {
		var callsA, callsB int
		a := &stubNotifier{msgID: 42, calls: &callsA}
		b := &stubNotifier{calls: &callsB}
		m := NewMultiNotifier(nil, a, b)

		msgID, err := m.NotifyFailure(context.Background(), ep)
		if err != nil {
			t.Fatalf("NotifyFailure: %v", err)
		}
		if msgID != 42 {
			t.Errorf("msgID = %d, want 42", msgID)
		}
		if callsA != 1 || callsB != 1 {
			t.Errorf("calls = %d, %d; want 1, 1", callsA, callsB)
		}
	})

	t.Run("partial failure is tolerated", func(t *testing.T) {
		var callsA, callsB int
		a := &stubNotifier{msgID: 42, calls: &callsA}
		b := &stubNotifier{err: errors.New("webhook down"), calls: &callsB}
		m := NewMultiNotifier(nil, a, b)

		msgID, err := m.NotifyFailure(context.Background(), ep)
		if err != nil {
			t.Errorf("partial failure should not return error: %v", err)
		}
		if msgID != 42 {
			t.Errorf("msgID = %d, want 42 even when the webhook fails", msgID)
		}
		if callsB != 1 {
			t.Error("failing notifier should still be called")
		}
	})

	t.Run("all failing returns error", func(t *testing.T) {
		var callsA, callsB int
		a := &stubNotifier{err: errors.New("a"), calls: &callsA}
		b := &stubNotifier{err: errors.New("b"), calls: &callsB}
		m := NewMultiNotifier(nil, a, b)

		if _, err := m.NotifyFailure(context.Background(), ep); err == nil {
			t.Error("expected error when all notifiers fail")
		} else if !strings.Contains(err.Error(), "a") || !strings.Contains(err.Error(), "b") {
			t.Errorf("joined error should contain both causes: %v", err)
		}
	})

	t.Run("recovery fans out despite failure", func(t *testing.T) {
		var callsA, callsB int
		a := &stubNotifier{calls: &callsA}
		b := &stubNotifier{err: errors.New("down"), calls: &callsB}
		m := NewMultiNotifier(nil, a, b)

		if err := m.NotifyRecovery(context.Background(), ep, time.Minute); err != nil {
			t.Errorf("partial failure should not return error: %v", err)
		}
		if callsA != 1 || callsB != 1 {
			t.Errorf("calls = %d, %d; want 1, 1", callsA, callsB)
		}
	})
}

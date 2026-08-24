package monitor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"noroshi/internal/storage"
)

func TestFormatDigest(t *testing.T) {
	endpoints := []storage.Endpoint{
		{ID: 1, Name: "prod-api", Status: "ok"},
		{ID: 2, Name: "db", Status: "not_ok"},
		{ID: 3, Name: "new<svc>", Status: "unknown"},
	}
	stats := map[int64]storage.WindowStats{
		1: {Total: 100, Up: 100, AvgLatencyMs: 120.4, Incidents: 0},
		2: {Total: 100, Up: 97, AvgLatencyMs: 45, Incidents: 2},
		3: {},
	}

	text := FormatDigest(endpoints, stats, 24*time.Hour)
	for _, want := range []string{
		"last 24h", "3 endpoints · 1 up · 1 down",
		"prod-api</b> — 100.0% up · avg 120ms · 0 incidents",
		"db</b> — 97.0% up · avg 45ms · 2 incidents",
		"new&lt;svc&gt;</b> — no checks in window",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("digest missing %q:\n%s", want, text)
		}
	}

	weekly := FormatDigest(endpoints, stats, 7*24*time.Hour)
	if !strings.Contains(weekly, "last 7d") {
		t.Errorf("weekly digest should say 'last 7d':\n%s", weekly)
	}
}

type digestMockStore struct {
	endpoints []storage.Endpoint
	err       error
}

func (s *digestMockStore) ListEndpoints(_ context.Context) ([]storage.Endpoint, error) {
	return s.endpoints, s.err
}

func (s *digestMockStore) GetCheckStats(_ context.Context, _ int64, _ time.Time) (storage.WindowStats, error) {
	return storage.WindowStats{Total: 10, Up: 10, AvgLatencyMs: 50}, nil
}

func TestBuildDigest(t *testing.T) {
	// Paused endpoints are excluded.
	store := &digestMockStore{endpoints: []storage.Endpoint{
		{ID: 1, Name: "api", Status: "ok"},
		{ID: 2, Name: "db", Status: "ok", Paused: true},
	}}
	text, err := BuildDigest(context.Background(), store, 24*time.Hour)
	if err != nil {
		t.Fatalf("BuildDigest: %v", err)
	}
	if !strings.Contains(text, "1 endpoints") {
		t.Errorf("paused endpoint should be excluded:\n%s", text)
	}
	if strings.Contains(text, "db") {
		t.Errorf("paused endpoint should not appear:\n%s", text)
	}

	// No active endpoints → empty text.
	store = &digestMockStore{endpoints: []storage.Endpoint{{ID: 2, Name: "db", Paused: true}}}
	text, err = BuildDigest(context.Background(), store, 24*time.Hour)
	if err != nil {
		t.Fatalf("BuildDigest: %v", err)
	}
	if text != "" {
		t.Errorf("expected empty digest, got:\n%s", text)
	}

	// Store errors propagate.
	store = &digestMockStore{err: errors.New("db down")}
	if _, err := BuildDigest(context.Background(), store, 24*time.Hour); err == nil {
		t.Error("expected error to propagate")
	}
}

func TestSendDigest(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, Name: "api", URL: "https://example.com", Status: "ok"})
	notifier := &mockNotifier{}

	sched, err := NewScheduler(context.Background(), store, &mockChecker{}, notifier, 3, 1, 0,
		DigestConfig{Mode: "daily", TimeMinutes: 9 * 60}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sched.sendDigest()

	if notifier.digestCount() != 1 {
		t.Fatalf("digests = %d, want 1", notifier.digestCount())
	}
	if !strings.Contains(notifier.digests[0], "api") {
		t.Errorf("digest should mention the endpoint:\n%s", notifier.digests[0])
	}
}

func TestSendDigestSkipsWhenNoEndpoints(t *testing.T) {
	store := newMockStore()
	notifier := &mockNotifier{}

	sched, err := NewScheduler(context.Background(), store, &mockChecker{}, notifier, 3, 1, 0,
		DigestConfig{Mode: "daily", TimeMinutes: 9 * 60}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sched.sendDigest()

	if notifier.digestCount() != 0 {
		t.Errorf("digests = %d, want 0 when there are no endpoints", notifier.digestCount())
	}
}

func TestDigestConfigWindow(t *testing.T) {
	if got := (DigestConfig{Mode: "daily"}).Window(); got != 24*time.Hour {
		t.Errorf("daily window = %v, want 24h", got)
	}
	if got := (DigestConfig{Mode: "weekly"}).Window(); got != 7*24*time.Hour {
		t.Errorf("weekly window = %v, want 168h", got)
	}
}

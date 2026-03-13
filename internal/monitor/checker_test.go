package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckerOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checker := NewChecker(5 * time.Second)
	code, err := checker.Check(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if code != 200 {
		t.Errorf("code = %d, want 200", code)
	}
}

func TestChecker503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	checker := NewChecker(5 * time.Second)
	// With PassthroughErrorHandler, retryablehttp returns the last response
	code, err := checker.Check(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if code != 503 {
		t.Errorf("code = %d, want 503", code)
	}
}

func TestCheckerUnreachable(t *testing.T) {
	checker := NewChecker(1 * time.Second)
	// Use a port that is not listening
	code, err := checker.Check(context.Background(), "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
}

func TestCheckerCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checker := NewChecker(30 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := checker.Check(ctx, srv.URL)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

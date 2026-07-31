package monitor

import (
	"context"
	"crypto/tls"
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

	checker := NewHTTPChecker(5 * time.Second)
	res := checker.Check(context.Background(), srv.URL, CheckOptions{})
	if res.Err != nil {
		t.Fatalf("Check: %v", res.Err)
	}
	if !res.Up {
		t.Errorf("Up = false, want true (reason: %s)", res.Reason)
	}
	if res.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", res.StatusCode)
	}
	if res.Latency <= 0 {
		t.Errorf("Latency = %v, want > 0", res.Latency)
	}
}

func TestChecker503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	checker := NewHTTPChecker(5 * time.Second)
	// With PassthroughErrorHandler, retryablehttp returns the last response
	res := checker.Check(context.Background(), srv.URL, CheckOptions{})
	if res.Err != nil {
		t.Fatalf("Check: %v", res.Err)
	}
	if res.Up {
		t.Error("Up = true, want false for 503")
	}
	if res.StatusCode != 503 {
		t.Errorf("StatusCode = %d, want 503", res.StatusCode)
	}
	if res.Reason == "" {
		t.Error("Reason should be set for a failed check")
	}
}

func TestCheckerExpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	checker := NewHTTPChecker(5 * time.Second)

	// 204 matches the expectation.
	res := checker.Check(context.Background(), srv.URL, CheckOptions{ExpectedStatus: 204})
	if !res.Up {
		t.Errorf("Up = false, want true when expected status matches (reason: %s)", res.Reason)
	}

	// Default (any 2xx) also passes.
	res = checker.Check(context.Background(), srv.URL, CheckOptions{})
	if !res.Up {
		t.Errorf("Up = false, want true for 2xx default (reason: %s)", res.Reason)
	}

	// Expecting exactly 200 fails.
	res = checker.Check(context.Background(), srv.URL, CheckOptions{ExpectedStatus: 200})
	if res.Up {
		t.Error("Up = true, want false when expected status differs")
	}
	if res.Reason != "expected HTTP 200, got 204" {
		t.Errorf("Reason = %q, want %q", res.Reason, "expected HTTP 200, got 204")
	}
}

func TestCheckerKeyword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	checker := NewHTTPChecker(5 * time.Second)

	res := checker.Check(context.Background(), srv.URL, CheckOptions{Keyword: `"status":"ok"`})
	if !res.Up {
		t.Errorf("Up = false, want true when keyword present (reason: %s)", res.Reason)
	}

	res = checker.Check(context.Background(), srv.URL, CheckOptions{Keyword: "healthy"})
	if res.Up {
		t.Error("Up = true, want false when keyword missing")
	}
	if res.Reason != `keyword "healthy" not found` {
		t.Errorf("Reason = %q", res.Reason)
	}
}

func TestCheckerCertExpiry(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checker := NewHTTPChecker(5 * time.Second)
	// Trust the test server's certificate.
	checker.client.HTTPClient.Transport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test only

	res := checker.Check(context.Background(), srv.URL, CheckOptions{})
	if !res.Up {
		t.Fatalf("Up = false, want true (reason: %s)", res.Reason)
	}
	if res.CertExpiry.IsZero() {
		t.Error("CertExpiry should be set for TLS endpoints")
	}
}

func TestCheckerUnreachable(t *testing.T) {
	checker := NewHTTPChecker(1 * time.Second)
	// Use a port that is not listening
	res := checker.Check(context.Background(), "http://127.0.0.1:1", CheckOptions{})
	if res.Err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if res.Up {
		t.Error("Up = true, want false")
	}
	if res.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0", res.StatusCode)
	}
	if res.Reason != "connection error" {
		t.Errorf("Reason = %q, want %q", res.Reason, "connection error")
	}
}

func TestCheckerCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checker := NewHTTPChecker(30 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	res := checker.Check(ctx, srv.URL, CheckOptions{})
	if res.Err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

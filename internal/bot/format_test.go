package bot

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"noroshi/internal/storage"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "0s"},
		{"seconds only", 45 * time.Second, "45s"},
		{"minutes and seconds", 12*time.Minute + 34*time.Second, "12m 34s"},
		{"hours minutes seconds", 2*time.Hour + 15*time.Minute + 30*time.Second, "2h 15m 30s"},
		{"exact minute", 1 * time.Minute, "1m"},
		{"exact hour", 1 * time.Hour, "1h"},
		{"hours and seconds", 1*time.Hour + 5*time.Second, "1h 5s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDuration(tt.d)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestFormatFailureWithCode(t *testing.T) {
	ep := storage.Endpoint{
		URL:                      "https://api.example.com/health",
		FailureNotificationsSent: 2,
	}

	msg := FormatFailureWithCode(ep, 503, 3)

	if !strings.Contains(msg, "🔴 ENDPOINT DOWN") {
		t.Error("should contain failure header")
	}
	if !strings.Contains(msg, "https://api.example.com/health") {
		t.Error("should contain URL")
	}
	if !strings.Contains(msg, "HTTP 503") {
		t.Error("should contain status code")
	}
	if !strings.Contains(msg, "2/3") {
		t.Error("should contain failure count")
	}
}

func TestFormatRecovery(t *testing.T) {
	ep := storage.Endpoint{
		URL: "https://api.example.com/health",
	}
	downtime := 12*time.Minute + 34*time.Second

	msg := FormatRecovery(ep, downtime)

	if !strings.Contains(msg, "🟢 ENDPOINT RECOVERED") {
		t.Error("should contain recovery header")
	}
	if !strings.Contains(msg, "https://api.example.com/health") {
		t.Error("should contain URL")
	}
	if !strings.Contains(msg, "12m 34s") {
		t.Error("should contain downtime")
	}
}

func TestFormatEndpointListEmpty(t *testing.T) {
	msg := FormatEndpointList(nil)
	if msg != "No endpoints are being monitored." {
		t.Errorf("got %q, want empty message", msg)
	}
}

func TestFormatEndpointListSingle(t *testing.T) {
	eps := []storage.Endpoint{
		{
			URL:             "https://example.com",
			IntervalSeconds: 30,
			Status:          "ok",
			LastCheckedAt:   sql.NullTime{Time: time.Date(2026, 3, 13, 14, 32, 5, 0, time.UTC), Valid: true},
		},
	}

	msg := FormatEndpointList(eps)

	if !strings.Contains(msg, "📋 Monitored Endpoints") {
		t.Error("should contain header")
	}
	if !strings.Contains(msg, "https://example.com") {
		t.Error("should contain URL")
	}
	if !strings.Contains(msg, "ok") {
		t.Error("should contain status")
	}
	if !strings.Contains(msg, "30s") {
		t.Error("should contain interval")
	}
}

func TestFormatEndpointListMultiple(t *testing.T) {
	eps := []storage.Endpoint{
		{URL: "https://a.com", IntervalSeconds: 30, Status: "ok"},
		{URL: "https://b.com", IntervalSeconds: 60, Status: "not_ok"},
	}

	msg := FormatEndpointList(eps)

	if !strings.Contains(msg, "1. https://a.com") {
		t.Error("should contain first endpoint numbered")
	}
	if !strings.Contains(msg, "2. https://b.com") {
		t.Error("should contain second endpoint numbered")
	}
}

func TestFormatHelp(t *testing.T) {
	msg := FormatHelp()

	commands := []string{"/add", "/delete", "/status", "/list", "/interval", "/help"}
	for _, cmd := range commands {
		if !strings.Contains(msg, cmd) {
			t.Errorf("help should contain %q", cmd)
		}
	}
}

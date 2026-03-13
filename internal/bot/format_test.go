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
		Name:                     "prod-api",
		URL:                      "https://api.example.com/health",
		FailureNotificationsSent: 2,
	}

	msg := FormatFailureWithCode(ep, 503, 3)

	checks := []struct {
		label    string
		contains string
	}{
		{"header", "<b>Endpoint Down</b>"},
		{"name", "prod-api"},
		{"url", "<code>https://api.example.com/health</code>"},
		{"status code", "503"},
		{"alerts", "2 of 3"},
	}
	for _, c := range checks {
		if !strings.Contains(msg, c.contains) {
			t.Errorf("should contain %s: %q", c.label, c.contains)
		}
	}
}

func TestFormatFailure(t *testing.T) {
	ep := storage.Endpoint{
		Name:                     "prod-api",
		URL:                      "https://api.example.com/health",
		FailureNotificationsSent: 1,
	}

	msg := FormatFailure(ep, 3)

	if !strings.Contains(msg, "<b>Endpoint Down</b>") {
		t.Error("should contain HTML header")
	}
	if !strings.Contains(msg, "connection error") {
		t.Error("should contain connection error")
	}
	if !strings.Contains(msg, "1 of 3") {
		t.Error("should contain alerts count")
	}
}

func TestFormatRecovery(t *testing.T) {
	ep := storage.Endpoint{
		Name: "prod-api",
		URL:  "https://api.example.com/health",
	}
	downtime := 12*time.Minute + 34*time.Second

	msg := FormatRecovery(ep, downtime)

	checks := []struct {
		label    string
		contains string
	}{
		{"header", "<b>Endpoint Recovered</b>"},
		{"name", "prod-api"},
		{"url", "<code>https://api.example.com/health</code>"},
		{"downtime", "12m 34s"},
	}
	for _, c := range checks {
		if !strings.Contains(msg, c.contains) {
			t.Errorf("should contain %s: %q", c.label, c.contains)
		}
	}
}

func TestFormatEndpointListEmpty(t *testing.T) {
	text, markup := FormatEndpointList(nil)
	if !strings.Contains(text, "No endpoints") {
		t.Errorf("got %q, want empty message", text)
	}
	if markup != nil {
		t.Error("empty list should have nil markup")
	}
}

func TestFormatEndpointListSingle(t *testing.T) {
	eps := []storage.Endpoint{
		{
			ID:              1,
			Name:            "prod-api",
			URL:             "https://example.com",
			IntervalSeconds: 30,
			Status:          "ok",
			LastCheckedAt:   sql.NullTime{Time: time.Date(2026, 3, 13, 14, 32, 5, 0, time.UTC), Valid: true},
		},
	}

	text, markup := FormatEndpointList(eps)

	if !strings.Contains(text, "1/1 endpoints healthy") {
		t.Error("should contain healthy summary")
	}
	if markup == nil {
		t.Fatal("should have markup")
	}
	// 1 endpoint button + 1 refresh button = 2 rows
	if len(markup.InlineKeyboard) != 2 {
		t.Errorf("expected 2 keyboard rows, got %d", len(markup.InlineKeyboard))
	}
	// First button should contain the endpoint name with emoji
	if len(markup.InlineKeyboard[0]) != 1 {
		t.Error("each endpoint should have exactly one button")
	}
	btnText := markup.InlineKeyboard[0][0].Text
	if !strings.Contains(btnText, "prod-api") {
		t.Errorf("button should contain endpoint name, got %q", btnText)
	}
}

func TestFormatEndpointListMultiple(t *testing.T) {
	eps := []storage.Endpoint{
		{ID: 1, Name: "site-a", URL: "https://a.com", IntervalSeconds: 30, Status: "ok"},
		{ID: 2, Name: "site-b", URL: "https://b.com", IntervalSeconds: 60, Status: "not_ok", ConsecutiveFailures: 3},
	}

	text, markup := FormatEndpointList(eps)

	if !strings.Contains(text, "1/2 endpoints healthy") {
		t.Error("should contain healthy summary")
	}
	if markup == nil {
		t.Fatal("should have markup")
	}
	// 2 endpoint buttons + 1 refresh button = 3 rows
	if len(markup.InlineKeyboard) != 3 {
		t.Errorf("expected 3 keyboard rows, got %d", len(markup.InlineKeyboard))
	}
}

func TestFormatEndpointDetail(t *testing.T) {
	ep := storage.Endpoint{
		ID:                  1,
		Name:                "prod-api",
		URL:                 "https://example.com",
		IntervalSeconds:     30,
		Status:              "not_ok",
		ConsecutiveFailures: 3,
		LastCheckedAt:       sql.NullTime{Time: time.Date(2026, 3, 13, 14, 32, 5, 0, time.UTC), Valid: true},
	}

	text, markup := FormatEndpointDetail(ep)

	checks := []struct {
		label    string
		contains string
	}{
		{"name", "<b>prod-api</b>"},
		{"url", "<code>https://example.com</code>"},
		{"interval", "30s"},
		{"status", "not_ok"},
		{"failures", "3 failures"},
		{"last check", "14:32:05 UTC"},
	}
	for _, c := range checks {
		if !strings.Contains(text, c.contains) {
			t.Errorf("should contain %s: %q", c.label, c.contains)
		}
	}

	if markup == nil {
		t.Fatal("should have markup")
	}
	// Row 1: interval + delete, Row 2: back
	if len(markup.InlineKeyboard) != 2 {
		t.Errorf("expected 2 keyboard rows, got %d", len(markup.InlineKeyboard))
	}
}

func TestFormatEndpointDetailNeverChecked(t *testing.T) {
	ep := storage.Endpoint{
		ID:              1,
		Name:            "new-ep",
		URL:             "https://new.com",
		IntervalSeconds: 60,
		Status:          "unknown",
	}

	text, _ := FormatEndpointDetail(ep)

	if !strings.Contains(text, "never") {
		t.Error("should show never for unchecked endpoint")
	}
	if !strings.Contains(text, "⚪") {
		t.Error("should show unknown emoji")
	}
}

func TestFormatHelp(t *testing.T) {
	msg := FormatHelp()

	commands := []string{"/add", "/delete", "/list", "/interval", "/help"}
	for _, cmd := range commands {
		if !strings.Contains(msg, cmd) {
			t.Errorf("help should contain %q", cmd)
		}
	}
	if !strings.Contains(msg, "<b>Noroshi") {
		t.Error("help should have HTML-formatted title")
	}
	if !strings.Contains(msg, "&lt;name&gt;") {
		t.Error("help should have HTML-escaped angle brackets")
	}
}

func TestHTMLEscapeInFormat(t *testing.T) {
	ep := storage.Endpoint{
		Name:                     "<script>alert</script>",
		URL:                      "https://example.com?a=1&b=2",
		FailureNotificationsSent: 1,
	}

	msg := FormatFailure(ep, 3)

	if strings.Contains(msg, "<script>") {
		t.Error("name should be HTML-escaped")
	}
	if !strings.Contains(msg, "&lt;script&gt;") {
		t.Error("name should contain escaped angle brackets")
	}
	if strings.Contains(msg, "?a=1&b=2") {
		if !strings.Contains(msg, "?a=1&amp;b=2") {
			t.Error("URL ampersand should be HTML-escaped")
		}
	}
}

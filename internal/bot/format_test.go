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

	if !strings.Contains(text, "1/1 healthy") {
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

	if !strings.Contains(text, "1/2 healthy") {
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
		LastStatusCode:      503,
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
		{"http status", "<b>HTTP:</b> 503"},
		{"last check", "2026-03-13 14:32 UTC"},
	}
	for _, c := range checks {
		if !strings.Contains(text, c.contains) {
			t.Errorf("should contain %s: %q", c.label, c.contains)
		}
	}

	if markup == nil {
		t.Fatal("should have markup")
	}
	// Row 1: check now + pause, Row 2: interval + delete, Row 3: back
	if len(markup.InlineKeyboard) != 3 {
		t.Errorf("expected 3 keyboard rows, got %d", len(markup.InlineKeyboard))
	}
}

func TestFormatEndpointDetailNoStatusCode(t *testing.T) {
	ep := storage.Endpoint{
		ID:                  1,
		Name:                "failing-ep",
		URL:                 "https://down.com",
		IntervalSeconds:     30,
		Status:              "not_ok",
		ConsecutiveFailures: 2,
		LastStatusCode:      0,
		LastCheckedAt:       sql.NullTime{Time: time.Date(2026, 3, 13, 14, 32, 5, 0, time.UTC), Valid: true},
	}

	text, _ := FormatEndpointDetail(ep)

	if strings.Contains(text, "<b>HTTP:</b>") {
		t.Error("should not show HTTP status line when LastStatusCode is 0")
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

func TestFormatCheckedAt(t *testing.T) {
	tests := []struct {
		name     string
		t        time.Time
		contains string
	}{
		{"just now", time.Now().Add(-3 * time.Second), "just now"},
		{"seconds ago", time.Now().Add(-45 * time.Second), "45s ago"},
		{"minutes ago", time.Now().Add(-5 * time.Minute), "5m"},
		{"hours ago", time.Now().Add(-2 * time.Hour), "2h"},
		{"absolute date", time.Date(2026, 3, 13, 14, 32, 5, 0, time.UTC), "2026-03-13"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatCheckedAt(tt.t); !strings.Contains(got, tt.contains) {
				t.Errorf("formatCheckedAt = %q, want containing %q", got, tt.contains)
			}
		})
	}
}

func TestAlertKeyboards(t *testing.T) {
	ep := storage.Endpoint{ID: 7, Name: "prod-api"}

	alert := AlertKeyboard(ep)
	if len(alert.InlineKeyboard) != 1 || len(alert.InlineKeyboard[0]) != 3 {
		t.Errorf("alert keyboard should have 1 row of 3 buttons, got %+v", alert.InlineKeyboard)
	}

	recovery := RecoveryKeyboard(ep)
	if len(recovery.InlineKeyboard) != 1 || len(recovery.InlineKeyboard[0]) != 2 {
		t.Errorf("recovery keyboard should have 1 row of 2 buttons, got %+v", recovery.InlineKeyboard)
	}
}

func TestFormatEndpointListMixed(t *testing.T) {
	eps := []storage.Endpoint{
		{ID: 1, Name: "ok-ep", URL: "https://a.com", Status: "ok", LastStatusCode: 200, LastLatencyMs: 42,
			LastCheckedAt: sql.NullTime{Time: time.Now(), Valid: true}},
		{ID: 2, Name: "down-ep", URL: "https://b.com", Status: "not_ok", LastStatusCode: 503},
		{ID: 3, Name: "paused-ep", URL: "https://c.com", Status: "ok", Paused: true},
		{ID: 4, Name: "new-ep", URL: "https://d.com", Status: "unknown"},
	}

	text, _ := FormatEndpointList(eps)

	for _, want := range []string{"1/4 healthy", "1 down", "1 paused", "1 pending", "42ms", "HTTP 503", "paused", "pending"} {
		if !strings.Contains(text, want) {
			t.Errorf("list should contain %q, got: %s", want, text)
		}
	}
}

func TestFormatEndpointDetailPaused(t *testing.T) {
	ep := storage.Endpoint{ID: 1, Name: "prod-api", URL: "https://example.com", IntervalSeconds: 30, Status: "ok", Paused: true}

	text, markup := FormatEndpointDetail(ep)

	if !strings.Contains(text, "monitoring paused") {
		t.Errorf("detail should show paused state, got: %s", text)
	}
	if !strings.Contains(markup.InlineKeyboard[0][1].Text, "Resume") {
		t.Errorf("pause button should become Resume, got %q", markup.InlineKeyboard[0][1].Text)
	}
}

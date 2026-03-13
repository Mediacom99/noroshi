package bot

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Endpoint mirrors storage.Endpoint for the bot package.
type Endpoint struct {
	ID                       int64
	URL                      string
	IntervalSeconds          int
	Status                   string
	LastCheckedAt            sql.NullTime
	LastFailureAt            sql.NullTime
	ConsecutiveFailures      int
	FailureNotificationsSent int
	CreatedAt                time.Time
}

// FormatDuration produces human-readable duration: "2h 15m 30s", "12m 34s", "45s".
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}

	totalSeconds := int(d.Seconds())
	if totalSeconds == 0 {
		return "0s"
	}

	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	var parts []string
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return strings.Join(parts, " ")
}

// FormatFailure formats a failure notification message.
func FormatFailure(ep Endpoint, maxFailures int) string {
	return fmt.Sprintf(
		"🔴 ENDPOINT DOWN\nURL: %s\nStatus: NOT_OK (HTTP %d)\nTime: %s\nConsecutive failures: %d/%d",
		ep.URL,
		0, // status code not stored in endpoint — caller can extend if needed
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		ep.FailureNotificationsSent,
		maxFailures,
	)
}

// FormatFailureWithCode formats a failure notification message with status code.
func FormatFailureWithCode(ep Endpoint, statusCode int, maxFailures int) string {
	return fmt.Sprintf(
		"🔴 ENDPOINT DOWN\nURL: %s\nStatus: NOT_OK (HTTP %d)\nTime: %s\nConsecutive failures: %d/%d",
		ep.URL,
		statusCode,
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		ep.FailureNotificationsSent,
		maxFailures,
	)
}

// FormatRecovery formats a recovery notification message.
func FormatRecovery(ep Endpoint, downtime time.Duration) string {
	return fmt.Sprintf(
		"🟢 ENDPOINT RECOVERED\nURL: %s\nStatus: OK (HTTP 200)\nDowntime: %s\nRecovered at: %s",
		ep.URL,
		FormatDuration(downtime),
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
}

// FormatEndpointList formats a list of endpoints for display.
func FormatEndpointList(endpoints []Endpoint) string {
	if len(endpoints) == 0 {
		return "No endpoints are being monitored."
	}

	var b strings.Builder
	b.WriteString("📋 Monitored Endpoints\n")
	for i, ep := range endpoints {
		interval := FormatDuration(time.Duration(ep.IntervalSeconds) * time.Second)
		lastCheck := "never"
		if ep.LastCheckedAt.Valid {
			lastCheck = ep.LastCheckedAt.Time.UTC().Format("2006-01-02 15:04:05 UTC")
		}
		fmt.Fprintf(&b, "\n%d. %s\n   Status: %s | Interval: %s | Last check: %s\n",
			i+1, ep.URL, ep.Status, interval, lastCheck)
	}
	return b.String()
}

// FormatHelp returns the help text.
func FormatHelp() string {
	return `📖 Available Commands

/add <url> <interval> — Add endpoint (e.g., /add https://example.com 30s)
/delete <id_or_url> — Remove endpoint
/status — Check all endpoints now
/list — List all endpoints
/interval <id_or_url> <interval> — Update check interval
/help — Show this message

Intervals: 10s, 30s, 1m, 5m, 1h, etc. (minimum 10s)`
}

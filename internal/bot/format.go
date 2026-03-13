package bot

import (
	"fmt"
	"strings"
	"time"

	"noroshi/internal/storage"
)

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
func FormatFailure(ep storage.Endpoint, maxFailures int) string {
	return fmt.Sprintf(
		"🔴 ENDPOINT DOWN\nName: %s\nURL: %s\nStatus: NOT_OK\nTime: %s\nConsecutive failures: %d/%d",
		ep.Name,
		ep.URL,
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		ep.FailureNotificationsSent,
		maxFailures,
	)
}

// FormatFailureWithCode formats a failure notification message with status code.
func FormatFailureWithCode(ep storage.Endpoint, statusCode int, maxFailures int) string {
	return fmt.Sprintf(
		"🔴 ENDPOINT DOWN\nName: %s\nURL: %s\nStatus: NOT_OK (HTTP %d)\nTime: %s\nConsecutive failures: %d/%d",
		ep.Name,
		ep.URL,
		statusCode,
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		ep.FailureNotificationsSent,
		maxFailures,
	)
}

// FormatRecovery formats a recovery notification message.
func FormatRecovery(ep storage.Endpoint, downtime time.Duration) string {
	return fmt.Sprintf(
		"🟢 ENDPOINT RECOVERED\nName: %s\nURL: %s\nStatus: OK (HTTP 200)\nDowntime: %s\nRecovered at: %s",
		ep.Name,
		ep.URL,
		FormatDuration(downtime),
		time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	)
}

// FormatEndpointList formats a list of endpoints for display.
func FormatEndpointList(endpoints []storage.Endpoint) string {
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
		fmt.Fprintf(&b, "\n%d. %s — %s\n   Status: %s | Interval: %s | Last check: %s\n",
			i+1, ep.Name, ep.URL, ep.Status, interval, lastCheck)
	}
	return b.String()
}

// FormatHelp returns the help text.
func FormatHelp() string {
	return `📖 Noroshi — Uptime Monitor

▸ Add an endpoint
  /add <name> <url> <interval>
  /add prod-api https://example.com 30s

▸ Remove an endpoint
  /delete <id, name, or url>

▸ Change check interval
  /interval <id, name, or url> <interval>
  /interval prod-api 5m

▸ View endpoints
  /list — list all endpoints
  /status — same as /list

▸ Help
  /help — show this message

Intervals: 10s, 30s, 1m, 5m, 1h, etc. (min 10s)
Endpoints can be referenced by ID, name, or URL.`
}

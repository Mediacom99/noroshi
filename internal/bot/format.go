package bot

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"noroshi/internal/storage"

	tele "gopkg.in/telebot.v4"
)

// Callback unique identifiers for inline keyboard buttons.
const (
	cbDetail        = "dtl"
	cbDelete        = "del"
	cbConfirmDelete = "cdel"
	cbBack          = "back"
	cbInterval      = "intv"
	cbSetInterval   = "sint"
	cbRefresh       = "ref"
)

func htmlEscape(s string) string {
	return html.EscapeString(s)
}

// statusEmoji returns the emoji for an endpoint status.
func statusEmoji(status string) string {
	switch status {
	case "ok":
		return "🟢"
	case "not_ok":
		return "🔴"
	default:
		return "⚪"
	}
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

// FormatFailure formats a failure notification message (no HTTP status code available).
func FormatFailure(ep storage.Endpoint, maxFailures int) string {
	return fmt.Sprintf(
		"🔴 <b>Endpoint Down</b>\n\n"+
			"<b>Name:</b> %s\n"+
			"<b>URL:</b> <code>%s</code>\n"+
			"<b>HTTP:</b> connection error\n"+
			"<b>Time:</b> %s\n"+
			"<b>Alerts:</b> %d of %d",
		htmlEscape(ep.Name),
		htmlEscape(ep.URL),
		time.Now().UTC().Format("15:04:05 UTC"),
		ep.FailureNotificationsSent,
		maxFailures,
	)
}

// FormatFailureWithCode formats a failure notification with an HTTP status code.
func FormatFailureWithCode(ep storage.Endpoint, statusCode int, maxFailures int) string {
	return fmt.Sprintf(
		"🔴 <b>Endpoint Down</b>\n\n"+
			"<b>Name:</b> %s\n"+
			"<b>URL:</b> <code>%s</code>\n"+
			"<b>HTTP:</b> %d\n"+
			"<b>Time:</b> %s\n"+
			"<b>Alerts:</b> %d of %d",
		htmlEscape(ep.Name),
		htmlEscape(ep.URL),
		statusCode,
		time.Now().UTC().Format("15:04:05 UTC"),
		ep.FailureNotificationsSent,
		maxFailures,
	)
}

// FormatRecovery formats a recovery notification message.
func FormatRecovery(ep storage.Endpoint, downtime time.Duration) string {
	return fmt.Sprintf(
		"🟢 <b>Endpoint Recovered</b>\n\n"+
			"<b>Name:</b> %s\n"+
			"<b>URL:</b> <code>%s</code>\n"+
			"<b>Downtime:</b> %s\n"+
			"<b>Time:</b> %s",
		htmlEscape(ep.Name),
		htmlEscape(ep.URL),
		FormatDuration(downtime),
		time.Now().UTC().Format("15:04:05 UTC"),
	)
}

// FormatEndpointList formats the dashboard overview.
// Each endpoint gets one button — tap it to see details and actions.
func FormatEndpointList(endpoints []storage.Endpoint) (string, *tele.ReplyMarkup) {
	if len(endpoints) == 0 {
		return "No endpoints are being monitored.\nUse /add to start monitoring.", nil
	}

	healthy := 0
	for _, ep := range endpoints {
		if ep.Status == "ok" {
			healthy++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "📊 <b>%d/%d endpoints healthy</b>", healthy, len(endpoints))

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row
	for _, ep := range endpoints {
		id := strconv.FormatInt(ep.ID, 10)
		emoji := statusEmoji(ep.Status)
		label := fmt.Sprintf("%s %s", emoji, ep.Name)
		rows = append(rows, menu.Row(menu.Data(label, cbDetail, id)))
	}
	rows = append(rows, menu.Row(menu.Data("🔄 Refresh", cbRefresh)))
	menu.Inline(rows...)

	return b.String(), menu
}

// FormatEndpointDetail formats a single endpoint's detail view with action buttons.
func FormatEndpointDetail(ep storage.Endpoint) (string, *tele.ReplyMarkup) {
	emoji := statusEmoji(ep.Status)
	interval := FormatDuration(time.Duration(ep.IntervalSeconds) * time.Second)

	var b strings.Builder
	fmt.Fprintf(&b, "%s <b>%s</b>\n\n", emoji, htmlEscape(ep.Name))
	fmt.Fprintf(&b, "<b>URL:</b> <code>%s</code>\n", htmlEscape(ep.URL))
	fmt.Fprintf(&b, "<b>Interval:</b> %s\n", interval)
	fmt.Fprintf(&b, "<b>Status:</b> %s", ep.Status)

	if ep.Status == "not_ok" && ep.ConsecutiveFailures > 0 {
		fmt.Fprintf(&b, " (%d failures)", ep.ConsecutiveFailures)
	}

	if ep.LastCheckedAt.Valid {
		fmt.Fprintf(&b, "\n<b>Last check:</b> %s", ep.LastCheckedAt.Time.UTC().Format("15:04:05 UTC"))
	} else {
		b.WriteString("\n<b>Last check:</b> never")
	}

	id := strconv.FormatInt(ep.ID, 10)
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("⏱ Change interval", cbInterval, id),
			menu.Data("🗑 Delete", cbDelete, id),
		),
		menu.Row(
			menu.Data("◀ Back to list", cbBack),
		),
	)

	return b.String(), menu
}

// FormatHelp returns the help text.
func FormatHelp() string {
	return "📖 <b>Noroshi — Uptime Monitor</b>\n\n" +
		"/list — View all endpoints\n" +
		"/add <code>&lt;name&gt; &lt;url&gt; [interval]</code> — Add endpoint (default: 1m)\n" +
		"/delete <code>&lt;name or id&gt;</code> — Remove endpoint\n" +
		"/interval <code>&lt;name or id&gt; &lt;duration&gt;</code> — Change interval\n" +
		"/help — This message\n\n" +
		"<b>Intervals:</b> 10s, 30s, 1m, 5m, 1h (min 10s)\n" +
		"<b>Tip:</b> Use endpoint name or ID in commands."
}

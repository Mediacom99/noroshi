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
	cbCheckNow      = "chk"
	cbPause         = "pse"
	cbUptime        = "upt"
	cbIncidents     = "inc"
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

// displayEmoji returns the emoji for an endpoint, honoring the paused state
// and the slow-latency threshold (0 = disabled).
func displayEmoji(ep storage.Endpoint, slowThresholdMs int64) string {
	if ep.Paused {
		return "⏸️"
	}
	if isSlow(ep, slowThresholdMs) {
		return "🟡"
	}
	return statusEmoji(ep.Status)
}

// isSlow reports whether an up endpoint is responding above the latency threshold.
func isSlow(ep storage.Endpoint, slowThresholdMs int64) bool {
	return slowThresholdMs > 0 && ep.Status == "ok" &&
		ep.LastCheckedAt.Valid && ep.LastLatencyMs > slowThresholdMs
}

// formatUntil renders a future timestamp as a countdown ("1h 30m left").
func formatUntil(t time.Time) string {
	d := time.Until(t)
	if d <= 0 {
		return "now"
	}
	return FormatDuration(d) + " left"
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

// formatCheckedAt renders a check timestamp relative to now ("2m ago"),
// falling back to an absolute date for anything older than a day.
func formatCheckedAt(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 10*time.Second:
		return "just now"
	case d < 24*time.Hour:
		return FormatDuration(d) + " ago"
	default:
		return t.UTC().Format("2006-01-02 15:04 UTC")
	}
}

// endpointLine renders one compact status line: "🟢 prod-api · 45ms".
func endpointLine(ep storage.Endpoint, slowThresholdMs int64) string {
	line := fmt.Sprintf("%s <b>%s</b>", displayEmoji(ep, slowThresholdMs), htmlEscape(ep.Name))
	switch {
	case ep.Paused:
		line += " · paused"
	case ep.Status == "not_ok":
		if ep.LastStatusCode > 0 {
			line += fmt.Sprintf(" · HTTP %d", ep.LastStatusCode)
		} else {
			line += " · connection error"
		}
	case !ep.LastCheckedAt.Valid:
		line += " · pending"
	default: // ok
		line += fmt.Sprintf(" · %dms", ep.LastLatencyMs)
		if isSlow(ep, slowThresholdMs) {
			line += " (slow)"
		}
	}
	return line
}

// FormatFailure formats a failure notification message (no HTTP status code available).
func FormatFailure(ep storage.Endpoint, maxFailures int) string {
	return fmt.Sprintf(
		"🔴 <b>Endpoint Down</b>\n\n"+
			"<b>%s</b>\n"+
			"<code>%s</code>\n\n"+
			"<b>HTTP:</b> connection error\n"+
			"<b>Time:</b> %s\n"+
			"<b>Alert:</b> %d of %d",
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
			"<b>%s</b>\n"+
			"<code>%s</code>\n\n"+
			"<b>HTTP:</b> %d · %dms\n"+
			"<b>Time:</b> %s\n"+
			"<b>Alert:</b> %d of %d",
		htmlEscape(ep.Name),
		htmlEscape(ep.URL),
		statusCode,
		ep.LastLatencyMs,
		time.Now().UTC().Format("15:04:05 UTC"),
		ep.FailureNotificationsSent,
		maxFailures,
	)
}

// FormatRecovery formats a recovery notification message.
func FormatRecovery(ep storage.Endpoint, downtime time.Duration) string {
	return fmt.Sprintf(
		"🟢 <b>Endpoint Recovered</b>\n\n"+
			"<b>%s</b>\n"+
			"<code>%s</code>\n\n"+
			"<b>Downtime:</b> %s\n"+
			"<b>Recovered:</b> %s",
		htmlEscape(ep.Name),
		htmlEscape(ep.URL),
		FormatDuration(downtime),
		time.Now().UTC().Format("15:04:05 UTC"),
	)
}

// FormatCertWarning formats a certificate-expiry warning message.
func FormatCertWarning(ep storage.Endpoint, daysLeft int) string {
	var when string
	switch {
	case daysLeft < 0:
		when = "expired"
	case daysLeft == 0:
		when = "expires today"
	default:
		when = fmt.Sprintf("expires in %d days", daysLeft)
	}

	expiry := ""
	if ep.CertExpiresAt.Valid {
		expiry = fmt.Sprintf("\n<b>Expiry:</b> %s", ep.CertExpiresAt.Time.UTC().Format("2006-01-02"))
	}

	return fmt.Sprintf(
		"⚠️ <b>Certificate Expiring</b>\n\n"+
			"<b>%s</b>\n"+
			"<code>%s</code>\n\n"+
			"<b>TLS:</b> %s%s",
		htmlEscape(ep.Name),
		htmlEscape(ep.URL),
		when,
		expiry,
	)
}

// AlertKeyboard returns the action buttons attached to a failure alert.
func AlertKeyboard(ep storage.Endpoint) *tele.ReplyMarkup {
	id := strconv.FormatInt(ep.ID, 10)
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("🔍 Check now", cbCheckNow, id),
			menu.Data("⏸ Pause", cbPause, id),
			menu.Data("📊 Detail", cbDetail, id),
		),
	)
	return menu
}

// RecoveryKeyboard returns the buttons attached to a recovery alert.
func RecoveryKeyboard(ep storage.Endpoint) *tele.ReplyMarkup {
	id := strconv.FormatInt(ep.ID, 10)
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("📊 Detail", cbDetail, id),
			menu.Data("🔍 Check now", cbCheckNow, id),
		),
	)
	return menu
}

// FormatEndpointList formats the dashboard overview.
// Each endpoint gets one button — tap it to see details and actions.
func FormatEndpointList(endpoints []storage.Endpoint, slowThresholdMs int64) (string, *tele.ReplyMarkup) {
	if len(endpoints) == 0 {
		return "No endpoints are being monitored.\nUse /add to start monitoring.", nil
	}

	healthy, down, paused, pending := 0, 0, 0, 0
	for _, ep := range endpoints {
		switch {
		case ep.Paused:
			paused++
		case ep.Status == "ok":
			healthy++
		case ep.Status == "not_ok":
			down++
		default:
			pending++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "📊 <b>%d/%d healthy</b>", healthy, len(endpoints))
	if down > 0 {
		fmt.Fprintf(&b, " · %d down", down)
	}
	if paused > 0 {
		fmt.Fprintf(&b, " · %d paused", paused)
	}
	if pending > 0 {
		fmt.Fprintf(&b, " · %d pending", pending)
	}

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row
	for _, ep := range endpoints {
		id := strconv.FormatInt(ep.ID, 10)
		fmt.Fprintf(&b, "\n%s", endpointLine(ep, slowThresholdMs))
		rows = append(rows, menu.Row(menu.Data(fmt.Sprintf("%s %s", displayEmoji(ep, slowThresholdMs), ep.Name), cbDetail, id)))
	}
	rows = append(rows, menu.Row(menu.Data("🔄 Refresh", cbRefresh)))
	menu.Inline(rows...)

	return b.String(), menu
}

// FormatStatus formats the result of an on-demand /status check.
func FormatStatus(endpoints []storage.Endpoint, slowThresholdMs int64) string {
	healthy := 0
	for _, ep := range endpoints {
		if ep.Status == "ok" {
			healthy++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "📊 <b>Status check — %d/%d healthy</b>\n", healthy, len(endpoints))
	for _, ep := range endpoints {
		fmt.Fprintf(&b, "\n%s <b>%s</b>", displayEmoji(ep, slowThresholdMs), htmlEscape(ep.Name))
		switch {
		case ep.Paused:
			b.WriteString(" · paused")
		case ep.LastStatusCode > 0:
			fmt.Fprintf(&b, " · HTTP %d · %dms", ep.LastStatusCode, ep.LastLatencyMs)
		case ep.Status == "not_ok":
			b.WriteString(" · connection error")
		default:
			b.WriteString(" · pending")
		}
	}
	return b.String()
}

// FormatUptime formats uptime statistics for 24h/7d/30d windows.
func FormatUptime(ep storage.Endpoint, windows []string, stats []storage.WindowStats) string {
	var b strings.Builder
	fmt.Fprintf(&b, "📈 <b>%s — uptime</b>\n", htmlEscape(ep.Name))

	hasData := false
	for _, st := range stats {
		if st.Total > 0 {
			hasData = true
			break
		}
	}
	if !hasData {
		b.WriteString("\nNo check history yet — stats appear after a few checks.")
		return b.String()
	}

	for i, label := range windows {
		st := stats[i]
		if st.Total == 0 {
			fmt.Fprintf(&b, "\n<b>%s:</b> no data", label)
			continue
		}
		incidents := fmt.Sprintf("%d incidents", st.Incidents)
		if st.Incidents == 1 {
			incidents = "1 incident"
		}
		fmt.Fprintf(&b, "\n<b>%s:</b> %.2f%% · avg %.0fms · p95 %dms · %s",
			label, st.Uptime(), st.AvgLatencyMs, st.P95LatencyMs, incidents)
	}
	return b.String()
}

// FormatIncidents formats the recent outage history for an endpoint.
func FormatIncidents(ep storage.Endpoint, transitions []storage.CheckTransition) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🕐 <b>%s — recent incidents</b>\n", htmlEscape(ep.Name))

	// Pair down-flips with the following up-flip.
	type incident struct {
		start    time.Time
		duration time.Duration // 0 = ongoing
		code     int
	}
	var incidents []incident
	for i, t := range transitions {
		if t.Up {
			continue
		}
		inc := incident{start: t.CheckedAt, code: t.StatusCode}
		if i+1 < len(transitions) {
			inc.duration = transitions[i+1].CheckedAt.Sub(t.CheckedAt)
		}
		incidents = append(incidents, inc)
	}

	if len(incidents) == 0 {
		b.WriteString("\nNo incidents recorded. 🎉")
		return b.String()
	}

	// Newest first, cap at 5.
	for i := len(incidents) - 1; i >= 0 && i >= len(incidents)-5; i-- {
		inc := incidents[i]
		fmt.Fprintf(&b, "\n• %s", inc.start.UTC().Format("2006-01-02 15:04 UTC"))
		if inc.duration > 0 {
			fmt.Fprintf(&b, " · %s", FormatDuration(inc.duration))
		} else {
			b.WriteString(" · ongoing")
		}
		if inc.code > 0 {
			fmt.Fprintf(&b, " · HTTP %d", inc.code)
		} else {
			b.WriteString(" · connection error")
		}
	}
	return b.String()
}

// FormatEndpointDetail formats a single endpoint's detail view with action buttons.
func FormatEndpointDetail(ep storage.Endpoint, slowThresholdMs int64) (string, *tele.ReplyMarkup) {
	emoji := displayEmoji(ep, slowThresholdMs)
	interval := FormatDuration(time.Duration(ep.IntervalSeconds) * time.Second)

	var b strings.Builder
	fmt.Fprintf(&b, "%s <b>%s</b>", emoji, htmlEscape(ep.Name))
	if ep.Paused {
		if ep.PausedUntil.Valid {
			fmt.Fprintf(&b, "  <i>(paused · %s)</i>", formatUntil(ep.PausedUntil.Time))
		} else {
			b.WriteString("  <i>(monitoring paused)</i>")
		}
	}
	fmt.Fprintf(&b, "\n\n<b>URL:</b> <code>%s</code>\n", htmlEscape(ep.URL))

	fmt.Fprintf(&b, "<b>Status:</b> %s", ep.Status)
	if isSlow(ep, slowThresholdMs) {
		b.WriteString(" (slow)")
	}
	if ep.Status == "not_ok" && ep.ConsecutiveFailures > 0 {
		fmt.Fprintf(&b, " (%d failures)", ep.ConsecutiveFailures)
	}

	if ep.Status == "not_ok" && ep.LastStatusCode > 0 {
		fmt.Fprintf(&b, "\n<b>HTTP:</b> %d", ep.LastStatusCode)
	}

	if ep.Status == "not_ok" && ep.LastCheckError != "" {
		fmt.Fprintf(&b, "\n<b>Error:</b> %s", htmlEscape(ep.LastCheckError))
	}

	fmt.Fprintf(&b, "\n<b>Interval:</b> %s", interval)

	if ep.ExpectedStatus > 0 {
		fmt.Fprintf(&b, "\n<b>Expected:</b> HTTP %d", ep.ExpectedStatus)
	}
	if ep.ExpectedKeyword != "" {
		fmt.Fprintf(&b, "\n<b>Keyword:</b> <code>%s</code>", htmlEscape(ep.ExpectedKeyword))
	}
	if ep.CertExpiresAt.Valid {
		days := int(time.Until(ep.CertExpiresAt.Time).Hours() / 24)
		fmt.Fprintf(&b, "\n<b>TLS:</b> %s (%d days left)", ep.CertExpiresAt.Time.UTC().Format("2006-01-02"), days)
	}

	if ep.LastCheckedAt.Valid {
		fmt.Fprintf(&b, "\n<b>Latency:</b> %dms", ep.LastLatencyMs)
		fmt.Fprintf(&b, "\n<b>Last check:</b> %s", formatCheckedAt(ep.LastCheckedAt.Time))
	} else {
		b.WriteString("\n<b>Last check:</b> never")
	}

	id := strconv.FormatInt(ep.ID, 10)
	pauseLabel := "⏸ Pause"
	if ep.Paused {
		pauseLabel = "▶️ Resume"
	}
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("🔍 Check now", cbCheckNow, id),
			menu.Data(pauseLabel, cbPause, id),
		),
		menu.Row(
			menu.Data("⏱ Change interval", cbInterval, id),
			menu.Data("🗑 Delete", cbDelete, id),
		),
		menu.Row(
			menu.Data("📈 Uptime", cbUptime, id),
			menu.Data("🕐 Incidents", cbIncidents, id),
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
		"<b>Monitoring</b>\n" +
		"/add <code>&lt;name&gt; &lt;url&gt; [interval]</code> — Add endpoint (default 1m)\n" +
		"/list — Dashboard with per-endpoint actions\n" +
		"/status — Check everything right now\n" +
		"/uptime <code>&lt;name or id&gt;</code> — Uptime %, latency stats\n" +
		"/incidents <code>&lt;name or id&gt;</code> — Recent outages\n" +
		"/pause <code>&lt;name or id&gt;</code> — Silence an endpoint\n" +
		"/resume <code>&lt;name or id&gt;</code> — Resume monitoring\n\n" +
		"<b>Manage</b>\n" +
		"/interval <code>&lt;name or id&gt; &lt;duration&gt;</code> — Change interval\n" +
		"/expect <code>&lt;name or id&gt; &lt;status|any&gt;</code> — Require exact HTTP status\n" +
		"/keyword <code>&lt;name or id&gt; &lt;text|off&gt;</code> — Require text in response\n" +
		"/rename <code>&lt;name or id&gt; &lt;new-name&gt;</code> — Rename endpoint\n" +
		"/delete <code>&lt;name or id&gt;</code> — Remove endpoint\n" +
		"/help — This message\n\n" +
		"<b>Intervals:</b> 10s · 30s · 1m · 5m · 1h (min 10s)\n" +
		"<b>Tip:</b> tap an endpoint in /list for actions"
}

package bot

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"noroshi/internal/apperror"
	"noroshi/internal/monitor"
	"noroshi/internal/storage"

	tele "gopkg.in/telebot.v4"
)

func (b *Bot) registerHandlers() {
	b.bot.Handle("/add", b.guarded(b.handleAdd))
	b.bot.Handle("/delete", b.guarded(b.handleDelete))
	b.bot.Handle("/list", b.guarded(b.handleList))
	b.bot.Handle("/status", b.guarded(b.handleStatus))
	b.bot.Handle("/interval", b.guarded(b.handleInterval))
	b.bot.Handle("/pause", b.guarded(b.handlePause))
	b.bot.Handle("/resume", b.guarded(b.handleResume))
	b.bot.Handle("/uptime", b.guarded(b.handleUptime))
	b.bot.Handle("/incidents", b.guarded(b.handleIncidents))
	b.bot.Handle("/expect", b.guarded(b.handleExpect))
	b.bot.Handle("/keyword", b.guarded(b.handleKeyword))
	b.bot.Handle("/rename", b.guarded(b.handleRename))
	b.bot.Handle("/maint", b.guarded(b.handleMaint))
	b.bot.Handle("/digest", b.guarded(b.handleDigest))
	b.bot.Handle("/export", b.guarded(b.handleExport))
	b.bot.Handle("/help", b.guarded(b.handleHelp))

	b.registerCallbacks()
}

func (b *Bot) handleAdd(c tele.Context) error {
	args := strings.Fields(c.Message().Payload)
	if len(args) < 2 {
		return c.Send("Usage: /add <code>&lt;name&gt; &lt;url&gt; [interval]</code>\nExample: /add prod-api https://example.com 30s\nDefault interval: 1m", tele.NoPreview)
	}

	name := args[0]
	if err := ValidateName(name); err != nil {
		return c.Send(err.Error())
	}
	rawURL := args[1]
	if err := ValidateURL(rawURL); err != nil {
		return c.Send(fmt.Sprintf("Invalid URL: %s\nSupported schemes: http(s)://, tcp://host:port, dns://host, ping://host", htmlEscape(err.Error())))
	}

	intervalStr := "1m"
	if len(args) >= 3 {
		intervalStr = args[2]
	}
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		return c.Send("Invalid interval. Use format like 30s, 5m, 1h")
	}
	if interval < 10*time.Second {
		return c.Send("Interval must be at least 10s")
	}

	ep, err := b.store.AddEndpoint(b.rootCtx, name, rawURL, int(interval.Seconds()))
	if err != nil {
		if errors.Is(err, apperror.ErrDuplicate) {
			return c.Send("This name or URL is already being monitored.")
		}
		b.logger.Error("add endpoint", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	if b.scheduler != nil {
		if err := b.scheduler.Add(b.rootCtx, ep); err != nil {
			b.logger.Error("add to scheduler", "id", ep.ID, "error", err)
			return c.Send(fmt.Sprintf("⚠️ <b>Added endpoint #%d</b>, but scheduling failed — monitoring will start after the next restart.\n\n<b>Name:</b> %s\n<b>URL:</b> <code>%s</code>\n<b>Interval:</b> %s",
				ep.ID, htmlEscape(ep.Name), htmlEscape(ep.URL), FormatDuration(interval)), tele.NoPreview)
		}
	}

	// Run one immediate check so the user gets instant feedback. The scheduled
	// job (started immediately by the scheduler) persists the authoritative state.
	var firstCheck string
	if b.checker != nil {
		res := b.checker.Check(b.rootCtx, ep.URL, monitor.CheckOptions{})
		if res.Up {
			firstCheck = fmt.Sprintf("\n<b>First check:</b> 🟢 HTTP %d · %dms", res.StatusCode, res.Latency.Milliseconds())
		} else {
			firstCheck = fmt.Sprintf("\n<b>First check:</b> 🔴 %s · %dms", htmlEscape(res.Reason), res.Latency.Milliseconds())
		}
	}

	b.logger.Info("endpoint added", "id", ep.ID, "name", ep.Name, "url", ep.URL, "interval", interval.String())
	return c.Send(fmt.Sprintf("✅ <b>Added endpoint #%d</b>\n\n<b>Name:</b> %s\n<b>URL:</b> <code>%s</code>\n<b>Interval:</b> %s%s",
		ep.ID, htmlEscape(ep.Name), htmlEscape(ep.URL), FormatDuration(interval), firstCheck), tele.NoPreview)
}

func (b *Bot) handleDelete(c tele.Context) error {
	arg := strings.TrimSpace(c.Message().Payload)
	if arg == "" {
		return c.Send("Usage: /delete <code>&lt;name or id&gt;</code>")
	}

	ep, err := b.findEndpoint(arg)
	if err != nil {
		return b.replyLookupError(c, err)
	}

	if b.scheduler != nil {
		b.scheduler.Remove(ep.ID)
	}

	if err := b.store.DeleteEndpoint(b.rootCtx, ep.ID); err != nil {
		b.logger.Error("delete endpoint", "id", ep.ID, "error", err)
		return c.Send("Internal error. Please try again.")
	}

	b.logger.Info("endpoint deleted", "id", ep.ID, "name", ep.Name)
	return c.Send(fmt.Sprintf("🗑 <b>Deleted</b> %s (<code>%s</code>)", htmlEscape(ep.Name), htmlEscape(ep.URL)), tele.NoPreview)
}

func (b *Bot) handleList(c tele.Context) error {
	return b.sendEndpointList(c)
}

// handleStatus runs an immediate check on every endpoint (concurrently) and
// replies with fresh results. Ad-hoc checks don't affect the notification
// state machine (see Scheduler.CheckNow).
func (b *Bot) handleStatus(c tele.Context) error {
	endpoints, err := b.store.ListEndpoints(b.rootCtx)
	if err != nil {
		b.logger.Error("list endpoints", "error", err)
		return c.Send("Internal error. Please try again.")
	}
	if len(endpoints) == 0 {
		return c.Send("No endpoints are being monitored.\nUse /add to start monitoring.")
	}

	if b.scheduler != nil {
		var wg sync.WaitGroup
		for i, ep := range endpoints {
			wg.Add(1)
			go func() {
				defer wg.Done()
				updated, err := b.scheduler.CheckNow(b.rootCtx, ep.ID)
				if err != nil {
					b.logger.Error("status check", "id", ep.ID, "error", err)
					return
				}
				endpoints[i] = updated
			}()
		}
		wg.Wait()
	}

	return c.Send(FormatStatus(endpoints, b.slowThresholdMs), tele.NoPreview)
}

func (b *Bot) sendEndpointList(c tele.Context) error {
	endpoints, err := b.store.ListEndpoints(b.rootCtx)
	if err != nil {
		b.logger.Error("list endpoints", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	text, markup := FormatEndpointList(endpoints, b.slowThresholdMs)
	if markup == nil {
		return c.Send(text)
	}
	return c.Send(text, markup, tele.NoPreview)
}

func (b *Bot) handleInterval(c tele.Context) error {
	args := strings.Fields(c.Message().Payload)
	if len(args) < 2 {
		return c.Send("Usage: /interval <code>&lt;name or id&gt; &lt;interval&gt;</code>\nExample: /interval prod-api 5m")
	}

	ep, err := b.findEndpoint(args[0])
	if err != nil {
		return b.replyLookupError(c, err)
	}

	interval, err := time.ParseDuration(args[1])
	if err != nil {
		return c.Send("Invalid interval. Use format like 30s, 5m, 1h")
	}
	if interval < 10*time.Second {
		return c.Send("Interval must be at least 10s")
	}

	if err := b.updateInterval(ep, int(interval.Seconds())); err != nil {
		b.logger.Error("update interval", "id", ep.ID, "error", err)
		return c.Send("Internal error. Please try again.")
	}

	b.logger.Info("interval updated", "id", ep.ID, "name", ep.Name, "interval", interval.String())
	return c.Send(fmt.Sprintf("✅ <b>Updated interval</b> for %s to %s", htmlEscape(ep.Name), FormatDuration(interval)))
}

// updateInterval updates the endpoint interval in the store and reschedules the
// job. If rescheduling fails, the store update is rolled back and the previous
// job is restored, so the endpoint never ends up unmonitored by accident.
func (b *Bot) updateInterval(ep storage.Endpoint, seconds int) error {
	oldSeconds := ep.IntervalSeconds

	if err := b.store.UpdateEndpointInterval(b.rootCtx, ep.ID, seconds); err != nil {
		return err
	}

	if b.scheduler == nil {
		return nil
	}

	b.scheduler.Remove(ep.ID)
	// Paused endpoints have no job by design — just persist the new interval.
	if ep.Paused {
		return nil
	}
	ep.IntervalSeconds = seconds
	if err := b.scheduler.Add(b.rootCtx, ep); err != nil {
		ep.IntervalSeconds = oldSeconds
		if rbErr := b.store.UpdateEndpointInterval(b.rootCtx, ep.ID, oldSeconds); rbErr != nil {
			b.logger.Error("rollback interval", "id", ep.ID, "error", rbErr)
		}
		if rbErr := b.scheduler.Add(b.rootCtx, ep); rbErr != nil {
			b.logger.Error("restore job", "id", ep.ID, "error", rbErr)
		}
		return err
	}
	return nil
}

func (b *Bot) handleHelp(c tele.Context) error {
	return c.Send(FormatHelp())
}

func (b *Bot) handlePause(c tele.Context) error {
	return b.handlePauseResume(c, true)
}

func (b *Bot) handleResume(c tele.Context) error {
	return b.handlePauseResume(c, false)
}

func (b *Bot) handlePauseResume(c tele.Context, pause bool) error {
	args := strings.Fields(c.Message().Payload)
	verb := "resume"
	if pause {
		verb = "pause"
	}
	if len(args) == 0 {
		if pause {
			return c.Send("Usage: /pause <code>&lt;name or id&gt; [duration]</code>\nExample: /pause prod-api 2h")
		}
		return c.Send("Usage: /resume <code>&lt;name or id&gt;</code>")
	}

	var until sql.NullTime
	if pause && len(args) >= 2 {
		d, err := time.ParseDuration(args[1])
		if err != nil || d <= 0 {
			return c.Send("Invalid duration. Use format like 30m, 2h, 24h")
		}
		until = sql.NullTime{Time: time.Now().UTC().Add(d), Valid: true}
	}

	if args[0] == "all" {
		return b.pauseResumeAll(c, pause, until)
	}

	ep, err := b.findEndpoint(args[0])
	if err != nil {
		return b.replyLookupError(c, err)
	}

	if ep.Paused == pause && !until.Valid {
		return c.Send(fmt.Sprintf("%s is already %sd.", htmlEscape(ep.Name), verb))
	}

	if err := b.setPaused(ep, pause, until); err != nil {
		b.logger.Error("set paused", "id", ep.ID, "paused", pause, "error", err)
		return c.Send("Internal error. Please try again.")
	}

	if pause {
		if until.Valid {
			b.logger.Info("endpoint paused", "id", ep.ID, "name", ep.Name, "until", until.Time.Format("2006-01-02 15:04 UTC"))
			return c.Send(fmt.Sprintf("⏸ <b>Paused</b> %s for %s — resumes automatically.", htmlEscape(ep.Name), FormatDuration(time.Until(until.Time))))
		}
		b.logger.Info("endpoint paused", "id", ep.ID, "name", ep.Name, "until", "indefinite")
		return c.Send(fmt.Sprintf("⏸ <b>Paused</b> %s — no more checks until resumed.", htmlEscape(ep.Name)))
	}
	b.logger.Info("endpoint resumed", "id", ep.ID, "name", ep.Name)
	return c.Send(fmt.Sprintf("▶️ <b>Resumed</b> %s — monitoring restarted.", htmlEscape(ep.Name)))
}

// setPaused persists the paused flag and adds/removes the scheduler job.
// until is the optional auto-resume time (zero Valid = indefinite pause).
// On scheduler failure during resume, the store update is rolled back.
func (b *Bot) setPaused(ep storage.Endpoint, pause bool, until sql.NullTime) error {
	if err := b.store.SetEndpointPaused(b.rootCtx, ep.ID, pause, until); err != nil {
		return err
	}

	if b.scheduler == nil {
		return nil
	}

	if pause {
		b.scheduler.Remove(ep.ID)
		return nil
	}

	if err := b.scheduler.Add(b.rootCtx, ep); err != nil {
		if rbErr := b.store.SetEndpointPaused(b.rootCtx, ep.ID, true, sql.NullTime{}); rbErr != nil {
			b.logger.Error("rollback pause", "id", ep.ID, "error", rbErr)
		}
		return err
	}
	return nil
}

// replyLookupError replies to a failed endpoint lookup: a friendly message
// for ErrNotFound, a generic internal error (logged) for anything else.
func (b *Bot) replyLookupError(c tele.Context, err error) error {
	if errors.Is(err, apperror.ErrNotFound) {
		return c.Send("Endpoint not found.")
	}
	b.logger.Error("find endpoint", "error", err)
	return c.Send("Internal error. Please try again.")
}

// findEndpoint tries to find an endpoint by ID first, then by name, then by URL.
func (b *Bot) findEndpoint(arg string) (storage.Endpoint, error) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		return b.store.GetEndpoint(b.rootCtx, id)
	}
	ep, err := b.store.GetEndpointByName(b.rootCtx, arg)
	if err == nil {
		return ep, nil
	}
	return b.store.GetEndpointByURL(b.rootCtx, arg)
}

// pauseResumeAll applies a pause or resume to every endpoint.
func (b *Bot) pauseResumeAll(c tele.Context, pause bool, until sql.NullTime) error {
	endpoints, err := b.store.ListEndpoints(b.rootCtx)
	if err != nil {
		b.logger.Error("list endpoints", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	verb := "resumed"
	if pause {
		verb = "paused"
	}

	failed := 0
	changed := 0
	for _, ep := range endpoints {
		if ep.Paused == pause {
			continue
		}
		if err := b.setPaused(ep, pause, until); err != nil {
			b.logger.Error("set paused", "id", ep.ID, "paused", pause, "error", err)
			failed++
			continue
		}
		changed++
	}

	msg := fmt.Sprintf("%s <b>%s %d endpoint(s)</b>.", map[bool]string{true: "⏸", false: "▶️"}[pause], verb, changed)
	if pause && until.Valid {
		msg += fmt.Sprintf(" Resumes automatically in %s.", FormatDuration(time.Until(until.Time)))
	}
	if failed > 0 {
		msg += fmt.Sprintf(" %d failed — check logs.", failed)
	}
	if changed == 0 && failed == 0 {
		msg = fmt.Sprintf("Nothing to do — all endpoints are already %s.", verb)
	}
	if changed > 0 {
		b.logger.Info("endpoints "+verb, "changed", changed, "failed", failed)
	}
	return c.Send(msg)
}

// handleExpect sets an exact expected HTTP status for an endpoint.
// "/expect <name> any" resets to the default (any 2xx).
func (b *Bot) handleExpect(c tele.Context) error {
	args := strings.Fields(c.Message().Payload)
	if len(args) < 2 {
		return c.Send("Usage: /expect <code>&lt;name or id&gt; &lt;status|any&gt;</code>\nExample: /expect prod-api 200")
	}

	ep, err := b.findEndpoint(args[0])
	if err != nil {
		return b.replyLookupError(c, err)
	}

	code := 0
	if args[1] != "any" {
		code, err = strconv.Atoi(args[1])
		if err != nil || code < 100 || code > 599 {
			return c.Send("Invalid status code. Use 100-599, or \"any\" for any 2xx.")
		}
	}

	if err := b.store.SetExpectedStatus(b.rootCtx, ep.ID, code); err != nil {
		b.logger.Error("set expected status", "id", ep.ID, "error", err)
		return c.Send("Internal error. Please try again.")
	}

	b.logger.Info("expected status updated", "id", ep.ID, "name", ep.Name, "status", code)
	if code == 0 {
		return c.Send(fmt.Sprintf("✅ %s now expects <b>any 2xx</b> status.", htmlEscape(ep.Name)))
	}
	return c.Send(fmt.Sprintf("✅ %s now expects exactly <b>HTTP %d</b>.", htmlEscape(ep.Name), code))
}

// handleKeyword sets a required response-body keyword spec for an endpoint.
// Specs: plain text (must contain), "!text" (must not contain),
// "re:pattern" (must match), "!re:pattern" (must not match).
// "/keyword <name> off" clears it.
func (b *Bot) handleKeyword(c tele.Context) error {
	args := strings.Fields(c.Message().Payload)
	if len(args) < 2 {
		return c.Send("Usage: /keyword <code>&lt;name or id&gt; &lt;text|!text|re:pattern|!re:pattern|off&gt;</code>\n" +
			"Examples:\n" +
			"/keyword prod-api \"status\":\"ok\" — body must contain text\n" +
			"/keyword prod-api !fatal error — body must NOT contain text\n" +
			"/keyword prod-api re:version-[0-9]+ — body must match regex")
	}

	ep, err := b.findEndpoint(args[0])
	if err != nil {
		return b.replyLookupError(c, err)
	}

	keyword := strings.Join(args[1:], " ")
	if keyword == "off" {
		keyword = ""
	}
	if len(keyword) > 200 {
		return c.Send("Keyword too long (max 200 characters).")
	}
	if keyword != "" {
		if err := ValidateKeywordSpec(keyword); err != nil {
			return c.Send(err.Error())
		}
	}

	if err := b.store.SetExpectedKeyword(b.rootCtx, ep.ID, keyword); err != nil {
		b.logger.Error("set expected keyword", "id", ep.ID, "error", err)
		return c.Send("Internal error. Please try again.")
	}

	b.logger.Info("expected keyword updated", "id", ep.ID, "name", ep.Name, "keyword", keyword)
	if keyword == "" {
		return c.Send(fmt.Sprintf("✅ Keyword check <b>disabled</b> for %s.", htmlEscape(ep.Name)))
	}
	return c.Send(fmt.Sprintf("✅ Keyword check for %s: <code>%s</code>", htmlEscape(ep.Name), htmlEscape(keyword)), tele.NoPreview)
}

// handleRename renames an endpoint.
func (b *Bot) handleRename(c tele.Context) error {
	args := strings.Fields(c.Message().Payload)
	if len(args) != 2 {
		return c.Send("Usage: /rename <code>&lt;name or id&gt; &lt;new-name&gt;</code>")
	}

	ep, err := b.findEndpoint(args[0])
	if err != nil {
		return b.replyLookupError(c, err)
	}

	newName := args[1]
	if err := ValidateName(newName); err != nil {
		return c.Send(err.Error())
	}
	if newName == ep.Name {
		return c.Send("That's already its name.")
	}

	if err := b.store.RenameEndpoint(b.rootCtx, ep.ID, newName); err != nil {
		if errors.Is(err, apperror.ErrDuplicate) {
			return c.Send("That name is already taken.")
		}
		b.logger.Error("rename endpoint", "id", ep.ID, "error", err)
		return c.Send("Internal error. Please try again.")
	}

	b.logger.Info("endpoint renamed", "id", ep.ID, "old_name", ep.Name, "new_name", newName)
	return c.Send(fmt.Sprintf("✅ Renamed <b>%s</b> → <b>%s</b>", htmlEscape(ep.Name), htmlEscape(newName)))
}

// uptimeWindows are the stats windows shown by /uptime.
var uptimeWindows = []struct {
	label string
	dur   time.Duration
}{
	{"24h", 24 * time.Hour},
	{"7d", 7 * 24 * time.Hour},
	{"30d", 30 * 24 * time.Hour},
}

func (b *Bot) handleUptime(c tele.Context) error {
	ep, ok := b.findEndpointArg(c, "/uptime")
	if !ok {
		return nil
	}
	stats, err := b.collectStats(ep.ID)
	if err != nil {
		b.logger.Error("collect stats", "id", ep.ID, "error", err)
		return c.Send("Internal error. Please try again.")
	}
	labels := make([]string, len(uptimeWindows))
	for i, w := range uptimeWindows {
		labels[i] = w.label
	}
	return c.Send(FormatUptime(ep, labels, stats), tele.NoPreview)
}

func (b *Bot) handleIncidents(c tele.Context) error {
	ep, ok := b.findEndpointArg(c, "/incidents")
	if !ok {
		return nil
	}
	transitions, err := b.store.GetRecentTransitions(b.rootCtx, ep.ID, 20)
	if err != nil {
		b.logger.Error("get transitions", "id", ep.ID, "error", err)
		return c.Send("Internal error. Please try again.")
	}
	return c.Send(FormatIncidents(ep, transitions), tele.NoPreview)
}

// collectStats gathers WindowStats for all uptime windows.
func (b *Bot) collectStats(endpointID int64) ([]storage.WindowStats, error) {
	stats := make([]storage.WindowStats, len(uptimeWindows))
	for i, w := range uptimeWindows {
		st, err := b.store.GetCheckStats(b.rootCtx, endpointID, time.Now().UTC().Add(-w.dur))
		if err != nil {
			return nil, err
		}
		stats[i] = st
	}
	return stats, nil
}

// findEndpointArg parses the first command argument and resolves the endpoint,
// replying to the user on usage errors. ok=false means a reply was already sent.
func (b *Bot) findEndpointArg(c tele.Context, cmd string) (storage.Endpoint, bool) {
	arg := strings.TrimSpace(c.Message().Payload)
	if arg == "" {
		c.Send(fmt.Sprintf("Usage: %s <code>&lt;name or id&gt;</code>", cmd))
		return storage.Endpoint{}, false
	}
	ep, err := b.findEndpoint(arg)
	if err != nil {
		b.replyLookupError(c, err)
		return storage.Endpoint{}, false
	}
	return ep, true
}

// handleMaint manages recurring maintenance windows: /maint add|list|del.
// During a window, scheduled checks are skipped entirely (times are UTC).
func (b *Bot) handleMaint(c tele.Context) error {
	args := strings.Fields(c.Message().Payload)
	if len(args) == 0 {
		return c.Send(maintUsage())
	}
	switch args[0] {
	case "add":
		return b.maintAdd(c, args[1:])
	case "list":
		return b.maintList(c)
	case "del", "delete", "remove", "rm":
		return b.maintDel(c, args[1:])
	default:
		return c.Send(maintUsage())
	}
}

func maintUsage() string {
	return "Usage:\n" +
		"/maint add <code>&lt;name|all&gt; &lt;days&gt; &lt;HH:MM-HH:MM&gt;</code>\n" +
		"/maint list\n" +
		"/maint del <code>&lt;window id&gt;</code>\n\n" +
		"Days: <code>all</code> or <code>mon,tue,wed,thu,fri,sat,sun</code>\n" +
		"Times are UTC. Checks are skipped while a window is active.\n" +
		"Example: /maint add prod-api sat,sun 02:00-04:00"
}

func (b *Bot) maintAdd(c tele.Context, args []string) error {
	if len(args) != 3 {
		return c.Send(maintUsage())
	}

	days, err := ParseMaintDays(args[1])
	if err != nil {
		return c.Send(err.Error())
	}
	start, end, err := ParseMaintTimeRange(args[2])
	if err != nil {
		return c.Send(err.Error())
	}

	var endpointID sql.NullInt64
	target := "all endpoints"
	if args[0] != "all" {
		ep, err := b.findEndpoint(args[0])
		if err != nil {
			return b.replyLookupError(c, err)
		}
		endpointID = sql.NullInt64{Int64: ep.ID, Valid: true}
		target = ep.Name
	}

	w, err := b.store.AddMaintenanceWindow(b.rootCtx, endpointID, days, start, end)
	if err != nil {
		b.logger.Error("add maintenance window", "target", target, "error", err)
		return c.Send("Internal error. Please try again.")
	}

	b.logger.Info("maintenance window added", "id", w.ID, "target", target, "days", days, "start", start, "end", end)
	return c.Send(fmt.Sprintf("✅ <b>Maintenance window #%d</b> for <b>%s</b>\n%s · %s–%s UTC\nScheduled checks are skipped while active.",
		w.ID, htmlEscape(target), days, formatMaintTime(start), formatMaintTime(end)), tele.NoPreview)
}

func (b *Bot) maintList(c tele.Context) error {
	windows, err := b.store.ListMaintenanceWindows(b.rootCtx)
	if err != nil {
		b.logger.Error("list maintenance windows", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	// Resolve endpoint names for per-endpoint windows.
	names := map[int64]string{}
	if endpoints, err := b.store.ListEndpoints(b.rootCtx); err == nil {
		for _, ep := range endpoints {
			names[ep.ID] = ep.Name
		}
	}
	return c.Send(FormatMaintList(windows, names), tele.NoPreview)
}

func (b *Bot) maintDel(c tele.Context, args []string) error {
	if len(args) != 1 {
		return c.Send("Usage: /maint del <code>&lt;window id&gt;</code> — see /maint list")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Window ID must be a number — see /maint list")
	}
	if err := b.store.DeleteMaintenanceWindow(b.rootCtx, id); err != nil {
		if errors.Is(err, apperror.ErrNotFound) {
			return c.Send(fmt.Sprintf("Maintenance window #%d not found.", id))
		}
		b.logger.Error("delete maintenance window", "id", id, "error", err)
		return c.Send("Internal error. Please try again.")
	}
	b.logger.Info("maintenance window deleted", "id", id)
	return c.Send(fmt.Sprintf("✅ Maintenance window #%d deleted.", id))
}

// formatMaintTime renders minutes since midnight as HH:MM.
func formatMaintTime(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}

// handleDigest sends the 24h uptime digest on demand.
func (b *Bot) handleDigest(c tele.Context) error {
	text, err := monitor.BuildDigest(b.rootCtx, b.store, 24*time.Hour)
	if err != nil {
		b.logger.Error("build digest", "error", err)
		return c.Send("Internal error. Please try again.")
	}
	if text == "" {
		return c.Send("No active endpoints to report on.")
	}
	return c.Send(text, tele.NoPreview)
}

// handleExport sends the full monitor configuration as a JSON document.
func (b *Bot) handleExport(c tele.Context) error {
	endpoints, err := b.store.ListEndpoints(b.rootCtx)
	if err != nil {
		b.logger.Error("export: list endpoints", "error", err)
		return c.Send("Internal error. Please try again.")
	}
	windows, err := b.store.ListMaintenanceWindows(b.rootCtx)
	if err != nil {
		b.logger.Error("export: list maintenance windows", "error", err)
		return c.Send("Internal error. Please try again.")
	}
	data, err := buildExport(endpoints, windows, time.Now())
	if err != nil {
		b.logger.Error("export: build", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	doc := &tele.Document{
		File:     tele.FromReader(bytes.NewReader(data)),
		FileName: fmt.Sprintf("noroshi-export-%s.json", time.Now().UTC().Format("2006-01-02")),
		MIME:     "application/json",
		Caption:  fmt.Sprintf("📦 Noroshi export — %d endpoints, %d maintenance windows", len(endpoints), len(windows)),
	}
	b.logger.Info("config exported", "endpoints", len(endpoints), "maintenance_windows", len(windows))
	return c.Send(doc)
}

package bot

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"noroshi/internal/apperror"
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
		return c.Send("Invalid URL. Must be a valid http:// or https:// address.")
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
		slog.Error("add endpoint", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	if b.scheduler != nil {
		if err := b.scheduler.Add(b.rootCtx, ep); err != nil {
			slog.Error("add to scheduler", "id", ep.ID, "error", err)
			return c.Send(fmt.Sprintf("⚠️ <b>Added endpoint #%d</b>, but scheduling failed — monitoring will start after the next restart.\n\n<b>Name:</b> %s\n<b>URL:</b> <code>%s</code>\n<b>Interval:</b> %s",
				ep.ID, htmlEscape(ep.Name), htmlEscape(ep.URL), FormatDuration(interval)), tele.NoPreview)
		}
	}

	// Run one immediate check so the user gets instant feedback. The scheduled
	// job (started immediately by the scheduler) persists the authoritative state.
	var firstCheck string
	if b.checker != nil {
		code, latency, checkErr := b.checker.Check(b.rootCtx, ep.URL)
		switch {
		case checkErr != nil:
			firstCheck = "\n<b>First check:</b> 🔴 connection error"
		case code >= 200 && code < 300:
			firstCheck = fmt.Sprintf("\n<b>First check:</b> 🟢 HTTP %d · %dms", code, latency.Milliseconds())
		default:
			firstCheck = fmt.Sprintf("\n<b>First check:</b> 🔴 HTTP %d · %dms", code, latency.Milliseconds())
		}
	}

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
		if errors.Is(err, apperror.ErrNotFound) {
			return c.Send("Endpoint not found.")
		}
		slog.Error("find endpoint", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	if b.scheduler != nil {
		b.scheduler.Remove(ep.ID)
	}

	if err := b.store.DeleteEndpoint(b.rootCtx, ep.ID); err != nil {
		slog.Error("delete endpoint", "error", err)
		return c.Send("Internal error. Please try again.")
	}

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
		slog.Error("list endpoints", "error", err)
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
					slog.Error("status check", "id", ep.ID, "error", err)
					return
				}
				endpoints[i] = updated
			}()
		}
		wg.Wait()
	}

	return c.Send(FormatStatus(endpoints), tele.NoPreview)
}

func (b *Bot) sendEndpointList(c tele.Context) error {
	endpoints, err := b.store.ListEndpoints(b.rootCtx)
	if err != nil {
		slog.Error("list endpoints", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	text, markup := FormatEndpointList(endpoints)
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
		if errors.Is(err, apperror.ErrNotFound) {
			return c.Send("Endpoint not found.")
		}
		slog.Error("find endpoint", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	interval, err := time.ParseDuration(args[1])
	if err != nil {
		return c.Send("Invalid interval. Use format like 30s, 5m, 1h")
	}
	if interval < 10*time.Second {
		return c.Send("Interval must be at least 10s")
	}

	if err := b.updateInterval(ep, int(interval.Seconds())); err != nil {
		slog.Error("update interval", "id", ep.ID, "error", err)
		return c.Send("Internal error. Please try again.")
	}

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
	ep.IntervalSeconds = seconds
	if err := b.scheduler.Add(b.rootCtx, ep); err != nil {
		ep.IntervalSeconds = oldSeconds
		if rbErr := b.store.UpdateEndpointInterval(b.rootCtx, ep.ID, oldSeconds); rbErr != nil {
			slog.Error("rollback interval", "id", ep.ID, "error", rbErr)
		}
		if rbErr := b.scheduler.Add(b.rootCtx, ep); rbErr != nil {
			slog.Error("restore job", "id", ep.ID, "error", rbErr)
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
	arg := strings.TrimSpace(c.Message().Payload)
	verb := "resume"
	if pause {
		verb = "pause"
	}
	if arg == "" {
		return c.Send(fmt.Sprintf("Usage: /%s <code>&lt;name or id&gt;</code>", verb))
	}

	ep, err := b.findEndpoint(arg)
	if err != nil {
		if errors.Is(err, apperror.ErrNotFound) {
			return c.Send("Endpoint not found.")
		}
		slog.Error("find endpoint", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	if ep.Paused == pause {
		return c.Send(fmt.Sprintf("%s is already %sd.", htmlEscape(ep.Name), verb))
	}

	if err := b.setPaused(ep, pause); err != nil {
		slog.Error("set paused", "id", ep.ID, "paused", pause, "error", err)
		return c.Send("Internal error. Please try again.")
	}

	if pause {
		return c.Send(fmt.Sprintf("⏸ <b>Paused</b> %s — no more checks until resumed.", htmlEscape(ep.Name)))
	}
	return c.Send(fmt.Sprintf("▶️ <b>Resumed</b> %s — monitoring restarted.", htmlEscape(ep.Name)))
}

// setPaused persists the paused flag and adds/removes the scheduler job.
// On scheduler failure during resume, the store update is rolled back.
func (b *Bot) setPaused(ep storage.Endpoint, pause bool) error {
	if err := b.store.SetEndpointPaused(b.rootCtx, ep.ID, pause); err != nil {
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
		if rbErr := b.store.SetEndpointPaused(b.rootCtx, ep.ID, true); rbErr != nil {
			slog.Error("rollback pause", "id", ep.ID, "error", rbErr)
		}
		return err
	}
	return nil
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

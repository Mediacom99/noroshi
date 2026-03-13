package bot

import (
	"errors"
	"fmt"
	"log/slog"
"strconv"
	"strings"
	"time"

	"noroshi/internal/apperror"
	"noroshi/internal/storage"

	tele "gopkg.in/telebot.v4"
)

func (b *Bot) registerHandlers() {
	b.bot.Handle("/add", b.guarded(b.handleAdd))
	b.bot.Handle("/delete", b.guarded(b.handleDelete))
	b.bot.Handle("/list", b.guarded(b.handleList))
	b.bot.Handle("/interval", b.guarded(b.handleInterval))
	b.bot.Handle("/help", b.guarded(b.handleHelp))

	b.registerCallbacks()
}

func (b *Bot) handleAdd(c tele.Context) error {
	args := strings.Fields(c.Message().Payload)
	if len(args) < 2 {
		return c.Send("Usage: /add <code>&lt;name&gt; &lt;url&gt; [interval]</code>\nExample: /add prod-api https://example.com 30s\nDefault interval: 1m", tele.NoPreview)
	}

	name := args[0]
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
			slog.Error("add to scheduler", "error", err)
		}
	}

	return c.Send(fmt.Sprintf("✅ <b>Added endpoint #%d</b>\n\n<b>Name:</b> %s\n<b>URL:</b> <code>%s</code>\n<b>Interval:</b> %s",
		ep.ID, htmlEscape(ep.Name), htmlEscape(ep.URL), FormatDuration(interval)), tele.NoPreview)
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

	if err := b.store.UpdateEndpointInterval(b.rootCtx, ep.ID, int(interval.Seconds())); err != nil {
		slog.Error("update interval", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	if b.scheduler != nil {
		b.scheduler.Remove(ep.ID)
		ep.IntervalSeconds = int(interval.Seconds())
			if err := b.scheduler.Add(b.rootCtx, ep); err != nil {
			slog.Error("re-add to scheduler", "error", err)
		}
	}

	return c.Send(fmt.Sprintf("✅ <b>Updated interval</b> for %s to %s", htmlEscape(ep.Name), FormatDuration(interval)))
}

func (b *Bot) handleHelp(c tele.Context) error {
	return c.Send(FormatHelp())
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


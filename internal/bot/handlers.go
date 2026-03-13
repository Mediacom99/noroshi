package bot

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"noroshi/internal/apperror"
	"noroshi/internal/storage"

	tele "gopkg.in/telebot.v4"
)

func (b *Bot) registerHandlers() {
	b.bot.Handle("/add", b.handleAdd)
	b.bot.Handle("/delete", b.handleDelete)
	b.bot.Handle("/status", b.handleStatus)
	b.bot.Handle("/list", b.handleList)
	b.bot.Handle("/interval", b.handleInterval)
	b.bot.Handle("/help", b.handleHelp)
}

func (b *Bot) handleAdd(c tele.Context) error {
	args := strings.Fields(c.Message().Payload)
	if len(args) < 2 {
		return c.Send("Usage: /add <url> <interval>\nExample: /add https://example.com 30s")
	}

	rawURL := args[0]
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return c.Send("Invalid URL. Must start with http:// or https://")
	}

	interval, err := time.ParseDuration(args[1])
	if err != nil {
		return c.Send("Invalid interval. Use format like 30s, 5m, 1h")
	}
	if interval < 10*time.Second {
		return c.Send("Interval must be at least 10s")
	}

	ep, err := b.store.AddEndpoint(b.rootCtx, rawURL, int(interval.Seconds()))
	if err != nil {
		if errors.Is(err, apperror.ErrDuplicate) {
			return c.Send("This URL is already being monitored.")
		}
		slog.Error("add endpoint", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	if b.scheduler != nil {
			if err := b.scheduler.Add(b.rootCtx, ep); err != nil {
			slog.Error("add to scheduler", "error", err)
		}
	}

	return c.Send(fmt.Sprintf("✅ Added endpoint #%d\nURL: %s\nInterval: %s", ep.ID, ep.URL, FormatDuration(interval)))
}

func (b *Bot) handleDelete(c tele.Context) error {
	arg := strings.TrimSpace(c.Message().Payload)
	if arg == "" {
		return c.Send("Usage: /delete <id_or_url>")
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

	return c.Send(fmt.Sprintf("🗑 Deleted endpoint #%d (%s)", ep.ID, ep.URL))
}

func (b *Bot) handleStatus(c tele.Context) error {
	endpoints, err := b.store.ListEndpoints(b.rootCtx)
	if err != nil {
		slog.Error("list endpoints", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	if len(endpoints) == 0 {
		return c.Send("No endpoints are being monitored.")
	}

	// Perform immediate checks
	for i, ep := range endpoints {
		statusCode, checkErr := b.checker.Check(b.rootCtx, ep.URL)
		if checkErr != nil {
			endpoints[i].Status = "not_ok"
		} else if statusCode == 200 {
			endpoints[i].Status = "ok"
		} else {
			endpoints[i].Status = "not_ok"
		}
		status := endpoints[i].Status
		b.store.UpdateEndpointStatus(b.rootCtx, ep.ID, status, statusCode)
	}

	return c.Send(FormatEndpointList(endpoints))
}

func (b *Bot) handleList(c tele.Context) error {
	endpoints, err := b.store.ListEndpoints(b.rootCtx)
	if err != nil {
		slog.Error("list endpoints", "error", err)
		return c.Send("Internal error. Please try again.")
	}

	return c.Send(FormatEndpointList(endpoints))
}

func (b *Bot) handleInterval(c tele.Context) error {
	args := strings.Fields(c.Message().Payload)
	if len(args) < 2 {
		return c.Send("Usage: /interval <id_or_url> <new_interval>\nExample: /interval 1 5m")
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

	return c.Send(fmt.Sprintf("✅ Updated endpoint #%d interval to %s", ep.ID, FormatDuration(interval)))
}

func (b *Bot) handleHelp(c tele.Context) error {
	return c.Send(FormatHelp())
}

// findEndpoint tries to find an endpoint by ID first, then by URL.
func (b *Bot) findEndpoint(arg string) (storage.Endpoint, error) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		return b.store.GetEndpoint(b.rootCtx, id)
	}
	return b.store.GetEndpointByURL(b.rootCtx, arg)
}


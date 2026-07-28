package bot

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"
)

func (b *Bot) registerCallbacks() {
	b.bot.Handle(&tele.Btn{Unique: cbDetail}, b.guarded(b.handleDetailCallback))
	b.bot.Handle(&tele.Btn{Unique: cbDelete}, b.guarded(b.handleDeleteCallback))
	b.bot.Handle(&tele.Btn{Unique: cbConfirmDelete}, b.guarded(b.handleConfirmDeleteCallback))
	b.bot.Handle(&tele.Btn{Unique: cbBack}, b.guarded(b.handleBackCallback))
	b.bot.Handle(&tele.Btn{Unique: cbInterval}, b.guarded(b.handleIntervalCallback))
	b.bot.Handle(&tele.Btn{Unique: cbSetInterval}, b.guarded(b.handleSetIntervalCallback))
	b.bot.Handle(&tele.Btn{Unique: cbRefresh}, b.guarded(b.handleRefreshCallback))
	b.bot.Handle(&tele.Btn{Unique: cbCheckNow}, b.guarded(b.handleCheckNowCallback))
	b.bot.Handle(&tele.Btn{Unique: cbPause}, b.guarded(b.handlePauseCallback))
}

// handleDetailCallback shows the detail view for a single endpoint.
func (b *Bot) handleDetailCallback(c tele.Context) error {
	epID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid endpoint."})
	}

	ep, err := b.store.GetEndpoint(b.rootCtx, epID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Endpoint not found."})
	}

	text, markup := FormatEndpointDetail(ep)
	_ = c.Edit(text, markup, tele.NoPreview)
	return c.Respond()
}

// handleDeleteCallback shows a confirmation prompt before deleting.
func (b *Bot) handleDeleteCallback(c tele.Context) error {
	epID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid endpoint."})
	}

	ep, err := b.store.GetEndpoint(b.rootCtx, epID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Endpoint not found."})
	}

	id := strconv.FormatInt(ep.ID, 10)
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("✅ Yes, delete", cbConfirmDelete, id),
			menu.Data("❌ Cancel", cbBack),
		),
	)

	text := fmt.Sprintf("⚠️ <b>Delete endpoint?</b>\n\n<b>%s</b>\n<code>%s</code>",
		htmlEscape(ep.Name), htmlEscape(ep.URL))

	_ = c.Edit(text, menu, tele.NoPreview)
	return c.Respond()
}

// handleConfirmDeleteCallback deletes the endpoint and returns to the list.
func (b *Bot) handleConfirmDeleteCallback(c tele.Context) error {
	epID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid endpoint."})
	}

	ep, err := b.store.GetEndpoint(b.rootCtx, epID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Endpoint not found."})
	}

	if b.scheduler != nil {
		b.scheduler.Remove(ep.ID)
	}

	if err := b.store.DeleteEndpoint(b.rootCtx, ep.ID); err != nil {
		slog.Error("delete endpoint", "error", err)
		return c.Respond(&tele.CallbackResponse{Text: "Error deleting endpoint."})
	}

	_ = c.Respond(&tele.CallbackResponse{Text: "Deleted!"})
	return b.editEndpointList(c)
}

// handleBackCallback returns to the full endpoint list.
func (b *Bot) handleBackCallback(c tele.Context) error {
	_ = c.Respond()
	return b.editEndpointList(c)
}

// handleIntervalCallback shows preset interval buttons.
func (b *Bot) handleIntervalCallback(c tele.Context) error {
	epID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid endpoint."})
	}

	ep, err := b.store.GetEndpoint(b.rootCtx, epID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Endpoint not found."})
	}

	id := strconv.FormatInt(ep.ID, 10)
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(
			menu.Data("30s", cbSetInterval, id, "30"),
			menu.Data("1m", cbSetInterval, id, "60"),
			menu.Data("5m", cbSetInterval, id, "300"),
		),
		menu.Row(
			menu.Data("15m", cbSetInterval, id, "900"),
			menu.Data("1h", cbSetInterval, id, "3600"),
			menu.Data("❌ Cancel", cbBack),
		),
	)

	current := FormatDuration(time.Duration(ep.IntervalSeconds) * time.Second)
	text := fmt.Sprintf("⏱ <b>Change interval for %s</b>\n\nCurrent: %s",
		htmlEscape(ep.Name), current)

	_ = c.Edit(text, menu)
	return c.Respond()
}

// handleSetIntervalCallback applies the chosen interval and returns to the list.
func (b *Bot) handleSetIntervalCallback(c tele.Context) error {
	parts := strings.Split(c.Callback().Data, "|")
	if len(parts) != 2 {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid data."})
	}

	epID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid endpoint."})
	}

	seconds, err := strconv.Atoi(parts[1])
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid interval."})
	}

	ep, err := b.store.GetEndpoint(b.rootCtx, epID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Endpoint not found."})
	}

	if err := b.updateInterval(ep, seconds); err != nil {
		slog.Error("update interval", "id", ep.ID, "error", err)
		return c.Respond(&tele.CallbackResponse{Text: "Error updating interval."})
	}

	interval := FormatDuration(time.Duration(seconds) * time.Second)
	_ = c.Respond(&tele.CallbackResponse{Text: fmt.Sprintf("Interval updated to %s", interval)})
	return b.editEndpointList(c)
}

// handleRefreshCallback re-fetches endpoints and edits the message in-place.
func (b *Bot) handleRefreshCallback(c tele.Context) error {
	_ = c.Respond()
	return b.editEndpointList(c)
}

// handleCheckNowCallback runs an immediate check for one endpoint and
// re-renders its detail view with the fresh result.
func (b *Bot) handleCheckNowCallback(c tele.Context) error {
	epID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid endpoint."})
	}

	ep, err := b.store.GetEndpoint(b.rootCtx, epID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Endpoint not found."})
	}

	if b.scheduler == nil {
		return c.Respond(&tele.CallbackResponse{Text: "Scheduler unavailable."})
	}

	_ = c.Respond(&tele.CallbackResponse{Text: "Checking..."})

	updated, err := b.scheduler.CheckNow(b.rootCtx, ep.ID)
	if err != nil {
		slog.Error("check now", "id", ep.ID, "error", err)
		return c.Respond(&tele.CallbackResponse{Text: "Error running check."})
	}

	text, markup := FormatEndpointDetail(updated)
	_ = c.Edit(text, markup, tele.NoPreview)
	return nil
}

// handlePauseCallback toggles the paused state and re-renders the detail view.
func (b *Bot) handlePauseCallback(c tele.Context) error {
	epID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid endpoint."})
	}

	ep, err := b.store.GetEndpoint(b.rootCtx, epID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Endpoint not found."})
	}

	if err := b.setPaused(ep, !ep.Paused); err != nil {
		slog.Error("set paused", "id", ep.ID, "paused", !ep.Paused, "error", err)
		return c.Respond(&tele.CallbackResponse{Text: "Error updating endpoint."})
	}

	label := "Paused"
	if !ep.Paused {
		label = "Resumed"
	}
	_ = c.Respond(&tele.CallbackResponse{Text: label})

	ep.Paused = !ep.Paused
	text, markup := FormatEndpointDetail(ep)
	_ = c.Edit(text, markup, tele.NoPreview)
	return nil
}

// editEndpointList re-renders the endpoint list and edits the callback message.
func (b *Bot) editEndpointList(c tele.Context) error {
	endpoints, err := b.store.ListEndpoints(b.rootCtx)
	if err != nil {
		slog.Error("list endpoints", "error", err)
		return c.Edit("Internal error. Please try again.")
	}

	text, markup := FormatEndpointList(endpoints)
	if markup == nil {
		return c.Edit(text)
	}
	return c.Edit(text, markup, tele.NoPreview)
}

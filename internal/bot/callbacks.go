package bot

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"
)

// editOrLog edits the callback message, logging failures at debug level.
// Telegram edits fail benignly when the content is unchanged, so these are
// not errors worth surfacing — but they shouldn't vanish silently either.
func (b *Bot) editOrLog(c tele.Context, what interface{}, opts ...interface{}) {
	if err := c.Edit(what, opts...); err != nil {
		b.logger.Debug("callback edit", "error", err)
	}
}

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
	b.bot.Handle(&tele.Btn{Unique: cbUptime}, b.guarded(b.handleUptimeCallback))
	b.bot.Handle(&tele.Btn{Unique: cbIncidents}, b.guarded(b.handleIncidentsCallback))
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

	text, markup := FormatEndpointDetail(ep, b.slowThresholdMs)
	b.editOrLog(c, text, markup, tele.NoPreview)
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

	b.editOrLog(c, text, menu, tele.NoPreview)
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
		b.logger.Error("delete endpoint", "id", ep.ID, "error", err)
		return c.Respond(&tele.CallbackResponse{Text: "Error deleting endpoint."})
	}

	b.logger.Info("endpoint deleted", "id", ep.ID, "name", ep.Name)

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

	b.editOrLog(c, text, menu)
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
		b.logger.Error("update interval", "id", ep.ID, "error", err)
		return c.Respond(&tele.CallbackResponse{Text: "Error updating interval."})
	}

	b.logger.Info("interval updated", "id", ep.ID, "name", ep.Name, "interval", FormatDuration(time.Duration(seconds)*time.Second))
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
		b.logger.Error("check now", "id", ep.ID, "error", err)
		return c.Respond(&tele.CallbackResponse{Text: "Error running check."})
	}

	text, markup := FormatEndpointDetail(updated, b.slowThresholdMs)
	b.editOrLog(c, text, markup, tele.NoPreview)
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

	if err := b.setPaused(ep, !ep.Paused, sql.NullTime{}); err != nil {
		b.logger.Error("set paused", "id", ep.ID, "paused", !ep.Paused, "error", err)
		return c.Respond(&tele.CallbackResponse{Text: "Error updating endpoint."})
	}

	label := "Paused"
	if !ep.Paused {
		label = "Resumed"
	}
	b.logger.Info("endpoint "+strings.ToLower(label), "id", ep.ID, "name", ep.Name)
	_ = c.Respond(&tele.CallbackResponse{Text: label})

	ep.Paused = !ep.Paused
	text, markup := FormatEndpointDetail(ep, b.slowThresholdMs)
	b.editOrLog(c, text, markup, tele.NoPreview)
	return nil
}

// editEndpointList re-renders the endpoint list and edits the callback message.
func (b *Bot) editEndpointList(c tele.Context) error {
	endpoints, err := b.store.ListEndpoints(b.rootCtx)
	if err != nil {
		b.logger.Error("list endpoints", "error", err)
		b.editOrLog(c, "Internal error. Please try again.")
		return nil
	}

	text, markup := FormatEndpointList(endpoints, b.slowThresholdMs)
	if markup == nil {
		b.editOrLog(c, text)
		return nil
	}
	b.editOrLog(c, text, markup, tele.NoPreview)
	return nil
}

// handleUptimeCallback shows the uptime stats view for one endpoint.
func (b *Bot) handleUptimeCallback(c tele.Context) error {
	epID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid endpoint."})
	}

	ep, err := b.store.GetEndpoint(b.rootCtx, epID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Endpoint not found."})
	}

	stats, err := b.collectStats(ep.ID)
	if err != nil {
		b.logger.Error("collect stats", "id", ep.ID, "error", err)
		return c.Respond(&tele.CallbackResponse{Text: "Error loading stats."})
	}

	labels := make([]string, len(uptimeWindows))
	for i, w := range uptimeWindows {
		labels[i] = w.label
	}

	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("◀ Back", cbDetail, strconv.FormatInt(ep.ID, 10))))
	b.editOrLog(c, FormatUptime(ep, labels, stats), menu, tele.NoPreview)
	return c.Respond()
}

// handleIncidentsCallback shows the incident history view for one endpoint.
func (b *Bot) handleIncidentsCallback(c tele.Context) error {
	epID, err := strconv.ParseInt(c.Callback().Data, 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid endpoint."})
	}

	ep, err := b.store.GetEndpoint(b.rootCtx, epID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Endpoint not found."})
	}

	transitions, err := b.store.GetRecentTransitions(b.rootCtx, ep.ID, 20)
	if err != nil {
		b.logger.Error("get transitions", "id", ep.ID, "error", err)
		return c.Respond(&tele.CallbackResponse{Text: "Error loading incidents."})
	}

	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("◀ Back", cbDetail, strconv.FormatInt(ep.ID, 10))))
	b.editOrLog(c, FormatIncidents(ep, transitions), menu, tele.NoPreview)
	return c.Respond()
}

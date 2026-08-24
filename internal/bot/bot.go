package bot

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"noroshi/internal/monitor"
	"noroshi/internal/storage"

	tele "gopkg.in/telebot.v4"
)

// Store defines the storage methods the bot needs.
type Store interface {
	AddEndpoint(ctx context.Context, name, url string, intervalSeconds int) (storage.Endpoint, error)
	GetEndpoint(ctx context.Context, id int64) (storage.Endpoint, error)
	GetEndpointByURL(ctx context.Context, url string) (storage.Endpoint, error)
	GetEndpointByName(ctx context.Context, name string) (storage.Endpoint, error)
	DeleteEndpoint(ctx context.Context, id int64) error
	ListEndpoints(ctx context.Context) ([]storage.Endpoint, error)
	UpdateEndpointInterval(ctx context.Context, id int64, intervalSeconds int) error
	SetEndpointPaused(ctx context.Context, id int64, paused bool, until sql.NullTime) error
	SetExpectedStatus(ctx context.Context, id int64, code int) error
	SetExpectedKeyword(ctx context.Context, id int64, keyword string) error
	RenameEndpoint(ctx context.Context, id int64, newName string) error
	GetCheckStats(ctx context.Context, endpointID int64, since time.Time) (storage.WindowStats, error)
	GetRecentTransitions(ctx context.Context, endpointID int64, limit int) ([]storage.CheckTransition, error)
	AddMaintenanceWindow(ctx context.Context, endpointID sql.NullInt64, days string, startMinutes, endMinutes int) (storage.MaintenanceWindow, error)
	ListMaintenanceWindows(ctx context.Context) ([]storage.MaintenanceWindow, error)
	DeleteMaintenanceWindow(ctx context.Context, id int64) error
}

// Scheduler defines the scheduling methods the bot needs.
type Scheduler interface {
	Add(ctx context.Context, ep storage.Endpoint) error
	Remove(endpointID int64) error
	CheckNow(ctx context.Context, endpointID int64) (storage.Endpoint, error)
}

// Checker performs an immediate HTTP health check (used by /add feedback).
type Checker interface {
	Check(ctx context.Context, url string, opts monitor.CheckOptions) monitor.CheckResult
}

// Bot wraps the Telegram bot with application logic.
type Bot struct {
	bot             *tele.Bot
	store           Store
	scheduler       Scheduler
	checker         Checker
	chatID          int64
	slowThresholdMs int64
	webhookMode     bool
	rootCtx         context.Context
	logger          *slog.Logger
}

// WebhookConfig switches the bot from long polling to webhook mode: Telegram
// POSTs updates to PublicURL (must be https; TLS usually terminates at a
// reverse proxy), which must forward to the bot's plain-HTTP listener on
// Port. Secret, when set, is verified against the X-Telegram-Bot-Api-Secret-Token
// header so only Telegram can inject updates.
type WebhookConfig struct {
	PublicURL string
	Port      int
	Secret    string
}

// choosePoller builds the update poller: webhook when configured (registering
// the public URL with Telegram), long polling otherwise.
func choosePoller(webhook *WebhookConfig) tele.Poller {
	if webhook == nil {
		return &tele.LongPoller{Timeout: 10 * time.Second}
	}
	return &tele.Webhook{
		Listen:      fmt.Sprintf(":%d", webhook.Port),
		SecretToken: webhook.Secret,
		DropUpdates: true, // don't replay updates queued while the bot was down
		Endpoint:    &tele.WebhookEndpoint{PublicURL: webhook.PublicURL},
	}
}

// NewBot creates a Bot. SetScheduler must be called before Start.
// webhook switches to webhook mode when non-nil (nil = long polling).
// slowThresholdMs marks healthy endpoints as "slow" above this latency (0 = disabled).
// logger is scoped with a "component" attribute by the caller; nil falls back
// to slog.Default().
func NewBot(token string, chatID int64, store Store, checker Checker, slowThresholdMs int64, webhook *WebhookConfig, rootCtx context.Context, logger *slog.Logger) (*Bot, error) {
	if logger == nil {
		logger = slog.Default()
	}
	pref := tele.Settings{
		Token:     token,
		Poller:    choosePoller(webhook),
		ParseMode: tele.ModeHTML,
		// Route handler errors through slog instead of telebot's default logger.
		// "message is not modified" is benign (user tapped refresh with no
		// changes) — keep it out of the error logs.
		OnError: func(err error, c tele.Context) {
			if err != nil && strings.Contains(err.Error(), "message is not modified") {
				logger.Debug("telegram handler: message not modified")
				return
			}
			logger.Error("telegram handler error", "error", err)
		},
	}

	tb, err := tele.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}

	b := &Bot{
		bot:             tb,
		store:           store,
		checker:         checker,
		chatID:          chatID,
		slowThresholdMs: slowThresholdMs,
		webhookMode:     webhook != nil,
		rootCtx:         rootCtx,
		logger:          logger,
	}

	b.registerHandlers()
	b.registerCommands()
	return b, nil
}

// SetScheduler sets the scheduler reference (resolves circular dependency).
func (b *Bot) SetScheduler(s Scheduler) {
	b.scheduler = s
}

// Start begins the bot poller in a goroutine.
func (b *Bot) Start() {
	go b.bot.Start()
	b.logger.Info("telegram bot started")
}

// Stop stops the bot poller. In webhook mode the webhook is deregistered
// first so Telegram stops queueing updates for this instance.
func (b *Bot) Stop() {
	if b.webhookMode {
		if err := b.bot.RemoveWebhook(); err != nil {
			b.logger.Error("remove webhook", "error", err)
		}
	}
	b.bot.Stop()
	b.logger.Info("telegram bot stopped")
}

func (b *Bot) registerCommands() {
	err := b.bot.SetCommands([]tele.Command{
		{Text: "list", Description: "View all monitored endpoints"},
		{Text: "add", Description: "Add endpoint: /add <name> <url> [interval]"},
		{Text: "delete", Description: "Remove an endpoint"},
		{Text: "interval", Description: "Change check interval"},
		{Text: "status", Description: "Check all endpoints now"},
		{Text: "uptime", Description: "Uptime stats: /uptime <name>"},
		{Text: "incidents", Description: "Outage history: /incidents <name>"},
		{Text: "pause", Description: "Pause monitoring an endpoint"},
		{Text: "resume", Description: "Resume monitoring an endpoint"},
		{Text: "expect", Description: "Require an exact HTTP status"},
		{Text: "keyword", Description: "Require/forbid text or regex in the response"},
		{Text: "rename", Description: "Rename an endpoint"},
		{Text: "maint", Description: "Maintenance windows: /maint add|list|del"},
		{Text: "digest", Description: "24h uptime summary"},
		{Text: "export", Description: "Export config as JSON"},
		{Text: "help", Description: "Show help and usage info"},
	})
	if err != nil {
		b.logger.Error("register commands", "error", err)
	}
}

// guarded wraps a handler to ignore messages from chats other than the configured one.
func (b *Bot) guarded(h tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		// Chat() can be nil for some update types (e.g. channel posts) —
		// guard against a nil dereference.
		chat := c.Chat()
		if chat == nil || chat.ID != b.chatID {
			return nil
		}
		return h(c)
	}
}

// SendMessage sends a text message to the configured chat ID and returns the
// sent message (its ID is used to thread the recovery as a reply).
// markup is optional and may be nil.
func (b *Bot) SendMessage(text string, markup *tele.ReplyMarkup) (*tele.Message, error) {
	return b.send(text, markup, false, 0)
}

// SendSilentReply sends a message without notification sound, as a reply to
// the given alert message when replyToID > 0. markup is optional and may be nil.
func (b *Bot) SendSilentReply(text string, markup *tele.ReplyMarkup, replyToID int64) error {
	_, err := b.send(text, markup, true, replyToID)
	return err
}

func (b *Bot) send(text string, markup *tele.ReplyMarkup, silent bool, replyToID int64) (*tele.Message, error) {
	opts := &tele.SendOptions{
		// A passed *SendOptions REPLACES telebot's defaults (incl. the bot's
		// ParseMode), so HTML must be set explicitly here.
		ParseMode:             tele.ModeHTML,
		DisableWebPagePreview: true,
		DisableNotification:   silent,
		ReplyMarkup:           markup,
	}
	if replyToID > 0 {
		opts.ReplyTo = &tele.Message{ID: int(replyToID)}
	}
	return b.bot.Send(&tele.Chat{ID: b.chatID}, text, opts)
}

// TelegramNotifier implements monitor.Notifier using the bot.
type TelegramNotifier struct {
	bot     *Bot
	maxFail int
}

// NewTelegramNotifier creates a notifier that sends messages via the bot.
func NewTelegramNotifier(bot *Bot, maxFail int) *TelegramNotifier {
	return &TelegramNotifier{bot: bot, maxFail: maxFail}
}

// NotifyFailure sends a failure notification to the configured chat.
// The alert carries action buttons: check now, pause, detail.
// Returns the sent message ID so the recovery can be threaded as a reply.
func (n *TelegramNotifier) NotifyFailure(ctx context.Context, ep storage.Endpoint) (int64, error) {
	var msg string
	if ep.LastStatusCode > 0 {
		msg = FormatFailureWithCode(ep, ep.LastStatusCode, n.maxFail)
	} else {
		msg = FormatFailure(ep, n.maxFail)
	}
	m, err := n.bot.SendMessage(msg, AlertKeyboard(ep))
	if err != nil {
		return 0, err
	}
	if m == nil {
		return 0, nil
	}
	return int64(m.ID), nil
}

// NotifyRecovery sends a silent recovery notification to the configured chat,
// threaded as a reply to the original failure alert when available.
func (n *TelegramNotifier) NotifyRecovery(ctx context.Context, ep storage.Endpoint, downtime time.Duration) error {
	msg := FormatRecovery(ep, downtime)
	return n.bot.SendSilentReply(msg, RecoveryKeyboard(ep), ep.AlertMessageID)
}

// NotifyCertExpiry sends a silent certificate-expiry warning.
func (n *TelegramNotifier) NotifyCertExpiry(ctx context.Context, ep storage.Endpoint, daysLeft int) error {
	msg := FormatCertWarning(ep, daysLeft)
	return n.bot.SendSilentReply(msg, RecoveryKeyboard(ep), 0)
}

// NotifyDigest sends the periodic uptime digest as a silent message.
func (n *TelegramNotifier) NotifyDigest(ctx context.Context, text string) error {
	return n.bot.SendSilentReply(text, nil, 0)
}

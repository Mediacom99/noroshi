package bot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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
}

// Scheduler defines the scheduling methods the bot needs.
type Scheduler interface {
	Add(ctx context.Context, ep storage.Endpoint) error
	Remove(endpointID int64) error
}

// Bot wraps the Telegram bot with application logic.
type Bot struct {
	bot       *tele.Bot
	store     Store
	scheduler Scheduler
	chatID    int64
	rootCtx   context.Context
}

// NewBot creates a Bot. SetScheduler must be called before Start.
func NewBot(token string, chatID int64, store Store, rootCtx context.Context) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	tb, err := tele.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}

	b := &Bot{
		bot:     tb,
		store:   store,
		chatID:  chatID,
		rootCtx: rootCtx,
	}

	b.registerHandlers()
	return b, nil
}

// SetScheduler sets the scheduler reference (resolves circular dependency).
func (b *Bot) SetScheduler(s Scheduler) {
	b.scheduler = s
}

// Start begins the bot poller in a goroutine.
func (b *Bot) Start() {
	go b.bot.Start()
	slog.Info("telegram bot started")
}

// Stop stops the bot poller.
func (b *Bot) Stop() {
	b.bot.Stop()
	slog.Info("telegram bot stopped")
}

// SendMessage sends a text message to the configured chat ID.
func (b *Bot) SendMessage(text string) error {
	chat := &tele.Chat{ID: b.chatID}
	_, err := b.bot.Send(chat, text)
	return err
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
func (n *TelegramNotifier) NotifyFailure(ctx context.Context, ep storage.Endpoint) error {
	msg := FormatFailure(ep, n.maxFail)
	return n.bot.SendMessage(msg)
}

// NotifyRecovery sends a recovery notification to the configured chat.
func (n *TelegramNotifier) NotifyRecovery(ctx context.Context, ep storage.Endpoint, downtime time.Duration) error {
	msg := FormatRecovery(ep, downtime)
	return n.bot.SendMessage(msg)
}

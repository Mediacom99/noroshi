package bot

import (
	"testing"

	tele "gopkg.in/telebot.v4"
)

func TestChoosePoller(t *testing.T) {
	t.Run("nil config uses long polling", func(t *testing.T) {
		p := choosePoller(nil)
		if _, ok := p.(*tele.LongPoller); !ok {
			t.Errorf("poller = %T, want *tele.LongPoller", p)
		}
	})

	t.Run("webhook config", func(t *testing.T) {
		p := choosePoller(&WebhookConfig{
			PublicURL: "https://noroshi.example.com/telegram",
			Port:      8081,
			Secret:    "s3cret",
		})
		w, ok := p.(*tele.Webhook)
		if !ok {
			t.Fatalf("poller = %T, want *tele.Webhook", p)
		}
		if w.Listen != ":8081" {
			t.Errorf("Listen = %q, want :8081", w.Listen)
		}
		if w.SecretToken != "s3cret" {
			t.Errorf("SecretToken = %q", w.SecretToken)
		}
		if w.Endpoint == nil || w.Endpoint.PublicURL != "https://noroshi.example.com/telegram" {
			t.Errorf("Endpoint = %+v", w.Endpoint)
		}
		if !w.DropUpdates {
			t.Error("DropUpdates should be true (no replay after downtime)")
		}
	})
}

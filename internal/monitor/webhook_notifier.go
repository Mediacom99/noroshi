package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"noroshi/internal/storage"
)

// alertPayload is the JSON body POSTed to the alert webhook for every event.
type alertPayload struct {
	Event           string          `json:"event"` // failure | recovery | cert_expiry | digest
	Timestamp       time.Time       `json:"timestamp"`
	Endpoint        *alertEndpoint  `json:"endpoint,omitempty"`
	DowntimeSeconds int64           `json:"downtime_seconds,omitempty"` // recovery
	DaysLeft        int             `json:"days_left,omitempty"`        // cert_expiry
	Text            string          `json:"text,omitempty"`             // digest
}

type alertEndpoint struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	LatencyMs  int64  `json:"latency_ms"`
	Reason     string `json:"reason,omitempty"`
}

// WebhookNotifier implements Notifier by POSTing JSON events to a generic
// webhook URL. Delivery is fire-and-forget: one POST, no retries, and
// NotifyFailure returns message ID 0 (only Telegram has message IDs).
type WebhookNotifier struct {
	url   string
	token string // sent as "Authorization: Bearer <token>" when non-empty
	http  *http.Client
}

// NewWebhookNotifier creates a notifier POSTing alerts to url.
func NewWebhookNotifier(url, token string) *WebhookNotifier {
	return &WebhookNotifier{
		url:   url,
		token: token,
		http:  &http.Client{Timeout: 10 * time.Second},
	}
}

// NotifyFailure POSTs a "failure" event.
func (n *WebhookNotifier) NotifyFailure(ctx context.Context, ep storage.Endpoint) (int64, error) {
	return 0, n.post(ctx, alertPayload{
		Event:     "failure",
		Timestamp: time.Now().UTC(),
		Endpoint:  webhookEndpoint(ep),
	})
}

// NotifyRecovery POSTs a "recovery" event with the outage duration.
func (n *WebhookNotifier) NotifyRecovery(ctx context.Context, ep storage.Endpoint, downtime time.Duration) error {
	return n.post(ctx, alertPayload{
		Event:           "recovery",
		Timestamp:       time.Now().UTC(),
		Endpoint:        webhookEndpoint(ep),
		DowntimeSeconds: int64(downtime.Seconds()),
	})
}

// NotifyCertExpiry POSTs a "cert_expiry" event.
func (n *WebhookNotifier) NotifyCertExpiry(ctx context.Context, ep storage.Endpoint, daysLeft int) error {
	return n.post(ctx, alertPayload{
		Event:     "cert_expiry",
		Timestamp: time.Now().UTC(),
		Endpoint:  webhookEndpoint(ep),
		DaysLeft:  daysLeft,
	})
}

// NotifyDigest POSTs a "digest" event carrying the rendered digest text.
func (n *WebhookNotifier) NotifyDigest(ctx context.Context, text string) error {
	return n.post(ctx, alertPayload{
		Event:     "digest",
		Timestamp: time.Now().UTC(),
		Text:      text,
	})
}

func webhookEndpoint(ep storage.Endpoint) *alertEndpoint {
	return &alertEndpoint{
		ID:         ep.ID,
		Name:       ep.Name,
		URL:        ep.URL,
		StatusCode: ep.LastStatusCode,
		LatencyMs:  ep.LastLatencyMs,
		Reason:     ep.LastCheckError,
	}
}

func (n *WebhookNotifier) post(ctx context.Context, p alertPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal alert payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if n.token != "" {
		req.Header.Set("Authorization", "Bearer "+n.token)
	}

	resp, err := n.http.Do(req)
	if err != nil {
		return fmt.Errorf("post alert webhook: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("alert webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// MultiNotifier fans notifications out to several notifiers. A failing
// channel is logged and never affects the others; an error is returned only
// when EVERY notifier fails. NotifyFailure returns the first non-zero
// message ID (the Telegram alert ID used for threading recoveries).
type MultiNotifier struct {
	notifiers []Notifier
	logger    *slog.Logger
}

// NewMultiNotifier creates a Notifier that calls every given notifier.
// logger may be nil (falls back to slog.Default()).
func NewMultiNotifier(logger *slog.Logger, notifiers ...Notifier) *MultiNotifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &MultiNotifier{notifiers: notifiers, logger: logger}
}

// NotifyFailure calls all notifiers and returns the first non-zero message ID.
func (m *MultiNotifier) NotifyFailure(ctx context.Context, ep storage.Endpoint) (int64, error) {
	var msgID int64
	var errs []error
	for i, n := range m.notifiers {
		id, err := n.NotifyFailure(ctx, ep)
		if err != nil {
			m.logger.Error("notifier failed", "notifier", i, "event", "failure", "error", err)
			errs = append(errs, err)
			continue
		}
		if msgID == 0 {
			msgID = id
		}
	}
	return msgID, m.allFailed(errs)
}

// NotifyRecovery calls all notifiers.
func (m *MultiNotifier) NotifyRecovery(ctx context.Context, ep storage.Endpoint, downtime time.Duration) error {
	var errs []error
	for i, n := range m.notifiers {
		if err := n.NotifyRecovery(ctx, ep, downtime); err != nil {
			m.logger.Error("notifier failed", "notifier", i, "event", "recovery", "error", err)
			errs = append(errs, err)
		}
	}
	return m.allFailed(errs)
}

// NotifyCertExpiry calls all notifiers.
func (m *MultiNotifier) NotifyCertExpiry(ctx context.Context, ep storage.Endpoint, daysLeft int) error {
	var errs []error
	for i, n := range m.notifiers {
		if err := n.NotifyCertExpiry(ctx, ep, daysLeft); err != nil {
			m.logger.Error("notifier failed", "notifier", i, "event", "cert_expiry", "error", err)
			errs = append(errs, err)
		}
	}
	return m.allFailed(errs)
}

// NotifyDigest calls all notifiers.
func (m *MultiNotifier) NotifyDigest(ctx context.Context, text string) error {
	var errs []error
	for i, n := range m.notifiers {
		if err := n.NotifyDigest(ctx, text); err != nil {
			m.logger.Error("notifier failed", "notifier", i, "event", "digest", "error", err)
			errs = append(errs, err)
		}
	}
	return m.allFailed(errs)
}

// allFailed returns a joined error only when every notifier failed; partial
// failures are already logged and must not fail the caller's flow.
func (m *MultiNotifier) allFailed(errs []error) error {
	if len(errs) > 0 && len(errs) == len(m.notifiers) {
		return errors.Join(errs...)
	}
	return nil
}

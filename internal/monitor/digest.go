package monitor

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"noroshi/internal/storage"
)

// DigestConfig controls the periodic uptime digest. The zero value disables it.
type DigestConfig struct {
	Mode        string // "daily" or "weekly"; "" = off
	TimeMinutes int    // minutes since midnight UTC
}

// Window returns the stats period the digest covers for its mode.
func (c DigestConfig) Window() time.Duration {
	if c.Mode == "weekly" {
		return 7 * 24 * time.Hour
	}
	return 24 * time.Hour
}

// DigestStore is the storage surface digest building needs.
type DigestStore interface {
	ListEndpoints(ctx context.Context) ([]storage.Endpoint, error)
	GetCheckStats(ctx context.Context, endpointID int64, since time.Time) (storage.WindowStats, error)
}

// BuildDigest gathers per-endpoint stats over window and renders the digest
// message. Returns "" when there are no active (non-paused) endpoints.
func BuildDigest(ctx context.Context, store DigestStore, window time.Duration) (string, error) {
	endpoints, err := store.ListEndpoints(ctx)
	if err != nil {
		return "", fmt.Errorf("list endpoints: %w", err)
	}
	active := make([]storage.Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		if !ep.Paused {
			active = append(active, ep)
		}
	}
	if len(active) == 0 {
		return "", nil
	}

	stats := make(map[int64]storage.WindowStats, len(active))
	since := time.Now().UTC().Add(-window)
	for _, ep := range active {
		st, err := store.GetCheckStats(ctx, ep.ID, since)
		if err != nil {
			return "", fmt.Errorf("get stats for endpoint %d: %w", ep.ID, err)
		}
		stats[ep.ID] = st
	}
	return FormatDigest(active, stats, window), nil
}

// FormatDigest renders the digest message (Telegram HTML).
func FormatDigest(endpoints []storage.Endpoint, stats map[int64]storage.WindowStats, window time.Duration) string {
	label := "24h"
	if window >= 7*24*time.Hour {
		label = "7d"
	}

	up, down := 0, 0
	for _, ep := range endpoints {
		switch ep.Status {
		case "ok":
			up++
		case "not_ok":
			down++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "📊 <b>Noroshi digest</b> — last %s\n", label)
	fmt.Fprintf(&b, "%d endpoints · %d up · %d down", len(endpoints), up, down)

	for _, ep := range endpoints {
		icon := "⚪"
		switch ep.Status {
		case "ok":
			icon = "🟢"
		case "not_ok":
			icon = "🔴"
		}
		st := stats[ep.ID]
		if st.Total == 0 {
			fmt.Fprintf(&b, "\n%s <b>%s</b> — no checks in window", icon, html.EscapeString(ep.Name))
			continue
		}
		fmt.Fprintf(&b, "\n%s <b>%s</b> — %.1f%% up · avg %dms · %d incidents",
			icon, html.EscapeString(ep.Name), st.Uptime(), int64(st.AvgLatencyMs), st.Incidents)
	}
	return b.String()
}

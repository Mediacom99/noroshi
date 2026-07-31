package storage

import (
	"database/sql"
	"time"
)

// Endpoint represents a monitored endpoint.
type Endpoint struct {
	ID                       int64
	Name                     string
	URL                      string
	IntervalSeconds          int
	Status                   string
	LastCheckedAt            sql.NullTime
	LastFailureAt            sql.NullTime
	ConsecutiveFailures      int
	FailureNotificationsSent int
	LastStatusCode           int
	LastLatencyMs            int64
	LastCheckError           string
	ExpectedStatus           int
	ExpectedKeyword          string
	CertExpiresAt            sql.NullTime
	LastCertWarningAt        sql.NullTime
	Paused                   bool
	PausedUntil              sql.NullTime
	LastNotifiedAt           sql.NullTime
	AlertMessageID           int64
	CreatedAt                time.Time
}

// CheckOutcome carries the persisted result of a single health check.
type CheckOutcome struct {
	Status     string // "ok" or "not_ok"
	StatusCode int
	LatencyMs  int64
	Reason     string // human-readable failure reason, "" when ok
	CertExpiry sql.NullTime
}

// WindowStats aggregates check results over a time window.
type WindowStats struct {
	Total        int
	Up           int
	AvgLatencyMs float64
	P95LatencyMs int64
	Incidents    int
}

// Uptime returns the percentage of successful checks in the window.
func (w WindowStats) Uptime() float64 {
	if w.Total == 0 {
		return 100
	}
	return float64(w.Up) / float64(w.Total) * 100
}

// CheckTransition is a single up/down state flip in the check history.
type CheckTransition struct {
	CheckedAt  time.Time
	Up         bool
	StatusCode int
}

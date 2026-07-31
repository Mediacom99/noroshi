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

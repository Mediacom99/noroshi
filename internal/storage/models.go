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
	CreatedAt                time.Time
}

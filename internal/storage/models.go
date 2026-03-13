package storage

import (
	"database/sql"
	"time"
)

// Endpoint represents a monitored endpoint.
type Endpoint struct {
	ID                       int64
	URL                      string
	IntervalSeconds          int
	Status                   string
	LastCheckedAt            sql.NullTime
	LastFailureAt            sql.NullTime
	ConsecutiveFailures      int
	FailureNotificationsSent int
	CreatedAt                time.Time
}

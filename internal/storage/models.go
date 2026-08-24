package storage

import (
	"database/sql"
	"strings"
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

// MaintenanceWindow is a recurring UTC time window during which scheduled
// checks are skipped. EndpointID NULL means the window applies to every
// endpoint. StartMinutes/EndMinutes are minutes since midnight UTC; an
// EndMinutes earlier than StartMinutes is an overnight window (22:00–02:00).
type MaintenanceWindow struct {
	ID           int64
	EndpointID   sql.NullInt64
	Days         string // "all" or comma-separated "mon,wed,..."
	StartMinutes int
	EndMinutes   int
}

// Applies reports whether now falls inside the window. All times are UTC.
// Overnight windows after midnight belong to the previous day's schedule.
func (w MaintenanceWindow) Applies(now time.Time) bool {
	now = now.UTC()
	m := now.Hour()*60 + now.Minute()

	dayOK := func(t time.Time) bool {
		if w.Days == "all" {
			return true
		}
		day := strings.ToLower(t.Weekday().String()[:3])
		for _, d := range strings.Split(w.Days, ",") {
			if strings.TrimSpace(d) == day {
				return true
			}
		}
		return false
	}

	if w.StartMinutes <= w.EndMinutes {
		return m >= w.StartMinutes && m < w.EndMinutes && dayOK(now)
	}
	// Overnight window: the post-midnight portion belongs to yesterday.
	if m >= w.StartMinutes {
		return dayOK(now)
	}
	return m < w.EndMinutes && dayOK(now.AddDate(0, 0, -1))
}

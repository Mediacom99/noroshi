package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"noroshi/internal/apperror"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// OpenDB opens a SQLite database with WAL mode and busy timeout.
func OpenDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}

// RunMigrations runs all pending goose migrations using the embedded SQL files.
func RunMigrations(db *sql.DB) error {
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// SQLiteStore implements endpoint storage using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLiteStore.
func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) AddEndpoint(ctx context.Context, name, url string, intervalSeconds int) (Endpoint, error) {
	result, err := s.db.ExecContext(ctx,
		"INSERT INTO endpoints (name, url, interval_seconds) VALUES (?, ?, ?)",
		name, url, intervalSeconds,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Endpoint{}, apperror.Wrap(apperror.ErrDuplicate, err)
		}
		return Endpoint{}, apperror.Wrap(apperror.ErrDatabase, err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return Endpoint{}, apperror.Wrap(apperror.ErrDatabase, err)
	}

	return s.GetEndpoint(ctx, id)
}

func (s *SQLiteStore) GetEndpoint(ctx context.Context, id int64) (Endpoint, error) {
	var ep Endpoint
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, url, interval_seconds, status, last_checked_at, last_failure_at,
		        consecutive_failures, failure_notifications_sent, last_status_code, last_latency_ms,
		        last_check_error, expected_status, expected_keyword, cert_expires_at, last_cert_warning_at,
		        paused, paused_until, last_notified_at, alert_message_id, created_at
		 FROM endpoints WHERE id = ?`, id,
	).Scan(&ep.ID, &ep.Name, &ep.URL, &ep.IntervalSeconds, &ep.Status,
		&ep.LastCheckedAt, &ep.LastFailureAt,
		&ep.ConsecutiveFailures, &ep.FailureNotificationsSent, &ep.LastStatusCode, &ep.LastLatencyMs,
		&ep.LastCheckError, &ep.ExpectedStatus, &ep.ExpectedKeyword, &ep.CertExpiresAt, &ep.LastCertWarningAt,
		&ep.Paused, &ep.PausedUntil, &ep.LastNotifiedAt, &ep.AlertMessageID, &ep.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return Endpoint{}, apperror.Wrap(apperror.ErrNotFound, err)
	}
	if err != nil {
		return Endpoint{}, apperror.Wrap(apperror.ErrDatabase, err)
	}
	return ep, nil
}

func (s *SQLiteStore) GetEndpointByURL(ctx context.Context, url string) (Endpoint, error) {
	var ep Endpoint
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, url, interval_seconds, status, last_checked_at, last_failure_at,
		        consecutive_failures, failure_notifications_sent, last_status_code, last_latency_ms,
		        last_check_error, expected_status, expected_keyword, cert_expires_at, last_cert_warning_at,
		        paused, paused_until, last_notified_at, alert_message_id, created_at
		 FROM endpoints WHERE url = ?`, url,
	).Scan(&ep.ID, &ep.Name, &ep.URL, &ep.IntervalSeconds, &ep.Status,
		&ep.LastCheckedAt, &ep.LastFailureAt,
		&ep.ConsecutiveFailures, &ep.FailureNotificationsSent, &ep.LastStatusCode, &ep.LastLatencyMs,
		&ep.LastCheckError, &ep.ExpectedStatus, &ep.ExpectedKeyword, &ep.CertExpiresAt, &ep.LastCertWarningAt,
		&ep.Paused, &ep.PausedUntil, &ep.LastNotifiedAt, &ep.AlertMessageID, &ep.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return Endpoint{}, apperror.Wrap(apperror.ErrNotFound, err)
	}
	if err != nil {
		return Endpoint{}, apperror.Wrap(apperror.ErrDatabase, err)
	}
	return ep, nil
}

func (s *SQLiteStore) GetEndpointByName(ctx context.Context, name string) (Endpoint, error) {
	var ep Endpoint
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, url, interval_seconds, status, last_checked_at, last_failure_at,
		        consecutive_failures, failure_notifications_sent, last_status_code, last_latency_ms,
		        last_check_error, expected_status, expected_keyword, cert_expires_at, last_cert_warning_at,
		        paused, paused_until, last_notified_at, alert_message_id, created_at
		 FROM endpoints WHERE name = ?`, name,
	).Scan(&ep.ID, &ep.Name, &ep.URL, &ep.IntervalSeconds, &ep.Status,
		&ep.LastCheckedAt, &ep.LastFailureAt,
		&ep.ConsecutiveFailures, &ep.FailureNotificationsSent, &ep.LastStatusCode, &ep.LastLatencyMs,
		&ep.LastCheckError, &ep.ExpectedStatus, &ep.ExpectedKeyword, &ep.CertExpiresAt, &ep.LastCertWarningAt,
		&ep.Paused, &ep.PausedUntil, &ep.LastNotifiedAt, &ep.AlertMessageID, &ep.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return Endpoint{}, apperror.Wrap(apperror.ErrNotFound, err)
	}
	if err != nil {
		return Endpoint{}, apperror.Wrap(apperror.ErrDatabase, err)
	}
	return ep, nil
}

func (s *SQLiteStore) DeleteEndpoint(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM endpoints WHERE id = ?", id)
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	if rows == 0 {
		return apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("endpoint %d not found", id))
	}
	return nil
}

func (s *SQLiteStore) ListEndpoints(ctx context.Context) ([]Endpoint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, url, interval_seconds, status, last_checked_at, last_failure_at,
		        consecutive_failures, failure_notifications_sent, last_status_code, last_latency_ms,
		        last_check_error, expected_status, expected_keyword, cert_expires_at, last_cert_warning_at,
		        paused, paused_until, last_notified_at, alert_message_id, created_at
		 FROM endpoints ORDER BY id`,
	)
	if err != nil {
		return nil, apperror.Wrap(apperror.ErrDatabase, err)
	}
	defer rows.Close()

	var endpoints []Endpoint
	for rows.Next() {
		var ep Endpoint
		if err := rows.Scan(&ep.ID, &ep.Name, &ep.URL, &ep.IntervalSeconds, &ep.Status,
			&ep.LastCheckedAt, &ep.LastFailureAt,
			&ep.ConsecutiveFailures, &ep.FailureNotificationsSent, &ep.LastStatusCode, &ep.LastLatencyMs,
			&ep.LastCheckError, &ep.ExpectedStatus, &ep.ExpectedKeyword, &ep.CertExpiresAt, &ep.LastCertWarningAt,
			&ep.Paused, &ep.PausedUntil, &ep.LastNotifiedAt, &ep.AlertMessageID, &ep.CreatedAt); err != nil {
			return nil, apperror.Wrap(apperror.ErrDatabase, err)
		}
		endpoints = append(endpoints, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.Wrap(apperror.ErrDatabase, err)
	}
	return endpoints, nil
}

func (s *SQLiteStore) UpdateEndpointStatus(ctx context.Context, id int64, o CheckOutcome) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE endpoints SET status = ?, last_checked_at = ?, last_status_code = ?, last_latency_ms = ?,
			last_check_error = ?, cert_expires_at = ? WHERE id = ?`,
		o.Status, now, o.StatusCode, o.LatencyMs, o.Reason, o.CertExpiry, id,
	)
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	if rows == 0 {
		return apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("endpoint %d not found", id))
	}
	return nil
}

func (s *SQLiteStore) UpdateEndpointInterval(ctx context.Context, id int64, intervalSeconds int) error {
	result, err := s.db.ExecContext(ctx,
		"UPDATE endpoints SET interval_seconds = ? WHERE id = ?",
		intervalSeconds, id,
	)
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	if rows == 0 {
		return apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("endpoint %d not found", id))
	}
	return nil
}

func (s *SQLiteStore) SetEndpointPaused(ctx context.Context, id int64, paused bool, until sql.NullTime) error {
	// SQLite compares DATETIMEs as strings — normalize to UTC so offsets never mix.
	if until.Valid {
		until.Time = until.Time.UTC()
	}
	result, err := s.db.ExecContext(ctx,
		"UPDATE endpoints SET paused = ?, paused_until = ? WHERE id = ?",
		paused, until, id,
	)
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	if rows == 0 {
		return apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("endpoint %d not found", id))
	}
	return nil
}

// ListExpiredPauses returns paused endpoints whose paused_until has passed.
func (s *SQLiteStore) ListExpiredPauses(ctx context.Context, now time.Time) ([]Endpoint, error) {
	now = now.UTC()
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, url, interval_seconds, status, last_checked_at, last_failure_at,
		        consecutive_failures, failure_notifications_sent, last_status_code, last_latency_ms,
		        last_check_error, expected_status, expected_keyword, cert_expires_at, last_cert_warning_at,
		        paused, paused_until, last_notified_at, alert_message_id, created_at
		 FROM endpoints WHERE paused = 1 AND paused_until IS NOT NULL AND paused_until <= ?`,
		now,
	)
	if err != nil {
		return nil, apperror.Wrap(apperror.ErrDatabase, err)
	}
	defer rows.Close()

	var endpoints []Endpoint
	for rows.Next() {
		var ep Endpoint
		if err := rows.Scan(&ep.ID, &ep.Name, &ep.URL, &ep.IntervalSeconds, &ep.Status,
			&ep.LastCheckedAt, &ep.LastFailureAt,
			&ep.ConsecutiveFailures, &ep.FailureNotificationsSent, &ep.LastStatusCode, &ep.LastLatencyMs,
			&ep.LastCheckError, &ep.ExpectedStatus, &ep.ExpectedKeyword, &ep.CertExpiresAt, &ep.LastCertWarningAt,
			&ep.Paused, &ep.PausedUntil, &ep.LastNotifiedAt, &ep.AlertMessageID, &ep.CreatedAt); err != nil {
			return nil, apperror.Wrap(apperror.ErrDatabase, err)
		}
		endpoints = append(endpoints, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.Wrap(apperror.ErrDatabase, err)
	}
	return endpoints, nil
}

// SetAlertMessageID stores the Telegram message ID of the latest failure
// alert, so the recovery notification can be sent as a reply to it.
func (s *SQLiteStore) SetAlertMessageID(ctx context.Context, id int64, messageID int64) error {
	result, err := s.db.ExecContext(ctx,
		"UPDATE endpoints SET alert_message_id = ? WHERE id = ?",
		messageID, id,
	)
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	if rows == 0 {
		return apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("endpoint %d not found", id))
	}
	return nil
}

// TouchLastNotified records that a (reminder) notification was just sent.
func (s *SQLiteStore) TouchLastNotified(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE endpoints SET last_notified_at = ? WHERE id = ?",
		time.Now().UTC(), id,
	)
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	return nil
}

func (s *SQLiteStore) RecordFailure(ctx context.Context, id int64, o CheckOutcome, maxNotifications int, failureThreshold int) (Endpoint, error) {
	now := time.Now().UTC()

	// Set last_failure_at only on first failure (when consecutive_failures was 0).
	// failure_notifications_sent only counts failures at or beyond the alert
	// threshold and is capped at maxNotifications, so it always reflects the
	// number of notifications actually sent; last_notified_at tracks when.
	_, err := s.db.ExecContext(ctx,
		`UPDATE endpoints SET
			status = 'not_ok',
			last_checked_at = ?,
			consecutive_failures = consecutive_failures + 1,
			failure_notifications_sent = CASE
				WHEN failure_notifications_sent < ? AND consecutive_failures + 1 >= ? THEN failure_notifications_sent + 1
				ELSE failure_notifications_sent
			END,
			last_notified_at = CASE
				WHEN failure_notifications_sent < ? AND consecutive_failures + 1 >= ? THEN ?
				ELSE last_notified_at
			END,
			last_failure_at = CASE WHEN consecutive_failures = 0 THEN ? ELSE last_failure_at END,
			last_status_code = ?,
			last_latency_ms = ?,
			last_check_error = ?,
			cert_expires_at = ?
		 WHERE id = ?`,
		now, maxNotifications, failureThreshold, maxNotifications, failureThreshold, now, now,
		o.StatusCode, o.LatencyMs, o.Reason, o.CertExpiry, id,
	)
	if err != nil {
		return Endpoint{}, apperror.Wrap(apperror.ErrDatabase, err)
	}

	return s.GetEndpoint(ctx, id)
}

func (s *SQLiteStore) RecordRecovery(ctx context.Context, id int64, o CheckOutcome) (Endpoint, error) {
	// First get the current endpoint to preserve last_failure_at for downtime calculation
	ep, err := s.GetEndpoint(ctx, id)
	if err != nil {
		return Endpoint{}, err
	}

	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx,
		`UPDATE endpoints SET
			status = 'ok',
			last_checked_at = ?,
			consecutive_failures = 0,
			failure_notifications_sent = 0,
			last_failure_at = NULL,
			last_notified_at = NULL,
			alert_message_id = 0,
			last_status_code = ?,
			last_latency_ms = ?,
			last_check_error = '',
			cert_expires_at = ?
		 WHERE id = ?`,
		now, o.StatusCode, o.LatencyMs, o.CertExpiry, id,
	)
	if err != nil {
		return Endpoint{}, apperror.Wrap(apperror.ErrDatabase, err)
	}

	// Return the endpoint with the old last_failure_at and alert_message_id so
	// the caller can compute downtime and thread the recovery to the alert.
	ep.Status = "ok"
	ep.LastCheckedAt = sql.NullTime{Time: now, Valid: true}
	ep.ConsecutiveFailures = 0
	ep.FailureNotificationsSent = 0
	ep.LastStatusCode = o.StatusCode
	ep.LastLatencyMs = o.LatencyMs
	ep.LastCheckError = ""
	ep.CertExpiresAt = o.CertExpiry
	return ep, nil
}

// SetExpectedStatus sets the exact HTTP status an endpoint must return (0 = any 2xx).
func (s *SQLiteStore) SetExpectedStatus(ctx context.Context, id int64, code int) error {
	return s.updateColumn(ctx, id, "expected_status", code)
}

// SetExpectedKeyword sets the substring the response body must contain ("" = disabled).
func (s *SQLiteStore) SetExpectedKeyword(ctx context.Context, id int64, keyword string) error {
	return s.updateColumn(ctx, id, "expected_keyword", keyword)
}

// RenameEndpoint changes an endpoint's name.
func (s *SQLiteStore) RenameEndpoint(ctx context.Context, id int64, newName string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE endpoints SET name = ? WHERE id = ?", newName, id)
	if err != nil {
		if isUniqueViolation(err) {
			return apperror.Wrap(apperror.ErrDuplicate, err)
		}
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	if rows == 0 {
		return apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("endpoint %d not found", id))
	}
	return nil
}

// TouchCertWarning records that a certificate-expiry warning was just sent.
func (s *SQLiteStore) TouchCertWarning(ctx context.Context, id int64) error {
	return s.updateColumn(ctx, id, "last_cert_warning_at", time.Now().UTC())
}

func (s *SQLiteStore) updateColumn(ctx context.Context, id int64, column string, value interface{}) error {
	// column is only ever a hardcoded identifier from this package — never user input.
	query := fmt.Sprintf("UPDATE endpoints SET %s = ? WHERE id = ?", column)
	result, err := s.db.ExecContext(ctx, query, value, id)
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	if rows == 0 {
		return apperror.Wrap(apperror.ErrNotFound, fmt.Errorf("endpoint %d not found", id))
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// RecordCheck appends one check result to the history table.
func (s *SQLiteStore) RecordCheck(ctx context.Context, endpointID int64, up bool, statusCode int, latencyMs int64) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO checks (endpoint_id, up, status_code, latency_ms, checked_at) VALUES (?, ?, ?, ?, ?)",
		endpointID, up, statusCode, latencyMs, time.Now().UTC(),
	)
	if err != nil {
		return apperror.Wrap(apperror.ErrDatabase, err)
	}
	return nil
}

// GetCheckStats aggregates the check history for an endpoint since a given time.
func (s *SQLiteStore) GetCheckStats(ctx context.Context, endpointID int64, since time.Time) (WindowStats, error) {
	// SQLite compares DATETIMEs as strings — normalize to UTC so offsets never mix.
	since = since.UTC()
	var stats WindowStats
	var avg sql.NullFloat64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(up), 0), AVG(latency_ms)
		 FROM checks WHERE endpoint_id = ? AND checked_at >= ?`,
		endpointID, since,
	).Scan(&stats.Total, &stats.Up, &avg)
	if err != nil {
		return WindowStats{}, apperror.Wrap(apperror.ErrDatabase, err)
	}
	stats.AvgLatencyMs = avg.Float64

	if stats.Total > 0 {
		// p95 via ordered offset — avoids loading all latencies into memory.
		err = s.db.QueryRowContext(ctx,
			`SELECT latency_ms FROM checks
			 WHERE endpoint_id = ? AND checked_at >= ?
			 ORDER BY latency_ms
			 LIMIT 1 OFFSET (SELECT (COUNT(*) * 95) / 100 FROM checks
			                 WHERE endpoint_id = ? AND checked_at >= ?)`,
			endpointID, since, endpointID, since,
		).Scan(&stats.P95LatencyMs)
		if err != nil {
			return WindowStats{}, apperror.Wrap(apperror.ErrDatabase, err)
		}

		// Incidents: transitions from up to down within the window.
		err = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM (
				SELECT up, LAG(up) OVER (ORDER BY checked_at, id) AS prev_up
				FROM checks WHERE endpoint_id = ? AND checked_at >= ?
			) WHERE up = 0 AND prev_up = 1`,
			endpointID, since,
		).Scan(&stats.Incidents)
		if err != nil {
			return WindowStats{}, apperror.Wrap(apperror.ErrDatabase, err)
		}
	}

	return stats, nil
}

// GetRecentTransitions returns the most recent up/down state flips for an
// endpoint, newest last. limit applies to the number of flips returned.
func (s *SQLiteStore) GetRecentTransitions(ctx context.Context, endpointID int64, limit int) ([]CheckTransition, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT checked_at, up, status_code FROM (
			SELECT checked_at, up, status_code,
			       LAG(up) OVER (ORDER BY checked_at, id) AS prev_up
			FROM checks WHERE endpoint_id = ?
		) WHERE prev_up IS NULL OR up != prev_up
		ORDER BY checked_at DESC LIMIT ?`,
		endpointID, limit,
	)
	if err != nil {
		return nil, apperror.Wrap(apperror.ErrDatabase, err)
	}
	defer rows.Close()

	var transitions []CheckTransition
	for rows.Next() {
		var t CheckTransition
		if err := rows.Scan(&t.CheckedAt, &t.Up, &t.StatusCode); err != nil {
			return nil, apperror.Wrap(apperror.ErrDatabase, err)
		}
		transitions = append(transitions, t)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.Wrap(apperror.ErrDatabase, err)
	}

	// Restore chronological order (query fetched newest first).
	for i, j := 0, len(transitions)-1; i < j; i, j = i+1, j-1 {
		transitions[i], transitions[j] = transitions[j], transitions[i]
	}
	return transitions, nil
}

// PruneChecks deletes check history older than the given time.
// Returns the number of deleted rows.
func (s *SQLiteStore) PruneChecks(ctx context.Context, olderThan time.Time) (int64, error) {
	olderThan = olderThan.UTC()
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM checks WHERE checked_at < ?", olderThan,
	)
	if err != nil {
		return 0, apperror.Wrap(apperror.ErrDatabase, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, apperror.Wrap(apperror.ErrDatabase, err)
	}
	return n, nil
}

package storage

import (
	"context"
	"database/sql"
	"embed"
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
		        consecutive_failures, failure_notifications_sent, created_at
		 FROM endpoints WHERE id = ?`, id,
	).Scan(&ep.ID, &ep.Name, &ep.URL, &ep.IntervalSeconds, &ep.Status,
		&ep.LastCheckedAt, &ep.LastFailureAt,
		&ep.ConsecutiveFailures, &ep.FailureNotificationsSent, &ep.CreatedAt)

	if err == sql.ErrNoRows {
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
		        consecutive_failures, failure_notifications_sent, created_at
		 FROM endpoints WHERE url = ?`, url,
	).Scan(&ep.ID, &ep.Name, &ep.URL, &ep.IntervalSeconds, &ep.Status,
		&ep.LastCheckedAt, &ep.LastFailureAt,
		&ep.ConsecutiveFailures, &ep.FailureNotificationsSent, &ep.CreatedAt)

	if err == sql.ErrNoRows {
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
		        consecutive_failures, failure_notifications_sent, created_at
		 FROM endpoints WHERE name = ?`, name,
	).Scan(&ep.ID, &ep.Name, &ep.URL, &ep.IntervalSeconds, &ep.Status,
		&ep.LastCheckedAt, &ep.LastFailureAt,
		&ep.ConsecutiveFailures, &ep.FailureNotificationsSent, &ep.CreatedAt)

	if err == sql.ErrNoRows {
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
		        consecutive_failures, failure_notifications_sent, created_at
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
			&ep.ConsecutiveFailures, &ep.FailureNotificationsSent, &ep.CreatedAt); err != nil {
			return nil, apperror.Wrap(apperror.ErrDatabase, err)
		}
		endpoints = append(endpoints, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.Wrap(apperror.ErrDatabase, err)
	}
	return endpoints, nil
}

func (s *SQLiteStore) UpdateEndpointStatus(ctx context.Context, id int64, status string, statusCode int) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		"UPDATE endpoints SET status = ?, last_checked_at = ? WHERE id = ?",
		status, now, id,
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

func (s *SQLiteStore) RecordFailure(ctx context.Context, id int64, statusCode int) (Endpoint, error) {
	now := time.Now().UTC()

	// Set last_failure_at only on first failure (when consecutive_failures was 0)
	_, err := s.db.ExecContext(ctx,
		`UPDATE endpoints SET
			status = 'not_ok',
			last_checked_at = ?,
			consecutive_failures = consecutive_failures + 1,
			failure_notifications_sent = failure_notifications_sent + 1,
			last_failure_at = CASE WHEN consecutive_failures = 0 THEN ? ELSE last_failure_at END
		 WHERE id = ?`,
		now, now, id,
	)
	if err != nil {
		return Endpoint{}, apperror.Wrap(apperror.ErrDatabase, err)
	}

	return s.GetEndpoint(ctx, id)
}

func (s *SQLiteStore) RecordRecovery(ctx context.Context, id int64, statusCode int) (Endpoint, error) {
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
			last_failure_at = NULL
		 WHERE id = ?`,
		now, id,
	)
	if err != nil {
		return Endpoint{}, apperror.Wrap(apperror.ErrDatabase, err)
	}

	// Return the endpoint with the old last_failure_at so caller can compute downtime
	ep.Status = "ok"
	ep.LastCheckedAt = sql.NullTime{Time: now, Valid: true}
	ep.ConsecutiveFailures = 0
	ep.FailureNotificationsSent = 0
	return ep, nil
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

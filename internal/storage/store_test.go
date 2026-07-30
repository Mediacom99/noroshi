package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"noroshi/internal/apperror"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func TestRunMigrations(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	row := db.QueryRow("SELECT count(*) FROM endpoints")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query endpoints table: %v", err)
	}
}

func TestRunMigrationsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("second: %v", err)
	}
}

func TestAddAndGetEndpoint(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, err := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)
	if err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	if ep.Name != "prod-api" {
		t.Errorf("Name = %q, want %q", ep.Name, "prod-api")
	}
	if ep.URL != "https://example.com" {
		t.Errorf("URL = %q, want %q", ep.URL, "https://example.com")
	}
	if ep.IntervalSeconds != 30 {
		t.Errorf("IntervalSeconds = %d, want %d", ep.IntervalSeconds, 30)
	}
	if ep.Status != "unknown" {
		t.Errorf("Status = %q, want %q", ep.Status, "unknown")
	}
	if ep.LastStatusCode != 0 {
		t.Errorf("LastStatusCode = %d, want 0", ep.LastStatusCode)
	}

	got, err := store.GetEndpoint(ctx, ep.ID)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if got.Name != ep.Name {
		t.Errorf("GetEndpoint Name = %q, want %q", got.Name, ep.Name)
	}
	if got.URL != ep.URL {
		t.Errorf("GetEndpoint URL = %q, want %q", got.URL, ep.URL)
	}
}

func TestAddEndpointDuplicateURL(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	_, err := store.AddEndpoint(ctx, "ep1", "https://example.com", 30)
	if err != nil {
		t.Fatalf("first AddEndpoint: %v", err)
	}

	_, err = store.AddEndpoint(ctx, "ep2", "https://example.com", 60)
	if !errors.Is(err, apperror.ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got: %v", err)
	}
}

func TestAddEndpointDuplicateName(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	_, err := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)
	if err != nil {
		t.Fatalf("first AddEndpoint: %v", err)
	}

	_, err = store.AddEndpoint(ctx, "prod-api", "https://other.com", 60)
	if !errors.Is(err, apperror.ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got: %v", err)
	}
}

func TestGetEndpointNotFound(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	_, err := store.GetEndpoint(ctx, 999)
	if !errors.Is(err, apperror.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestDeleteEndpoint(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, err := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)
	if err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}

	if err := store.DeleteEndpoint(ctx, ep.ID); err != nil {
		t.Fatalf("DeleteEndpoint: %v", err)
	}

	_, err = store.GetEndpoint(ctx, ep.ID)
	if !errors.Is(err, apperror.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestDeleteEndpointNotFound(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	err := store.DeleteEndpoint(ctx, 999)
	if !errors.Is(err, apperror.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestListEndpoints(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	_, _ = store.AddEndpoint(ctx, "site-a", "https://a.com", 30)
	_, _ = store.AddEndpoint(ctx, "site-b", "https://b.com", 60)

	eps, err := store.ListEndpoints(ctx)
	if err != nil {
		t.Fatalf("ListEndpoints: %v", err)
	}
	if len(eps) != 2 {
		t.Fatalf("len = %d, want 2", len(eps))
	}
	if eps[0].URL != "https://a.com" {
		t.Errorf("first URL = %q, want %q", eps[0].URL, "https://a.com")
	}
	if eps[1].URL != "https://b.com" {
		t.Errorf("second URL = %q, want %q", eps[1].URL, "https://b.com")
	}
}

func TestRecordFailure(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)

	// First failure
	updated, err := store.RecordFailure(ctx, ep.ID, 503, 12, 3, 1)
	if err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if updated.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", updated.ConsecutiveFailures)
	}
	if updated.FailureNotificationsSent != 1 {
		t.Errorf("FailureNotificationsSent = %d, want 1", updated.FailureNotificationsSent)
	}
	if updated.Status != "not_ok" {
		t.Errorf("Status = %q, want %q", updated.Status, "not_ok")
	}
	if !updated.LastFailureAt.Valid {
		t.Error("LastFailureAt should be set on first failure")
	}
	if updated.LastStatusCode != 503 {
		t.Errorf("LastStatusCode = %d, want 503", updated.LastStatusCode)
	}

	// Second failure
	updated2, err := store.RecordFailure(ctx, ep.ID, 503, 12, 3, 1)
	if err != nil {
		t.Fatalf("RecordFailure 2: %v", err)
	}
	if updated2.ConsecutiveFailures != 2 {
		t.Errorf("ConsecutiveFailures = %d, want 2", updated2.ConsecutiveFailures)
	}
	// LastFailureAt should remain the same as first failure
	if updated2.LastFailureAt.Time != updated.LastFailureAt.Time {
		t.Error("LastFailureAt should not change on subsequent failures")
	}
}

func TestRecordFailureNotificationCap(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)

	maxNotifications := 3
	for range maxNotifications + 2 {
		if _, err := store.RecordFailure(ctx, ep.ID, 503, 12, maxNotifications, 1); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}

	updated, err := store.GetEndpoint(ctx, ep.ID)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if updated.FailureNotificationsSent != maxNotifications {
		t.Errorf("FailureNotificationsSent = %d, want capped at %d", updated.FailureNotificationsSent, maxNotifications)
	}
	if updated.ConsecutiveFailures != maxNotifications+2 {
		t.Errorf("ConsecutiveFailures = %d, want %d (uncapped)", updated.ConsecutiveFailures, maxNotifications+2)
	}
}

func TestRecordRecovery(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)
	if _, err := store.RecordFailure(ctx, ep.ID, 503, 12, 3, 1); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if _, err := store.RecordFailure(ctx, ep.ID, 503, 12, 3, 1); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	recovered, err := store.RecordRecovery(ctx, ep.ID, 200, 12)
	if err != nil {
		t.Fatalf("RecordRecovery: %v", err)
	}

	if recovered.Status != "ok" {
		t.Errorf("Status = %q, want %q", recovered.Status, "ok")
	}
	if recovered.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", recovered.ConsecutiveFailures)
	}
	if recovered.FailureNotificationsSent != 0 {
		t.Errorf("FailureNotificationsSent = %d, want 0", recovered.FailureNotificationsSent)
	}
	if recovered.LastStatusCode != 200 {
		t.Errorf("LastStatusCode = %d, want 200", recovered.LastStatusCode)
	}
	// Should still have the old LastFailureAt for downtime calculation
	if !recovered.LastFailureAt.Valid {
		t.Error("LastFailureAt should be preserved in return value for downtime calculation")
	}

	// Verify DB is actually reset
	fromDB, err := store.GetEndpoint(ctx, ep.ID)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if fromDB.LastFailureAt.Valid {
		t.Error("LastFailureAt should be NULL in DB after recovery")
	}
}

func TestUpdateEndpointInterval(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)

	if err := store.UpdateEndpointInterval(ctx, ep.ID, 60); err != nil {
		t.Fatalf("UpdateEndpointInterval: %v", err)
	}

	updated, err := store.GetEndpoint(ctx, ep.ID)
	if err != nil {
		t.Fatalf("GetEndpoint: %v", err)
	}
	if updated.IntervalSeconds != 60 {
		t.Errorf("IntervalSeconds = %d, want 60", updated.IntervalSeconds)
	}
}

func TestGetEndpointByURL(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	added, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)

	got, err := store.GetEndpointByURL(ctx, "https://example.com")
	if err != nil {
		t.Fatalf("GetEndpointByURL: %v", err)
	}
	if got.ID != added.ID {
		t.Errorf("ID = %d, want %d", got.ID, added.ID)
	}
}

func TestGetEndpointByURLNotFound(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	_, err := store.GetEndpointByURL(ctx, "https://nonexistent.com")
	if !errors.Is(err, apperror.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetEndpointByName(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	added, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)

	got, err := store.GetEndpointByName(ctx, "prod-api")
	if err != nil {
		t.Fatalf("GetEndpointByName: %v", err)
	}
	if got.ID != added.ID {
		t.Errorf("ID = %d, want %d", got.ID, added.ID)
	}
	if got.Name != "prod-api" {
		t.Errorf("Name = %q, want %q", got.Name, "prod-api")
	}
}

func TestGetEndpointByNameNotFound(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	_, err := store.GetEndpointByName(ctx, "nonexistent")
	if !errors.Is(err, apperror.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestUpdateEndpointStatus(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)

	if err := store.UpdateEndpointStatus(ctx, ep.ID, "ok", 200, 12); err != nil {
		t.Fatalf("UpdateEndpointStatus: %v", err)
	}

	updated, _ := store.GetEndpoint(ctx, ep.ID)
	if updated.Status != "ok" {
		t.Errorf("Status = %q, want %q", updated.Status, "ok")
	}
	if !updated.LastCheckedAt.Valid {
		t.Error("LastCheckedAt should be set after status update")
	}
	if updated.LastStatusCode != 200 {
		t.Errorf("LastStatusCode = %d, want 200", updated.LastStatusCode)
	}
}

func TestRecordFailureThreshold(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)

	// Threshold 3: first two failures must not count as notifications.
	for i := range 4 {
		updated, err := store.RecordFailure(ctx, ep.ID, 503, 12, 3, 3)
		if err != nil {
			t.Fatalf("RecordFailure %d: %v", i, err)
		}
		want := i - 1 // failures 3 and 4 → 1 and 2 notifications
		if want < 0 {
			want = 0
		}
		if updated.FailureNotificationsSent != want {
			t.Errorf("failure %d: FailureNotificationsSent = %d, want %d", i+1, updated.FailureNotificationsSent, want)
		}
	}
}

func TestSetEndpointPaused(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)
	if ep.Paused {
		t.Error("new endpoint should not be paused")
	}

	if err := store.SetEndpointPaused(ctx, ep.ID, true, sql.NullTime{}); err != nil {
		t.Fatalf("SetEndpointPaused: %v", err)
	}
	updated, _ := store.GetEndpoint(ctx, ep.ID)
	if !updated.Paused {
		t.Error("endpoint should be paused")
	}

	if err := store.SetEndpointPaused(ctx, ep.ID, false, sql.NullTime{}); err != nil {
		t.Fatalf("SetEndpointPaused: %v", err)
	}
	updated, _ = store.GetEndpoint(ctx, ep.ID)
	if updated.Paused {
		t.Error("endpoint should not be paused")
	}

	if err := store.SetEndpointPaused(ctx, 999, true, sql.NullTime{}); !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRecordFailureSetsLastNotified(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)

	// Below threshold (2): no notification, no last_notified_at.
	updated, _ := store.RecordFailure(ctx, ep.ID, 503, 12, 3, 2)
	if updated.LastNotifiedAt.Valid {
		t.Error("LastNotifiedAt should not be set below the alert threshold")
	}

	// Threshold reached: notified.
	updated, _ = store.RecordFailure(ctx, ep.ID, 503, 12, 3, 2)
	if !updated.LastNotifiedAt.Valid {
		t.Error("LastNotifiedAt should be set when a notification is counted")
	}

	// Recovery clears it.
	recovered, err := store.RecordRecovery(ctx, ep.ID, 200, 12)
	if err != nil {
		t.Fatalf("RecordRecovery: %v", err)
	}
	after, _ := store.GetEndpoint(ctx, ep.ID)
	if after.LastNotifiedAt.Valid {
		t.Error("LastNotifiedAt should be cleared on recovery")
	}
	if recovered.AlertMessageID != 0 && after.AlertMessageID != 0 {
		t.Error("AlertMessageID should be cleared on recovery")
	}
}

func TestAlertMessageID(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)

	if err := store.SetAlertMessageID(ctx, ep.ID, 4242); err != nil {
		t.Fatalf("SetAlertMessageID: %v", err)
	}
	updated, _ := store.GetEndpoint(ctx, ep.ID)
	if updated.AlertMessageID != 4242 {
		t.Errorf("AlertMessageID = %d, want 4242", updated.AlertMessageID)
	}

	if err := store.SetAlertMessageID(ctx, 999, 1); !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListExpiredPauses(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep1, _ := store.AddEndpoint(ctx, "expired", "https://a.com", 30)
	ep2, _ := store.AddEndpoint(ctx, "future", "https://b.com", 30)
	ep3, _ := store.AddEndpoint(ctx, "indefinite", "https://c.com", 30)

	past := sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true}
	future := sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true}
	store.SetEndpointPaused(ctx, ep1.ID, true, past)
	store.SetEndpointPaused(ctx, ep2.ID, true, future)
	store.SetEndpointPaused(ctx, ep3.ID, true, sql.NullTime{})

	expired, err := store.ListExpiredPauses(ctx, time.Now())
	if err != nil {
		t.Fatalf("ListExpiredPauses: %v", err)
	}
	if len(expired) != 1 || expired[0].ID != ep1.ID {
		t.Errorf("expected only the expired pause, got %+v", expired)
	}
}

func TestTouchLastNotified(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)
	if err := store.TouchLastNotified(ctx, ep.ID); err != nil {
		t.Fatalf("TouchLastNotified: %v", err)
	}
	updated, _ := store.GetEndpoint(ctx, ep.ID)
	if !updated.LastNotifiedAt.Valid {
		t.Error("LastNotifiedAt should be set")
	}
}

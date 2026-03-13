package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"

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
	updated, err := store.RecordFailure(ctx, ep.ID, 503)
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

	// Second failure
	updated2, err := store.RecordFailure(ctx, ep.ID, 503)
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

func TestRecordRecovery(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)
	store.RecordFailure(ctx, ep.ID, 503)
	store.RecordFailure(ctx, ep.ID, 503)

	recovered, err := store.RecordRecovery(ctx, ep.ID, 200)
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

	if err := store.UpdateEndpointStatus(ctx, ep.ID, "ok", 200); err != nil {
		t.Fatalf("UpdateEndpointStatus: %v", err)
	}

	updated, _ := store.GetEndpoint(ctx, ep.ID)
	if updated.Status != "ok" {
		t.Errorf("Status = %q, want %q", updated.Status, "ok")
	}
	if !updated.LastCheckedAt.Valid {
		t.Error("LastCheckedAt should be set after status update")
	}
}

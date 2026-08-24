package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
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
	updated, err := store.RecordFailure(ctx, ep.ID, CheckOutcome{Status: "not_ok", StatusCode: 503, LatencyMs: 12, Reason: "HTTP 503"}, 3, 1)
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
	updated2, err := store.RecordFailure(ctx, ep.ID, CheckOutcome{Status: "not_ok", StatusCode: 503, LatencyMs: 12, Reason: "HTTP 503"}, 3, 1)
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
		if _, err := store.RecordFailure(ctx, ep.ID, CheckOutcome{Status: "not_ok", StatusCode: 503, LatencyMs: 12, Reason: "HTTP 503"}, maxNotifications, 1); err != nil {
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
	if _, err := store.RecordFailure(ctx, ep.ID, CheckOutcome{Status: "not_ok", StatusCode: 503, LatencyMs: 12, Reason: "HTTP 503"}, 3, 1); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if _, err := store.RecordFailure(ctx, ep.ID, CheckOutcome{Status: "not_ok", StatusCode: 503, LatencyMs: 12, Reason: "HTTP 503"}, 3, 1); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	recovered, err := store.RecordRecovery(ctx, ep.ID, CheckOutcome{Status: "ok", StatusCode: 200, LatencyMs: 12})
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

	if err := store.UpdateEndpointStatus(ctx, ep.ID, CheckOutcome{Status: "ok", StatusCode: 200, LatencyMs: 12}); err != nil {
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
		updated, err := store.RecordFailure(ctx, ep.ID, CheckOutcome{Status: "not_ok", StatusCode: 503, LatencyMs: 12, Reason: "HTTP 503"}, 3, 3)
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
	updated, _ := store.RecordFailure(ctx, ep.ID, CheckOutcome{Status: "not_ok", StatusCode: 503, LatencyMs: 12, Reason: "HTTP 503"}, 3, 2)
	if updated.LastNotifiedAt.Valid {
		t.Error("LastNotifiedAt should not be set below the alert threshold")
	}

	// Threshold reached: notified.
	updated, _ = store.RecordFailure(ctx, ep.ID, CheckOutcome{Status: "not_ok", StatusCode: 503, LatencyMs: 12, Reason: "HTTP 503"}, 3, 2)
	if !updated.LastNotifiedAt.Valid {
		t.Error("LastNotifiedAt should be set when a notification is counted")
	}

	// Recovery clears it.
	recovered, err := store.RecordRecovery(ctx, ep.ID, CheckOutcome{Status: "ok", StatusCode: 200, LatencyMs: 12})
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

	past := sql.NullTime{Time: time.Now().Add(-time.Minute), Valid: true} // local tz: store must normalize
	future := sql.NullTime{Time: time.Now().UTC().Add(time.Hour), Valid: true}
	store.SetEndpointPaused(ctx, ep1.ID, true, past)
	store.SetEndpointPaused(ctx, ep2.ID, true, future)
	store.SetEndpointPaused(ctx, ep3.ID, true, sql.NullTime{})

	expired, err := store.ListExpiredPauses(ctx, time.Now().UTC())
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

func TestCheckOptionsAndRename(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)
	store.AddEndpoint(ctx, "other", "https://other.com", 30)

	if err := store.SetExpectedStatus(ctx, ep.ID, 200); err != nil {
		t.Fatalf("SetExpectedStatus: %v", err)
	}
	if err := store.SetExpectedKeyword(ctx, ep.ID, "ok"); err != nil {
		t.Fatalf("SetExpectedKeyword: %v", err)
	}
	updated, _ := store.GetEndpoint(ctx, ep.ID)
	if updated.ExpectedStatus != 200 || updated.ExpectedKeyword != "ok" {
		t.Errorf("got status=%d keyword=%q", updated.ExpectedStatus, updated.ExpectedKeyword)
	}

	if err := store.RenameEndpoint(ctx, ep.ID, "renamed"); err != nil {
		t.Fatalf("RenameEndpoint: %v", err)
	}
	updated, _ = store.GetEndpoint(ctx, ep.ID)
	if updated.Name != "renamed" {
		t.Errorf("Name = %q, want renamed", updated.Name)
	}

	// Duplicate name → ErrDuplicate
	if err := store.RenameEndpoint(ctx, ep.ID, "other"); !errors.Is(err, apperror.ErrDuplicate) {
		t.Errorf("expected ErrDuplicate, got %v", err)
	}

	// Not found
	if err := store.RenameEndpoint(ctx, 999, "x"); !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestLastCheckErrorPersisted(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)

	updated, err := store.RecordFailure(ctx, ep.ID,
		CheckOutcome{Status: "not_ok", StatusCode: 200, LatencyMs: 30, Reason: `keyword "ok" not found`}, 3, 1)
	if err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if updated.LastCheckError != `keyword "ok" not found` {
		t.Errorf("LastCheckError = %q", updated.LastCheckError)
	}

	recovered, err := store.RecordRecovery(ctx, ep.ID, CheckOutcome{Status: "ok", StatusCode: 200, LatencyMs: 25})
	if err != nil {
		t.Fatalf("RecordRecovery: %v", err)
	}
	if recovered.LastCheckError != "" {
		t.Errorf("LastCheckError should be cleared on recovery, got %q", recovered.LastCheckError)
	}
}

func TestCheckStatsAndPrune(t *testing.T) {
	db := testDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	ep, _ := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)

	// 10 checks: 8 up, 2 down (one incident: up→down), latencies 10..100.
	for i := 1; i <= 10; i++ {
		up := true
		if i == 5 || i == 6 {
			up = false
		}
		if err := store.RecordCheck(ctx, ep.ID, up, 200, int64(i*10)); err != nil {
			t.Fatalf("RecordCheck: %v", err)
		}
	}

	stats, err := store.GetCheckStats(ctx, ep.ID, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetCheckStats: %v", err)
	}
	if stats.Total != 10 || stats.Up != 8 {
		t.Errorf("Total=%d Up=%d, want 10/8", stats.Total, stats.Up)
	}
	if stats.Uptime() != 80 {
		t.Errorf("Uptime = %.1f, want 80", stats.Uptime())
	}
	if stats.Incidents != 1 {
		t.Errorf("Incidents = %d, want 1", stats.Incidents)
	}
	if stats.AvgLatencyMs != 55 {
		t.Errorf("AvgLatencyMs = %.1f, want 55", stats.AvgLatencyMs)
	}
	if stats.P95LatencyMs < 90 {
		t.Errorf("P95LatencyMs = %d, want >= 90", stats.P95LatencyMs)
	}

	// Window excluding all checks.
	stats, err = store.GetCheckStats(ctx, ep.ID, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("GetCheckStats empty: %v", err)
	}
	if stats.Total != 0 {
		t.Errorf("Total = %d, want 0", stats.Total)
	}

	// Transitions: first-up, down at i=5, up at i=7 → 3 flips.
	transitions, err := store.GetRecentTransitions(ctx, ep.ID, 10)
	if err != nil {
		t.Fatalf("GetRecentTransitions: %v", err)
	}
	if len(transitions) != 3 {
		t.Fatalf("transitions = %d, want 3", len(transitions))
	}
	if !transitions[0].Up || transitions[1].Up || !transitions[2].Up {
		t.Errorf("unexpected transition sequence: %+v", transitions)
	}

	// Prune: nothing older than the future, everything older than the past.
	deleted, err := store.PruneChecks(ctx, time.Now().Add(-time.Hour)) // non-UTC on purpose: store must normalize
	if err != nil {
		t.Fatalf("PruneChecks: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
	deleted, err = store.PruneChecks(ctx, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("PruneChecks: %v", err)
	}
	if deleted != 10 {
		t.Errorf("deleted = %d, want 10", deleted)
	}
}

func TestOpenDBConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	db, err := OpenDB(path)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// WAL mode must actually be active (the DSN pragma must not be ignored).
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}

	store := NewSQLiteStore(db)
	ctx := context.Background()
	ep, err := store.AddEndpoint(ctx, "prod-api", "https://example.com", 30)
	if err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}

	// Simulate many concurrent gocron jobs writing at once — this is what
	// produced SQLITE_BUSY in production.
	var wg sync.WaitGroup
	errs := make(chan error, 400)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				if err := store.RecordCheck(ctx, ep.ID, true, 200, 10); err != nil {
					errs <- err
				}
				if err := store.UpdateEndpointStatus(ctx, ep.ID,
					CheckOutcome{Status: "ok", StatusCode: 200, LatencyMs: 10}); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent write failed: %v", err)
	}
}

func TestMaintenanceWindows(t *testing.T) {
	db := testDB(t)
	s := NewSQLiteStore(db)
	ctx := context.Background()

	ep, err := s.AddEndpoint(ctx, "api", "https://example.com", 60)
	if err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	other, err := s.AddEndpoint(ctx, "web", "https://example.org", 60)
	if err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}

	// Per-endpoint window: 02:00–04:00 daily for ep.
	w1, err := s.AddMaintenanceWindow(ctx, sql.NullInt64{Int64: ep.ID, Valid: true}, "all", 120, 240)
	if err != nil {
		t.Fatalf("AddMaintenanceWindow: %v", err)
	}
	if w1.ID == 0 || w1.Days != "all" || w1.StartMinutes != 120 || w1.EndMinutes != 240 {
		t.Errorf("unexpected window: %+v", w1)
	}

	inside := time.Date(2026, 8, 21, 3, 0, 0, 0, time.UTC)
	outside := time.Date(2026, 8, 21, 5, 0, 0, 0, time.UTC)

	inMaint, err := s.IsInMaintenance(ctx, ep.ID, inside)
	if err != nil || !inMaint {
		t.Errorf("IsInMaintenance(ep, 03:00) = %v, %v; want true", inMaint, err)
	}
	inMaint, err = s.IsInMaintenance(ctx, ep.ID, outside)
	if err != nil || inMaint {
		t.Errorf("IsInMaintenance(ep, 05:00) = %v, %v; want false", inMaint, err)
	}
	inMaint, err = s.IsInMaintenance(ctx, other.ID, inside)
	if err != nil || inMaint {
		t.Errorf("IsInMaintenance(other, 03:00) = %v, %v; want false (per-endpoint window)", inMaint, err)
	}

	// Global window applies to every endpoint.
	if _, err := s.AddMaintenanceWindow(ctx, sql.NullInt64{}, "all", 0, 1439); err != nil {
		t.Fatalf("AddMaintenanceWindow global: %v", err)
	}
	inMaint, err = s.IsInMaintenance(ctx, other.ID, outside)
	if err != nil || !inMaint {
		t.Errorf("IsInMaintenance(other) with global window = %v, %v; want true", inMaint, err)
	}

	// List: both windows, global first.
	windows, err := s.ListMaintenanceWindows(ctx)
	if err != nil {
		t.Fatalf("ListMaintenanceWindows: %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("ListMaintenanceWindows returned %d, want 2", len(windows))
	}
	if windows[0].EndpointID.Valid {
		t.Errorf("first window should be the global one (endpoint_id NULL)")
	}

	// Delete.
	if err := s.DeleteMaintenanceWindow(ctx, w1.ID); err != nil {
		t.Fatalf("DeleteMaintenanceWindow: %v", err)
	}
	if err := s.DeleteMaintenanceWindow(ctx, w1.ID); !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("second delete: got %v, want ErrNotFound", err)
	}

	// Deleting an endpoint removes its windows (FK cascade is not enforced).
	if _, err := s.AddMaintenanceWindow(ctx, sql.NullInt64{Int64: ep.ID, Valid: true}, "all", 0, 1439); err != nil {
		t.Fatalf("AddMaintenanceWindow: %v", err)
	}
	if err := s.DeleteEndpoint(ctx, ep.ID); err != nil {
		t.Fatalf("DeleteEndpoint: %v", err)
	}
	windows, err = s.ListMaintenanceWindows(ctx)
	if err != nil {
		t.Fatalf("ListMaintenanceWindows: %v", err)
	}
	for _, w := range windows {
		if w.EndpointID.Valid && w.EndpointID.Int64 == ep.ID {
			t.Errorf("window for deleted endpoint should be gone: %+v", w)
		}
	}
}

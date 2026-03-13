package storage

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunMigrations(t *testing.T) {
	db := testDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Verify endpoints table exists
	row := db.QueryRow("SELECT count(*) FROM endpoints")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query endpoints table: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows, got %d", count)
	}
}

func TestRunMigrationsIdempotent(t *testing.T) {
	db := testDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("first RunMigrations: %v", err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("second RunMigrations should be idempotent: %v", err)
	}
}

func TestMigrationsCreateTable(t *testing.T) {
	db := testDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Check the table schema by inserting a row
	_, err := db.Exec(
		"INSERT INTO endpoints (url, interval_seconds) VALUES (?, ?)",
		"https://example.com", 30,
	)
	if err != nil {
		t.Fatalf("insert into endpoints: %v", err)
	}

	var url string
	var intervalSec int
	var status string
	err = db.QueryRow("SELECT url, interval_seconds, status FROM endpoints WHERE id = 1").
		Scan(&url, &intervalSec, &status)
	if err != nil {
		t.Fatalf("select from endpoints: %v", err)
	}
	if url != "https://example.com" {
		t.Errorf("url = %q, want %q", url, "https://example.com")
	}
	if intervalSec != 30 {
		t.Errorf("interval_seconds = %d, want %d", intervalSec, 30)
	}
	if status != "unknown" {
		t.Errorf("status = %q, want %q", status, "unknown")
	}
}

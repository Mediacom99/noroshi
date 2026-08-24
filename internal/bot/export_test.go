package bot

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"noroshi/internal/storage"

	tele "gopkg.in/telebot.v4"
)

func TestBuildExport(t *testing.T) {
	now := time.Date(2026, 8, 22, 10, 30, 0, 0, time.UTC)
	endpoints := []storage.Endpoint{
		{ID: 1, Name: "prod-api", URL: "https://example.com", IntervalSeconds: 60,
			ExpectedStatus: 200, ExpectedKeyword: "!error", Status: "ok"},
		{ID: 2, Name: "db", URL: "tcp://db:5432", IntervalSeconds: 30, Paused: true, Status: "unknown"},
	}
	windows := []storage.MaintenanceWindow{
		{ID: 1, Days: "all", StartMinutes: 60, EndMinutes: 120},
		{ID: 2, EndpointID: sql.NullInt64{Int64: 1, Valid: true}, Days: "sat,sun", StartMinutes: 1320, EndMinutes: 240},
		{ID: 3, EndpointID: sql.NullInt64{Int64: 99, Valid: true}, Days: "mon", StartMinutes: 0, EndMinutes: 60},
	}

	data, err := buildExport(endpoints, windows, now)
	if err != nil {
		t.Fatalf("buildExport: %v", err)
	}

	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("export is not valid JSON: %v", err)
	}

	if f.Version != 1 || !f.ExportedAt.Equal(now) {
		t.Errorf("header: version=%d exported_at=%v", f.Version, f.ExportedAt)
	}
	if len(f.Endpoints) != 2 {
		t.Fatalf("endpoints = %d, want 2", len(f.Endpoints))
	}
	ep := f.Endpoints[0]
	if ep.Name != "prod-api" || ep.URL != "https://example.com" || ep.IntervalSeconds != 60 ||
		ep.ExpectedStatus != 200 || ep.ExpectedKeyword != "!error" || ep.Paused || ep.Status != "ok" {
		t.Errorf("endpoint mismatch: %+v", ep)
	}
	if !f.Endpoints[1].Paused {
		t.Error("paused flag should be exported")
	}

	if len(f.MaintenanceWindows) != 3 {
		t.Fatalf("windows = %d, want 3", len(f.MaintenanceWindows))
	}
	if w := f.MaintenanceWindows[0]; w.Endpoint != "all" || w.Start != "01:00" || w.End != "02:00" {
		t.Errorf("global window mismatch: %+v", w)
	}
	if w := f.MaintenanceWindows[1]; w.Endpoint != "prod-api" || w.Start != "22:00" || w.End != "04:00" {
		t.Errorf("named window should use the endpoint name: %+v", w)
	}
	if w := f.MaintenanceWindows[2]; w.Endpoint != "id:99" {
		t.Errorf("window for unknown endpoint should fall back to id: %+v", w)
	}
}

func TestHandleExport(t *testing.T) {
	store := &mockStore{
		listEndpointsFn: func(_ context.Context) ([]storage.Endpoint, error) {
			return []storage.Endpoint{{ID: 1, Name: "prod-api", URL: "https://example.com", IntervalSeconds: 60, Status: "ok"}}, nil
		},
		listMaintenanceWindowsFn: func(_ context.Context) ([]storage.MaintenanceWindow, error) {
			return []storage.MaintenanceWindow{{ID: 1, Days: "all", StartMinutes: 60, EndMinutes: 120}}, nil
		},
	}
	b := newTestBot(store, &mockScheduler{})
	mc := &mockContext{messageFn: func() *tele.Message { return &tele.Message{} }}

	if err := b.handleExport(mc); err != nil {
		t.Fatalf("handleExport: %v", err)
	}
	if len(mc.sentDocuments) != 1 {
		t.Fatalf("sent %d documents, want 1", len(mc.sentDocuments))
	}
	doc := mc.sentDocuments[0]
	if !strings.HasPrefix(doc.FileName, "noroshi-export-") || !strings.HasSuffix(doc.FileName, ".json") {
		t.Errorf("FileName = %q", doc.FileName)
	}
	if doc.MIME != "application/json" {
		t.Errorf("MIME = %q", doc.MIME)
	}
	if !strings.Contains(doc.Caption, "1 endpoints") {
		t.Errorf("Caption = %q", doc.Caption)
	}
}

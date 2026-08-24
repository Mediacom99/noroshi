package bot

import (
	"encoding/json"
	"fmt"
	"time"

	"noroshi/internal/storage"
)

// exportFile is the top-level JSON structure sent by /export.
type exportFile struct {
	Version            int                    `json:"version"`
	ExportedAt         time.Time              `json:"exported_at"`
	Endpoints          []exportEndpoint       `json:"endpoints"`
	MaintenanceWindows []exportMaintWindow    `json:"maintenance_windows"`
}

type exportEndpoint struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
	ExpectedStatus  int    `json:"expected_status,omitempty"`
	ExpectedKeyword string `json:"expected_keyword,omitempty"`
	Paused          bool   `json:"paused"`
	// Informational runtime state at export time (not needed for a restore).
	Status string `json:"status"`
}

type exportMaintWindow struct {
	Endpoint string `json:"endpoint"` // endpoint name, or "all"
	Days     string `json:"days"`
	Start    string `json:"start"` // HH:MM UTC
	End      string `json:"end"`   // HH:MM UTC
}

// buildExport serializes the full monitor configuration as pretty-printed
// JSON. Check history is deliberately excluded — it is statistics, not
// configuration. Maintenance windows reference endpoints by name (IDs are
// not stable across a restore); global windows use "all".
func buildExport(endpoints []storage.Endpoint, windows []storage.MaintenanceWindow, now time.Time) ([]byte, error) {
	f := exportFile{
		Version:            1,
		ExportedAt:         now.UTC(),
		Endpoints:          make([]exportEndpoint, 0, len(endpoints)),
		MaintenanceWindows: make([]exportMaintWindow, 0, len(windows)),
	}
	names := make(map[int64]string, len(endpoints))
	for _, ep := range endpoints {
		names[ep.ID] = ep.Name
		f.Endpoints = append(f.Endpoints, exportEndpoint{
			Name:            ep.Name,
			URL:             ep.URL,
			IntervalSeconds: ep.IntervalSeconds,
			ExpectedStatus:  ep.ExpectedStatus,
			ExpectedKeyword: ep.ExpectedKeyword,
			Paused:          ep.Paused,
			Status:          ep.Status,
		})
	}
	for _, w := range windows {
		target := "all"
		if w.EndpointID.Valid {
			if name, ok := names[w.EndpointID.Int64]; ok {
				target = name
			} else {
				target = fmt.Sprintf("id:%d", w.EndpointID.Int64)
			}
		}
		f.MaintenanceWindows = append(f.MaintenanceWindows, exportMaintWindow{
			Endpoint: target,
			Days:     w.Days,
			Start:    formatMaintTime(w.StartMinutes),
			End:      formatMaintTime(w.EndMinutes),
		})
	}
	return json.MarshalIndent(f, "", "  ")
}

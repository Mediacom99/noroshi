package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"noroshi/internal/apperror"
	"noroshi/internal/storage"
)

// endpointJSON is the dashboard's view of a monitored endpoint.
type endpointJSON struct {
	ID                  int64      `json:"id"`
	Name                string     `json:"name"`
	URL                 string     `json:"url"`
	Type                string     `json:"type"` // URL scheme: http, https, tcp, dns, ping
	IntervalSeconds     int        `json:"interval_seconds"`
	Status              string     `json:"status"` // unknown | ok | not_ok
	Paused              bool       `json:"paused"`
	LastStatusCode      int        `json:"last_status_code"`
	LastLatencyMs       int64      `json:"last_latency_ms"`
	LastCheckError      string     `json:"last_check_error"`
	LastCheckedAt       *time.Time `json:"last_checked_at"`
	LastFailureAt       *time.Time `json:"last_failure_at"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	ExpectedStatus      int        `json:"expected_status"`  // 0 = any 2xx
	ExpectedKeyword     string     `json:"expected_keyword"` // "" = no body check
	CertExpiresAt       *time.Time `json:"cert_expires_at"`
	PausedUntil         *time.Time `json:"paused_until"`
	CreatedAt           time.Time  `json:"created_at"`
}

func toEndpointJSON(ep storage.Endpoint) endpointJSON {
	scheme, _, _ := strings.Cut(ep.URL, "://")
	return endpointJSON{
		ID:                  ep.ID,
		Name:                ep.Name,
		URL:                 ep.URL,
		Type:                scheme,
		IntervalSeconds:     ep.IntervalSeconds,
		Status:              ep.Status,
		Paused:              ep.Paused,
		LastStatusCode:      ep.LastStatusCode,
		LastLatencyMs:       ep.LastLatencyMs,
		LastCheckError:      ep.LastCheckError,
		LastCheckedAt:       timePtr(ep.LastCheckedAt),
		LastFailureAt:       timePtr(ep.LastFailureAt),
		ConsecutiveFailures: ep.ConsecutiveFailures,
		ExpectedStatus:      ep.ExpectedStatus,
		ExpectedKeyword:     ep.ExpectedKeyword,
		CertExpiresAt:       timePtr(ep.CertExpiresAt),
		PausedUntil:         timePtr(ep.PausedUntil),
		CreatedAt:           ep.CreatedAt,
	}
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

// statsJSON aggregates the check history over one time window.
type statsJSON struct {
	Total        int     `json:"total"`
	Up           int     `json:"up"`
	Uptime       float64 `json:"uptime"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P95LatencyMs int64   `json:"p95_latency_ms"`
	Incidents    int     `json:"incidents"`
}

func toStatsJSON(st storage.WindowStats) statsJSON {
	return statsJSON{
		Total:        st.Total,
		Up:           st.Up,
		Uptime:       st.Uptime(),
		AvgLatencyMs: st.AvgLatencyMs,
		P95LatencyMs: st.P95LatencyMs,
		Incidents:    st.Incidents,
	}
}

// incidentJSON is one outage: a down period with its duration (0 = ongoing).
type incidentJSON struct {
	Start           time.Time `json:"start"`
	DurationSeconds int64     `json:"duration_seconds"`
	StatusCode      int       `json:"status_code"` // 0 = connection error
}

// toIncidents pairs down-flips with the following up-flip, newest first,
// capped at 5 — the same composition the bot's /incidents uses.
func toIncidents(transitions []storage.CheckTransition) []incidentJSON {
	incidents := make([]incidentJSON, 0, len(transitions))
	for i, t := range transitions {
		if t.Up {
			continue
		}
		inc := incidentJSON{Start: t.CheckedAt.UTC(), StatusCode: t.StatusCode}
		if i+1 < len(transitions) {
			inc.DurationSeconds = int64(transitions[i+1].CheckedAt.Sub(t.CheckedAt).Seconds())
		}
		incidents = append(incidents, inc)
	}
	// Newest first, cap at 5.
	for i, j := 0, len(incidents)-1; i < j; i, j = i+1, j-1 {
		incidents[i], incidents[j] = incidents[j], incidents[i]
	}
	if len(incidents) > 5 {
		incidents = incidents[:5]
	}
	return incidents
}

// checkJSON is a single recorded health check.
type checkJSON struct {
	CheckedAt  time.Time `json:"checked_at"`
	Up         bool      `json:"up"`
	StatusCode int       `json:"status_code"`
	LatencyMs  int64     `json:"latency_ms"`
}

// maintenanceJSON is a recurring maintenance window as the dashboard sees it.
// EndpointID null means the window applies to all endpoints.
type maintenanceJSON struct {
	ID           int64  `json:"id"`
	EndpointID   *int64 `json:"endpoint_id"`
	Days         string `json:"days"` // "all" or comma day codes: mon,tue,...
	StartMinutes int    `json:"start_minutes"`
	EndMinutes   int    `json:"end_minutes"`
	Active       bool   `json:"active"` // window is in effect right now (UTC)
}

func toMaintenanceJSON(w storage.MaintenanceWindow, now time.Time) maintenanceJSON {
	var epID *int64
	if w.EndpointID.Valid {
		epID = &w.EndpointID.Int64
	}
	return maintenanceJSON{
		ID:           w.ID,
		EndpointID:   epID,
		Days:         w.Days,
		StartMinutes: w.StartMinutes,
		EndMinutes:   w.EndMinutes,
		Active:       w.Applies(now),
	}
}

// dayJSON aggregates one UTC day of check history.
type dayJSON struct {
	Date         string  `json:"date"` // YYYY-MM-DD (UTC)
	Total        int     `json:"total"`
	Up           int     `json:"up"`
	Uptime       float64 `json:"uptime"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

func toDayJSON(d storage.DayStat) dayJSON {
	return dayJSON{
		Date:         d.Date,
		Total:        d.Total,
		Up:           d.Up,
		Uptime:       d.Uptime(),
		AvgLatencyMs: d.AvgLatencyMs,
	}
}

func toCheckJSON(c storage.Check) checkJSON {
	return checkJSON{
		CheckedAt:  c.CheckedAt.UTC(),
		Up:         c.Up,
		StatusCode: c.StatusCode,
		LatencyMs:  c.LatencyMs,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeStoreError maps storage errors to HTTP statuses and logs the rest.
func (s *Server) writeStoreError(w http.ResponseWriter, err error, action string, id int64) {
	switch {
	case errors.Is(err, apperror.ErrNotFound):
		writeError(w, http.StatusNotFound, "endpoint not found")
	case errors.Is(err, apperror.ErrDuplicate):
		writeError(w, http.StatusConflict, "an endpoint with this name or URL already exists")
	case errors.Is(err, apperror.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		s.logger.Error(action, "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

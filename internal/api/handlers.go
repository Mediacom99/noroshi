package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"noroshi/internal/bot"
	"noroshi/internal/storage"
)

// minIntervalSeconds mirrors the bot's minimum check interval.
const minIntervalSeconds = 10

// defaultIntervalSeconds mirrors the bot's default check interval.
const defaultIntervalSeconds = 60

// maxWindow is the furthest back check history is kept (30-day retention).
const maxWindow = 30 * 24 * time.Hour

func (s *Server) listEndpoints(w http.ResponseWriter, r *http.Request) {
	eps, err := s.store.ListEndpoints(r.Context())
	if err != nil {
		s.logger.Error("list endpoints", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]endpointJSON, 0, len(eps))
	for _, ep := range eps {
		out = append(out, toEndpointJSON(ep))
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoints": out})
}

type addRequest struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
}

func (s *Server) addEndpoint(w http.ResponseWriter, r *http.Request) {
	var req addRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)

	if err := bot.ValidateName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := bot.ValidateURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	interval := req.IntervalSeconds
	if interval == 0 {
		interval = defaultIntervalSeconds
	}
	if interval < minIntervalSeconds {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("interval must be at least %ds", minIntervalSeconds))
		return
	}

	ep, err := s.store.AddEndpoint(r.Context(), req.Name, req.URL, interval)
	if err != nil {
		s.writeStoreError(w, err, "add endpoint", 0)
		return
	}
	// On scheduler failure the endpoint stays persisted and is monitored after
	// restart — same trade-off as the bot's /add.
	if err := s.scheduler.Add(r.Context(), ep); err != nil {
		s.logger.Error("add endpoint to scheduler", "id", ep.ID, "error", err)
	}
	s.logger.Info("endpoint added", "id", ep.ID, "name", ep.Name, "url", ep.URL)
	writeJSON(w, http.StatusCreated, map[string]any{"endpoint": toEndpointJSON(ep)})
}

func (s *Server) getEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := s.findEndpoint(w, r)
	if !ok {
		return
	}

	now := time.Now().UTC()
	stats := make(map[string]statsJSON, 3)
	for label, window := range map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour} {
		st, err := s.store.GetCheckStats(r.Context(), ep.ID, now.Add(-window))
		if err != nil {
			s.logger.Error("get check stats", "id", ep.ID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		stats[label] = toStatsJSON(st)
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoint": toEndpointJSON(ep), "stats": stats})
}

type updateRequest struct {
	Name            *string `json:"name"`
	IntervalSeconds *int    `json:"interval_seconds"`
	ExpectedStatus  *int    `json:"expected_status"`
	ExpectedKeyword *string `json:"expected_keyword"`
}

func (s *Server) updateEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := s.findEndpoint(w, r)
	if !ok {
		return
	}

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if err := bot.ValidateName(name); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.RenameEndpoint(r.Context(), ep.ID, name); err != nil {
			s.writeStoreError(w, err, "rename endpoint", ep.ID)
			return
		}
		s.logger.Info("endpoint renamed", "id", ep.ID, "name", name)
		ep.Name = name
	}
	if req.IntervalSeconds != nil {
		if *req.IntervalSeconds < minIntervalSeconds {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("interval must be at least %ds", minIntervalSeconds))
			return
		}
		if err := s.updateInterval(r, ep, *req.IntervalSeconds); err != nil {
			s.logger.Error("update interval", "id", ep.ID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		s.logger.Info("interval updated", "id", ep.ID, "interval_seconds", *req.IntervalSeconds)
		ep.IntervalSeconds = *req.IntervalSeconds
	}
	if req.ExpectedStatus != nil {
		code := *req.ExpectedStatus
		if code != 0 && (code < 100 || code > 599) {
			writeError(w, http.StatusBadRequest, "expected status must be 0 (any 2xx) or 100-599")
			return
		}
		if err := s.store.SetExpectedStatus(r.Context(), ep.ID, code); err != nil {
			s.writeStoreError(w, err, "set expected status", ep.ID)
			return
		}
		s.logger.Info("expected status updated", "id", ep.ID, "status", code)
	}
	if req.ExpectedKeyword != nil {
		keyword := *req.ExpectedKeyword
		if keyword != "" {
			if err := bot.ValidateKeywordSpec(keyword); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		if err := s.store.SetExpectedKeyword(r.Context(), ep.ID, keyword); err != nil {
			s.writeStoreError(w, err, "set expected keyword", ep.ID)
			return
		}
		s.logger.Info("expected keyword updated", "id", ep.ID)
	}

	updated, err := s.store.GetEndpoint(r.Context(), ep.ID)
	if err != nil {
		s.writeStoreError(w, err, "get endpoint", ep.ID)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoint": toEndpointJSON(updated)})
}

// updateInterval persists a new interval and reschedules the job, rolling the
// DB back when re-adding the job fails (mirrors the bot's /interval).
func (s *Server) updateInterval(r *http.Request, ep storage.Endpoint, seconds int) error {
	ctx := r.Context()
	oldSeconds := ep.IntervalSeconds

	if err := s.store.UpdateEndpointInterval(ctx, ep.ID, seconds); err != nil {
		return err
	}

	s.scheduler.Remove(ep.ID)
	// Paused endpoints have no job by design — just persist the new interval.
	if ep.Paused {
		return nil
	}
	ep.IntervalSeconds = seconds
	if err := s.scheduler.Add(ctx, ep); err != nil {
		ep.IntervalSeconds = oldSeconds
		if rbErr := s.store.UpdateEndpointInterval(ctx, ep.ID, oldSeconds); rbErr != nil {
			s.logger.Error("rollback interval", "id", ep.ID, "error", rbErr)
		}
		if rbErr := s.scheduler.Add(ctx, ep); rbErr != nil {
			s.logger.Error("restore job", "id", ep.ID, "error", rbErr)
		}
		return err
	}
	return nil
}

func (s *Server) deleteEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := s.findEndpoint(w, r)
	if !ok {
		return
	}
	s.scheduler.Remove(ep.ID)
	if err := s.store.DeleteEndpoint(r.Context(), ep.ID); err != nil {
		s.writeStoreError(w, err, "delete endpoint", ep.ID)
		return
	}
	s.logger.Info("endpoint deleted", "id", ep.ID, "name", ep.Name)
	w.WriteHeader(http.StatusNoContent)
}

type pauseRequest struct {
	Duration string `json:"duration"` // optional Go duration, e.g. "2h"; empty = indefinite
}

func (s *Server) pauseEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := s.findEndpoint(w, r)
	if !ok {
		return
	}

	var req pauseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var until sql.NullTime
	if req.Duration != "" {
		d, err := time.ParseDuration(req.Duration)
		if err != nil || d <= 0 {
			writeError(w, http.StatusBadRequest, "duration must be a positive Go duration (e.g. \"2h\")")
			return
		}
		until = sql.NullTime{Time: time.Now().UTC().Add(d), Valid: true}
	}

	if err := s.store.SetEndpointPaused(r.Context(), ep.ID, true, until); err != nil {
		s.writeStoreError(w, err, "pause endpoint", ep.ID)
		return
	}
	s.scheduler.Remove(ep.ID)
	s.logger.Info("endpoint paused", "id", ep.ID, "name", ep.Name)

	updated, err := s.store.GetEndpoint(r.Context(), ep.ID)
	if err != nil {
		s.writeStoreError(w, err, "get endpoint", ep.ID)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoint": toEndpointJSON(updated)})
}

func (s *Server) resumeEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := s.findEndpoint(w, r)
	if !ok {
		return
	}

	if err := s.store.SetEndpointPaused(r.Context(), ep.ID, false, sql.NullTime{}); err != nil {
		s.writeStoreError(w, err, "resume endpoint", ep.ID)
		return
	}
	// On job failure roll the flag back (mirrors the bot's /resume).
	if err := s.scheduler.Add(r.Context(), ep); err != nil {
		if rbErr := s.store.SetEndpointPaused(r.Context(), ep.ID, true, sql.NullTime{}); rbErr != nil {
			s.logger.Error("rollback pause", "id", ep.ID, "error", rbErr)
		}
		s.logger.Error("resume endpoint", "id", ep.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.logger.Info("endpoint resumed", "id", ep.ID, "name", ep.Name)

	updated, err := s.store.GetEndpoint(r.Context(), ep.ID)
	if err != nil {
		s.writeStoreError(w, err, "get endpoint", ep.ID)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoint": toEndpointJSON(updated)})
}

func (s *Server) checkEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, ok := s.findEndpoint(w, r)
	if !ok {
		return
	}
	// Ad-hoc check: updates status/code/latency but never touches failure
	// counters or notifies (same semantics as the bot's /status).
	updated, err := s.scheduler.CheckNow(r.Context(), ep.ID)
	if err != nil {
		s.writeStoreError(w, err, "check now", ep.ID)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoint": toEndpointJSON(updated)})
}

func (s *Server) listIncidents(w http.ResponseWriter, r *http.Request) {
	ep, ok := s.findEndpoint(w, r)
	if !ok {
		return
	}
	transitions, err := s.store.GetRecentTransitions(r.Context(), ep.ID, 10)
	if err != nil {
		s.logger.Error("get recent transitions", "id", ep.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incidents": toIncidents(transitions)})
}

func (s *Server) listChecks(w http.ResponseWriter, r *http.Request) {
	ep, ok := s.findEndpoint(w, r)
	if !ok {
		return
	}

	window := 24 * time.Hour
	if v := r.URL.Query().Get("window"); v != "" {
		d, err := parseWindow(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		window = d
	}

	checks, err := s.store.GetRecentChecks(r.Context(), ep.ID, time.Now().UTC().Add(-window))
	if err != nil {
		s.logger.Error("get recent checks", "id", ep.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]checkJSON, 0, len(checks))
	for _, c := range checks {
		out = append(out, toCheckJSON(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"checks": out})
}

// listDailyStats serves per-day uptime aggregates for the 30-day history
// strip. ?days=N selects the window (default 30, capped at the 30-day
// retention).
func (s *Server) listDailyStats(w http.ResponseWriter, r *http.Request) {
	ep, ok := s.findEndpoint(w, r)
	if !ok {
		return
	}

	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "days must be a positive integer")
			return
		}
		days = n
	}
	if window := time.Duration(days) * 24 * time.Hour; window > maxWindow {
		days = int(maxWindow / (24 * time.Hour))
	}

	stats, err := s.store.GetDailyStats(r.Context(), ep.ID, time.Now().UTC().Add(-time.Duration(days)*24*time.Hour))
	if err != nil {
		s.logger.Error("get daily stats", "id", ep.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]dayJSON, 0, len(stats))
	for _, d := range stats {
		out = append(out, toDayJSON(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": out})
}

// parseWindow accepts Go durations ("24h") plus day shorthand ("7d", "30d"),
// capped at the 30-day retention window.
func parseWindow(v string) (time.Duration, error) {
	if d, ok := strings.CutSuffix(v, "d"); ok {
		days, err := strconv.Atoi(d)
		if err != nil || days < 1 {
			return 0, fmt.Errorf("window must be a duration like \"24h\" or days like \"7d\"")
		}
		v = fmt.Sprintf("%dh", days*24)
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("window must be a duration like \"24h\" or days like \"7d\"")
	}
	if d > maxWindow {
		d = maxWindow
	}
	return d, nil
}

// findEndpoint resolves the {id} path value to an endpoint, writing the error
// response on failure. ok=false means the response is already written.
func (s *Server) findEndpoint(w http.ResponseWriter, r *http.Request) (storage.Endpoint, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endpoint id")
		return storage.Endpoint{}, false
	}
	ep, err := s.store.GetEndpoint(r.Context(), id)
	if err != nil {
		s.writeStoreError(w, err, "get endpoint", id)
		return storage.Endpoint{}, false
	}
	return ep, true
}

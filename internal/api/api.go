// Package api exposes a JSON HTTP API for the web dashboard. It is mounted
// under /api/ on the health server and only when DASHBOARD_TOKEN is set.
package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"noroshi/internal/storage"
)

// Store defines the storage methods the dashboard API needs.
type Store interface {
	AddEndpoint(ctx context.Context, name, url string, intervalSeconds int) (storage.Endpoint, error)
	GetEndpoint(ctx context.Context, id int64) (storage.Endpoint, error)
	DeleteEndpoint(ctx context.Context, id int64) error
	ListEndpoints(ctx context.Context) ([]storage.Endpoint, error)
	UpdateEndpointInterval(ctx context.Context, id int64, intervalSeconds int) error
	SetEndpointPaused(ctx context.Context, id int64, paused bool, until sql.NullTime) error
	SetExpectedStatus(ctx context.Context, id int64, code int) error
	SetExpectedKeyword(ctx context.Context, id int64, keyword string) error
	RenameEndpoint(ctx context.Context, id int64, newName string) error
	GetCheckStats(ctx context.Context, endpointID int64, since time.Time) (storage.WindowStats, error)
	GetRecentTransitions(ctx context.Context, endpointID int64, limit int) ([]storage.CheckTransition, error)
	GetRecentChecks(ctx context.Context, endpointID int64, since time.Time) ([]storage.Check, error)
	GetDailyStats(ctx context.Context, endpointID int64, since time.Time) ([]storage.DayStat, error)
	AddMaintenanceWindow(ctx context.Context, endpointID sql.NullInt64, days string, startMinutes, endMinutes int) (storage.MaintenanceWindow, error)
	ListMaintenanceWindows(ctx context.Context) ([]storage.MaintenanceWindow, error)
	DeleteMaintenanceWindow(ctx context.Context, id int64) error
}

// Scheduler defines the scheduling methods the dashboard API needs.
type Scheduler interface {
	Add(ctx context.Context, ep storage.Endpoint) error
	Remove(endpointID int64) error
	CheckNow(ctx context.Context, endpointID int64) (storage.Endpoint, error)
}

// Server serves the dashboard JSON API.
type Server struct {
	store     Store
	scheduler Scheduler
	token     string
	origins   map[string]bool
	logger    *slog.Logger
	mux       *http.ServeMux
}

// NewServer builds the dashboard API. token is the required Bearer credential;
// origins lists the allowed CORS origins (empty = cross-origin rejected).
func NewServer(store Store, scheduler Scheduler, token string, origins []string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		store:     store,
		scheduler: scheduler,
		token:     token,
		origins:   make(map[string]bool, len(origins)),
		logger:    logger,
		mux:       http.NewServeMux(),
	}
	for _, o := range origins {
		s.origins[o] = true
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/endpoints", s.listEndpoints)
	s.mux.HandleFunc("POST /api/endpoints", s.addEndpoint)
	s.mux.HandleFunc("GET /api/endpoints/{id}", s.getEndpoint)
	s.mux.HandleFunc("PATCH /api/endpoints/{id}", s.updateEndpoint)
	s.mux.HandleFunc("DELETE /api/endpoints/{id}", s.deleteEndpoint)
	s.mux.HandleFunc("POST /api/endpoints/{id}/pause", s.pauseEndpoint)
	s.mux.HandleFunc("POST /api/endpoints/{id}/resume", s.resumeEndpoint)
	s.mux.HandleFunc("POST /api/endpoints/{id}/check", s.checkEndpoint)
	s.mux.HandleFunc("GET /api/endpoints/{id}/incidents", s.listIncidents)
	s.mux.HandleFunc("GET /api/endpoints/{id}/checks", s.listChecks)
	s.mux.HandleFunc("GET /api/endpoints/{id}/daily", s.listDailyStats)
	s.mux.HandleFunc("GET /api/maintenance", s.listMaintenance)
	s.mux.HandleFunc("POST /api/maintenance", s.addMaintenance)
	s.mux.HandleFunc("DELETE /api/maintenance/{id}", s.deleteMaintenance)
}

// Handler returns the root handler with CORS and Bearer auth applied.
func (s *Server) Handler() http.Handler {
	return s.cors(s.auth(s.mux))
}

// auth requires "Authorization: Bearer <token>" on every request.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// cors answers preflights and allows cross-origin requests from the
// configured origins only.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && s.origins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

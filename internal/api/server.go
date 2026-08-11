package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
)

const (
	AppName = "listarr-go"
	Version = "0.1.0"
)

// Config is HTTP-facing runtime configuration.
type Config struct {
	APIKey       string
	InstanceName string
	ApplyEnabled bool
	SearchBudget *ratelimit.HourlyBudget
}

// Server serves health/status and Phase-1 stubs.
type Server struct {
	cfg Config
	mux *http.ServeMux
}

// New registers routes.
func New(cfg Config) *Server {
	if cfg.InstanceName == "" {
		cfg.InstanceName = "listarr"
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /api/v1/system/status", s.requireAPIKey(s.handleStatus))
	s.mux.HandleFunc("POST /api/v1/sync/{listId}/preview", s.requireAPIKey(s.handlePreviewStub))
	s.mux.HandleFunc("POST /api/v1/sync/{listId}/apply", s.requireAPIKey(s.handleApplyStub))
	return s
}

// Handler returns the root handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	remaining := 0
	limit := 0
	if s.cfg.SearchBudget != nil {
		remaining = s.cfg.SearchBudget.Remaining()
		limit = s.cfg.SearchBudget.Limit()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"appName":               AppName,
		"version":               Version,
		"instanceName":          s.cfg.InstanceName,
		"applyEnabled":          s.cfg.ApplyEnabled,
		"torboxSearchPerHour":   limit,
		"torboxSearchRemaining": remaining,
	})
}

func (s *Server) handlePreviewStub(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"message":     "not implemented",
		"description": "list preview arrives with TMDB/IMDB/Seerr clients",
	})
}

func (s *Server) handleApplyStub(w http.ResponseWriter, _ *http.Request) {
	if !s.cfg.ApplyEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"message":     "apply disabled",
			"description": "set LISTARR_APPLY=1 to enable mutating routes",
		})
		return
	}
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"message":     "not implemented",
		"description": "list apply arrives with TMDB/IMDB/Seerr clients",
	})
}

func (s *Server) requireAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	if s.cfg.APIKey == "" {
		return false
	}
	want := []byte(s.cfg.APIKey)
	if got := r.Header.Get("X-Api-Key"); got != "" {
		return subtle.ConstantTimeCompare([]byte(got), want) == 1
	}
	return subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("apikey")), want) == 1
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

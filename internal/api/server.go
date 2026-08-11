package api

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
	"github.com/bogartusmaximus/listarr-go/internal/store"
	"github.com/bogartusmaximus/listarr-go/internal/syncjob"
	"github.com/bogartusmaximus/listarr-go/internal/tmdb"
)

const (
	AppName = "listarr-go"
	Version = "0.4.0"
)

// Config is HTTP-facing runtime configuration.
type Config struct {
	APIKey       string
	InstanceName string
	ApplyEnabled bool
	SearchBudget *ratelimit.HourlyBudget
	Runner       *syncjob.Runner
	TMDB         *tmdb.Client
	Arr          *arr.Registry
	Store        store.Store
}

// Server serves health/status, discover, arr inventory, and sync endpoints.
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
	s.mux.HandleFunc("GET /api/v1/discover/movies", s.requireAPIKey(s.handleDiscoverMovies))
	s.mux.HandleFunc("GET /api/v1/discover/tv", s.requireAPIKey(s.handleDiscoverTV))
	s.mux.HandleFunc("GET /api/v1/arr/instances", s.requireAPIKey(s.handleArrInstances))
	s.mux.HandleFunc("GET /api/v1/arr/{name}/importlists", s.requireAPIKey(s.handleArrImportLists))
	s.mux.HandleFunc("GET /api/v1/activity", s.requireAPIKey(s.handleActivity))
	s.mux.HandleFunc("POST /api/v1/sync/preview", s.requireAPIKey(s.handleSyncPreview))
	s.mux.HandleFunc("POST /api/v1/sync/apply", s.requireAPIKey(s.handleSyncApply))
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
	arrCount := 0
	if s.cfg.Arr != nil {
		arrCount = s.cfg.Arr.Len()
	}
	storeBackend := ""
	if s.cfg.Store != nil {
		storeBackend = string(s.cfg.Store.Backend())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"appName":               AppName,
		"version":               Version,
		"instanceName":          s.cfg.InstanceName,
		"applyEnabled":          s.cfg.ApplyEnabled,
		"torboxSearchPerHour":   limit,
		"torboxSearchRemaining": remaining,
		"tmdbConfigured":        s.cfg.TMDB != nil,
		"arrInstances":          arrCount,
		"syncConfigured":        s.cfg.Runner != nil,
		"storeBackend":          storeBackend,
	})
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "limit must be a positive integer"})
			return
		}
		limit = n
	}
	runs, err := s.cfg.Store.ListSyncRuns(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"backend": s.cfg.Store.Backend(),
		"runs":    runs,
	})
}

func (s *Server) handleArrInstances(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Arr == nil {
		writeJSON(w, http.StatusOK, map[string]any{"instances": []arr.InstanceMeta{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"instances": s.cfg.Arr.List()})
}

func (s *Server) handleArrImportLists(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Arr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "arr registry not configured"})
		return
	}
	name := r.PathValue("name")
	if c, err := s.cfg.Arr.Radarr(name); err == nil {
		lists, err := c.ListImportLists(r.Context())
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"instance": name, "kind": arr.KindRadarr, "importLists": lists})
		return
	}
	if c, err := s.cfg.Arr.Sonarr(name); err == nil {
		lists, err := c.ListImportLists(r.Context())
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"instance": name, "kind": arr.KindSonarr, "importLists": lists})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"message": "unknown instance"})
}

func (s *Server) handleDiscoverMovies(w http.ResponseWriter, r *http.Request) {
	s.discover(w, r, "movie")
}

func (s *Server) handleDiscoverTV(w http.ResponseWriter, r *http.Request) {
	s.discover(w, r, "tv")
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request, media string) {
	if s.cfg.TMDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "tmdb not configured"})
		return
	}
	q := discoverQueryFromRequest(r)
	var (
		items []tmdb.Item
		err   error
	)
	if media == "movie" {
		items, err = s.cfg.TMDB.DiscoverMovies(r.Context(), q)
	} else {
		items, err = s.cfg.TMDB.DiscoverTV(r.Context(), q)
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleSyncPreview(w http.ResponseWriter, r *http.Request) {
	s.runSync(w, r, true)
}

func (s *Server) handleSyncApply(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.ApplyEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"message":     "apply disabled",
			"description": "set LISTARR_APPLY=1 to enable mutating routes",
		})
		return
	}
	s.runSync(w, r, false)
}

func (s *Server) runSync(w http.ResponseWriter, r *http.Request, dryRun bool) {
	if s.cfg.Runner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "sync runner not configured"})
		return
	}
	var req syncjob.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid json"})
		return
	}
	res, err := s.cfg.Runner.Run(r.Context(), req, dryRun)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	s.persistSyncRun(r, req, res)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) persistSyncRun(r *http.Request, req syncjob.Request, res syncjob.Result) {
	if s.cfg.Store == nil {
		return
	}
	targetInstance := req.Target.Instance
	if targetInstance == "" {
		if req.MediaType == "tv" {
			targetInstance = "sonarr"
		} else {
			targetInstance = "radarr"
		}
	}
	err := s.cfg.Store.SaveSyncRun(r.Context(), store.SyncRun{
		DryRun:         res.DryRun,
		Source:         req.Source,
		MediaType:      req.MediaType,
		SourceInstance: req.SourceInstance,
		TargetInstance: targetInstance,
		Adds:           res.Adds,
		Skips:          res.Skips,
		Deferred:       res.Deferred,
		Errors:         res.Errors,
	})
	if err != nil {
		slog.Error("persist sync run", "err", err, "backend", s.cfg.Store.Backend())
	}
}

func discoverQueryFromRequest(r *http.Request) tmdb.DiscoverQuery {
	q := r.URL.Query()
	out := tmdb.DiscoverQuery{
		SortBy:        q.Get("sort_by"),
		Language:      q.Get("language"),
		Region:        q.Get("region"),
		WithGenres:    q.Get("with_genres"),
		WithoutGenres: q.Get("without_genres"),
		IncludeAdult:  q.Get("include_adult") == "true",
	}
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			out.Page = n
		}
	}
	if v := q.Get("year"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			out.Year = n
		}
	}
	if v := q.Get("vote_average.gte"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			out.VoteAverageGte = n
		}
	}
	if v := q.Get("vote_count.gte"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			out.VoteCountGte = n
		}
	}
	return out
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

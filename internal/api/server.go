package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/appstate"
	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/httpx"
	"github.com/bogartusmaximus/listarr-go/internal/store"
	"github.com/bogartusmaximus/listarr-go/internal/syncjob"
	"github.com/bogartusmaximus/listarr-go/internal/tmdb"
	"github.com/bogartusmaximus/listarr-go/web"
)

const (
	AppName = "listarr-go"
	Version = "0.6.1"
)

// Server serves health/status, settings, discover, arr inventory, and sync endpoints.
type Server struct {
	rt  *appstate.Runtime
	mux *http.ServeMux
}

// New registers routes against a mutable runtime.
func New(rt *appstate.Runtime) *Server {
	if rt == nil {
		rt = &appstate.Runtime{}
	}
	s := &Server{rt: rt, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /api/v1/ui/bootstrap", s.handleUIBootstrap)
	s.mux.HandleFunc("GET /api/v1/system/status", s.requireAPIKey(s.handleStatus))
	s.mux.HandleFunc("GET /api/v1/settings", s.requireAPIKey(s.handleGetSettings))
	s.mux.HandleFunc("PUT /api/v1/settings", s.requireAPIKey(s.handlePutSettings))
	s.mux.HandleFunc("GET /api/v1/discover/movies", s.requireAPIKey(s.handleDiscoverMovies))
	s.mux.HandleFunc("GET /api/v1/discover/tv", s.requireAPIKey(s.handleDiscoverTV))
	s.mux.HandleFunc("GET /api/v1/arr/instances", s.requireAPIKey(s.handleArrInstances))
	s.mux.HandleFunc("POST /api/v1/arr/test", s.requireAPIKey(s.handleArrTest))
	s.mux.HandleFunc("GET /api/v1/arr/{name}/importlists", s.requireAPIKey(s.handleArrImportLists))
	s.mux.HandleFunc("GET /api/v1/arr/{name}/options", s.requireAPIKey(s.handleArrOptions))
	s.mux.HandleFunc("POST /api/v1/plex/auth/pin", s.requireAPIKey(s.handlePlexPinCreate))
	s.mux.HandleFunc("GET /api/v1/plex/auth/pin/{id}", s.requireAPIKey(s.handlePlexPinPoll))
	s.mux.HandleFunc("DELETE /api/v1/plex/auth", s.requireAPIKey(s.handlePlexUnlink))
	s.mux.HandleFunc("POST /api/v1/plex/test", s.requireAPIKey(s.handlePlexTest))
	s.mux.HandleFunc("GET /api/v1/plex/libraries", s.requireAPIKey(s.handlePlexLibraries))
	s.mux.HandleFunc("GET /api/v1/activity", s.requireAPIKey(s.handleActivity))
	s.mux.HandleFunc("GET /api/v1/catalog/titles", s.requireAPIKey(s.handleCatalogList))
	s.mux.HandleFunc("GET /api/v1/catalog/titles/{id}", s.requireAPIKey(s.handleCatalogGet))
	s.mux.HandleFunc("POST /api/v1/catalog/ingest", s.requireAPIKey(s.handleCatalogIngest))
	s.mux.HandleFunc("POST /api/v1/catalog/plex-watched", s.requireAPIKey(s.handleCatalogPlexWatched))
	s.mux.HandleFunc("POST /api/v1/sync/preview", s.requireAPIKey(s.handleSyncPreview))
	s.mux.HandleFunc("POST /api/v1/sync/apply", s.requireAPIKey(s.handleSyncApply))
	s.mountUI()
	return s
}

func (s *Server) mountUI() {
	s.mux.HandleFunc("GET /{$}", s.handleUIIndex)
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", web.Assets()))
}

func (s *Server) handleUIIndex(w http.ResponseWriter, _ *http.Request) {
	raw, err := web.IndexHTML()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "ui unavailable"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// Handler returns the root handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleUIBootstrap gives the embedded operator UI the current API key so no
// paste/connect step is required. Keep LISTARR_LISTEN on loopback.
func (s *Server) handleUIBootstrap(w http.ResponseWriter, _ *http.Request) {
	view := s.rt.View()
	writeJSON(w, http.StatusOK, map[string]any{
		"appName":      AppName,
		"version":      Version,
		"instanceName": view.InstanceName,
		"apiKey":       view.APIKey,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	view := s.rt.View()
	remaining := 0
	limit := 0
	if view.SearchBudget != nil {
		remaining = view.SearchBudget.Remaining()
		limit = view.SearchBudget.Limit()
	}
	arrCount := 0
	if view.Arr != nil {
		arrCount = view.Arr.Len()
	}
	storeBackend := ""
	if view.Store != nil {
		storeBackend = string(view.Store.Backend())
	}
	plex := view.Settings.Plex
	plexConfigured := strings.TrimSpace(plex.Token) != "" || strings.TrimSpace(plex.ServerURL) != ""
	writeJSON(w, http.StatusOK, map[string]any{
		"appName":               AppName,
		"version":               Version,
		"instanceName":          view.InstanceName,
		"safeMode":              view.SafeMode,
		"applyEnabled":          !view.SafeMode,
		"torboxSearchPerHour":   limit,
		"torboxSearchRemaining": remaining,
		"tmdbConfigured":        view.TMDB != nil,
		"plexConfigured":        plexConfigured,
		"plexAccountUsername":   plex.AccountUsername,
		"arrInstances":          arrCount,
		"syncConfigured":        view.Runner != nil,
		"storeBackend":          storeBackend,
	})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	view := s.rt.View()
	writeJSON(w, http.StatusOK, view.Settings)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	var set store.Settings
	if err := json.NewDecoder(r.Body).Decode(&set); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid json"})
		return
	}
	prev := view.Settings
	if strings.TrimSpace(set.Plex.ClientIdentifier) == "" {
		set.Plex.ClientIdentifier = prev.Plex.ClientIdentifier
	}
	if strings.TrimSpace(set.Plex.AccountUsername) == "" && set.Plex.Token == prev.Plex.Token {
		set.Plex.AccountUsername = prev.Plex.AccountUsername
	}
	set = appstate.Normalize(set)
	if err := appstate.Validate(set); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	set.UpdatedAt = time.Now().UTC()
	if err := s.rt.Apply(set); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	if err := view.Store.PutSettings(r.Context(), set); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s.rt.View().Settings)
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
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
	runs, err := view.Store.ListSyncRuns(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"backend": view.Store.Backend(),
		"runs":    runs,
	})
}

func (s *Server) handleArrInstances(w http.ResponseWriter, _ *http.Request) {
	view := s.rt.View()
	if view.Arr == nil {
		writeJSON(w, http.StatusOK, map[string]any{"instances": []arr.InstanceMeta{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"instances": view.Arr.List()})
}

func (s *Server) handleArrTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Kind       string `json:"kind"`
		URL        string `json:"url"`
		APIKey     string `json:"apiKey"`
		AuthCookie string `json:"authCookie"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid json"})
		return
	}
	var httpClient *httpx.Client
	if s.rt != nil {
		httpClient = s.rt.HTTPClient
	}
	result, err := arr.TestConnection(r.Context(), arr.Kind(req.Kind), req.URL, req.APIKey, req.AuthCookie, httpClient)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	status := http.StatusOK
	if !result.OK {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, result)
}

func (s *Server) handleArrImportLists(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Arr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "arr registry not configured"})
		return
	}
	name := r.PathValue("name")
	if c, err := view.Arr.Radarr(name); err == nil {
		lists, err := c.ListImportLists(r.Context())
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"instance": name, "kind": arr.KindRadarr, "importLists": lists})
		return
	}
	if c, err := view.Arr.Sonarr(name); err == nil {
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

func (s *Server) handleArrOptions(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Arr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "arr registry not configured"})
		return
	}
	name := r.PathValue("name")
	meta, err := view.Arr.Lookup(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": err.Error()})
		return
	}
	roots, profiles, err := listArrTargetOptions(r.Context(), view.Arr, meta)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"instance":        meta.Name,
		"kind":            meta.Kind,
		"rootFolders":     roots,
		"qualityProfiles": profiles,
	})
}

func listArrTargetOptions(ctx context.Context, reg *arr.Registry, meta arr.InstanceMeta) ([]arr.RootFolder, []arr.QualityProfile, error) {
	switch meta.Kind {
	case arr.KindRadarr:
		c, err := reg.Radarr(meta.Name)
		if err != nil {
			return nil, nil, err
		}
		return collectTargetOptions(ctx, c.ListRootFolders, c.ListQualityProfiles)
	case arr.KindSonarr:
		c, err := reg.Sonarr(meta.Name)
		if err != nil {
			return nil, nil, err
		}
		return collectTargetOptions(ctx, c.ListRootFolders, c.ListQualityProfiles)
	default:
		return nil, nil, fmt.Errorf("unsupported kind %q", meta.Kind)
	}
}

func collectTargetOptions(
	ctx context.Context,
	rootsFn func(context.Context) ([]arr.RootFolder, error),
	profilesFn func(context.Context) ([]arr.QualityProfile, error),
) ([]arr.RootFolder, []arr.QualityProfile, error) {
	roots, err := rootsFn(ctx)
	if err != nil {
		return nil, nil, err
	}
	profiles, err := profilesFn(ctx)
	if err != nil {
		return nil, nil, err
	}
	return roots, profiles, nil
}

func (s *Server) handleDiscoverMovies(w http.ResponseWriter, r *http.Request) {
	s.discover(w, r, "movie")
}

func (s *Server) handleDiscoverTV(w http.ResponseWriter, r *http.Request) {
	s.discover(w, r, "tv")
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request, media string) {
	view := s.rt.View()
	if view.TMDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "tmdb not configured"})
		return
	}
	q := discoverQueryFromRequest(r)
	var (
		items []tmdb.Item
		err   error
	)
	if media == "movie" {
		items, err = view.TMDB.DiscoverMovies(r.Context(), q)
	} else {
		items, err = view.TMDB.DiscoverTV(r.Context(), q)
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
	view := s.rt.View()
	if view.SafeMode {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"message":     "safe mode enabled",
			"description": "turn off Safe Mode in Settings to allow Apply writes to *arr",
		})
		return
	}
	s.runSync(w, r, false)
}

func (s *Server) runSync(w http.ResponseWriter, r *http.Request, dryRun bool) {
	view := s.rt.View()
	if view.Runner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "sync runner not configured"})
		return
	}
	var req syncjob.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid json"})
		return
	}
	res, err := view.Runner.Run(r.Context(), req, dryRun)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	s.persistSyncRun(r, view, req, res)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) persistSyncRun(r *http.Request, view appstate.Snapshot, req syncjob.Request, res syncjob.Result) {
	if view.Store == nil {
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
	err := view.Store.SaveSyncRun(r.Context(), store.SyncRun{
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
		slog.Error("persist sync run", "err", err, "backend", view.Store.Backend())
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
	view := s.rt.View()
	if view.APIKey == "" {
		return false
	}
	want := []byte(view.APIKey)
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

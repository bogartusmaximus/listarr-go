package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/bogartusmaximus/listarr-go/internal/catalog"
	"github.com/bogartusmaximus/listarr-go/internal/plex"
	"github.com/bogartusmaximus/listarr-go/internal/store"
)

func (s *Server) catalogService() (*catalog.Service, string) {
	view := s.rt.View()
	if view.Store == nil {
		return nil, "store is not configured"
	}
	svc := &catalog.Service{Store: view.Store, Arr: view.Arr}
	plexCfg := view.Settings.Plex
	if strings.TrimSpace(plexCfg.ServerURL) != "" && strings.TrimSpace(plexCfg.Token) != "" {
		client, err := plex.New(plexCfg.ServerURL, plexCfg.Token, plexCfg.ClientIdentifier, s.rt.HTTPClient)
		if err == nil {
			svc.Plex = client
		}
	}
	return svc, ""
}

func (s *Server) handleCatalogList(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	q := r.URL.Query()
	filter := store.CatalogFilter{
		MediaType: strings.ToLower(strings.TrimSpace(q.Get("mediaType"))),
		Query:     strings.TrimSpace(q.Get("q")),
	}
	if filter.MediaType != "" && filter.MediaType != store.CatalogMovie && filter.MediaType != store.CatalogTV {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "mediaType must be movie or tv"})
		return
	}
	switch strings.ToLower(strings.TrimSpace(q.Get("watched"))) {
	case "true", "1":
		v := true
		filter.Watched = &v
	case "false", "0":
		v := false
		filter.Watched = &v
	case "", "all":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "watched must be true, false, or all"})
		return
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "limit must be a positive integer"})
			return
		}
		filter.Limit = n
	}
	if raw := q.Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "offset must be >= 0"})
			return
		}
		filter.Offset = n
	}
	titles, total, err := view.Store.ListCatalogTitles(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"backend": view.Store.Backend(),
		"total":   total,
		"offset":  filter.Offset,
		"limit":   clampCatalogListLimit(filter.Limit),
		"titles":  titles,
	})
}

func clampCatalogListLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 2000 {
		return 2000
	}
	return limit
}

func (s *Server) handleCatalogGet(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	id := r.PathValue("id")
	title, found, err := view.Store.GetCatalogTitle(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "catalog title not found"})
		return
	}
	writeJSON(w, http.StatusOK, title)
}

func (s *Server) handleCatalogIngest(w http.ResponseWriter, r *http.Request) {
	svc, msg := s.catalogService()
	if svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": msg})
		return
	}
	var req catalog.IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid json"})
		return
	}
	res, err := svc.IngestFromArr(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCatalogPlexWatched(w http.ResponseWriter, r *http.Request) {
	svc, msg := s.catalogService()
	if svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": msg})
		return
	}
	if svc.Plex == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "plex is not configured"})
		return
	}
	mediaType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mediaType")))
	res, err := svc.SyncWatchedFromPlex(r.Context(), mediaType)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

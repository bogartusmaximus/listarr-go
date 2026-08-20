package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/bogartusmaximus/listarr-go/internal/appstate"
	"github.com/bogartusmaximus/listarr-go/internal/catalog"
	"github.com/bogartusmaximus/listarr-go/internal/store"
)

func (s *Server) catalogService() (*catalog.Service, string) {
	view := s.rt.View()
	if view.Store == nil {
		return nil, "store is not configured"
	}
	svc := appstate.CatalogService(view, s.rt.HTTPClient)
	if svc == nil {
		return nil, "store is not configured"
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
		MediaType:      strings.ToLower(strings.TrimSpace(q.Get("mediaType"))),
		Query:          strings.TrimSpace(q.Get("q")),
		SourceInstance: strings.TrimSpace(q.Get("sourceInstance")),
		Collection:     strings.TrimSpace(q.Get("collection")),
		Sort:           strings.ToLower(strings.TrimSpace(q.Get("sort"))),
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
	switch strings.ToLower(strings.TrimSpace(q.Get("monitored"))) {
	case "true", "1":
		v := true
		filter.Monitored = &v
	case "false", "0":
		v := false
		filter.Monitored = &v
	case "", "all":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "monitored must be true, false, or all"})
		return
	}
	if raw := q.Get("yearMin"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "yearMin must be an integer"})
			return
		}
		filter.YearMin = &n
	}
	if raw := q.Get("yearMax"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "yearMax must be an integer"})
			return
		}
		filter.YearMax = &n
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
	return store.ClampCatalogLimit(limit)
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
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	var req catalog.IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid json"})
		return
	}
	async := strings.EqualFold(r.URL.Query().Get("async"), "true") || r.URL.Query().Get("async") == "1"
	if async {
		job, err := enqueueNamed(r, view.Store, store.JobKindCatalogIngest, false, true, req)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	svc, msg := s.catalogService()
	if svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": msg})
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
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	mediaType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mediaType")))
	async := strings.EqualFold(r.URL.Query().Get("async"), "true") || r.URL.Query().Get("async") == "1"
	payload := map[string]string{"mediaType": mediaType}
	if async {
		job, err := enqueueNamed(r, view.Store, store.JobKindPlexWatched, false, true, payload)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	svc, msg := s.catalogService()
	if svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": msg})
		return
	}
	if svc.Plex == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "plex is not configured"})
		return
	}
	res, err := svc.SyncWatchedFromPlex(r.Context(), mediaType)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCatalogBulk(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	var patch store.CatalogBulkUpdate
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid json"})
		return
	}
	if len(patch.IDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "ids is required"})
		return
	}
	if patch.Monitored == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "monitored is required"})
		return
	}
	n, err := view.Store.BulkUpdateCatalogTitles(r.Context(), patch)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": n})
}

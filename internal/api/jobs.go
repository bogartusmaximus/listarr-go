package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/jobs"
	"github.com/bogartusmaximus/listarr-go/internal/store"
	"github.com/bogartusmaximus/listarr-go/internal/syncjob"
)

type enqueueJobRequest struct {
	Kind       string          `json:"kind"`
	DryRun     bool            `json:"dryRun"`
	AllowApply *bool           `json:"allowApply,omitempty"`
	Payload    json.RawMessage `json:"payload"`
}

func enqueueNamed(r *http.Request, st store.Store, kind string, dryRun, allowApply bool, payload any) (store.Job, error) {
	return jobs.Enqueue(r.Context(), st, kind, dryRun, allowApply, "", payload)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
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
	rows, err := view.Store.ListJobs(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": rows})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	job, found, err := view.Store.GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleEnqueueJob(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	var req enqueueJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid json"})
		return
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	switch kind {
	case store.JobKindSync, store.JobKindCatalogIngest, store.JobKindPlexWatched:
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "kind must be sync, catalog-ingest, or plex-watched"})
		return
	}
	payload := any(map[string]any{})
	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid payload"})
			return
		}
	}
	allowApply := false
	if req.AllowApply != nil {
		allowApply = *req.AllowApply
	} else if kind == store.JobKindSync && !req.DryRun {
		allowApply = s.resolveManualSyncAllowApply(req.Payload)
	}
	if kind == store.JobKindSync && !req.DryRun && !allowApply {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"message":     "apply not allowed",
			"description": "enable Allow apply on a matching SyncRoute, turn off Safe Mode for manual Sync, or set allowApply on the job",
		})
		return
	}
	job, err := jobs.Enqueue(r.Context(), view.Store, kind, req.DryRun, allowApply, "", payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) resolveManualSyncAllowApply(payload json.RawMessage) bool {
	view := s.rt.View()
	var req syncjob.Request
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &req)
	}
	if route, ok := jobs.MatchSyncRoute(view.Settings.SyncRoutes, req); ok {
		return route.AllowApply
	}
	return !view.SafeMode
}

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	rows, err := view.Store.ListSchedules(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": rows})
}

func (s *Server) handlePutSchedule(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	var sched store.Schedule
	if err := json.NewDecoder(r.Body).Decode(&sched); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid json"})
		return
	}
	if id := strings.TrimSpace(r.PathValue("id")); id != "" {
		sched.ID = id
	}
	normalized, err := store.NormalizeSchedule(sched)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	if normalized.ID == "" {
		normalized.ID = store.NewScheduleID()
	}
	if normalized.NextRunAt == nil {
		now := time.Now().UTC()
		normalized.NextRunAt = &now
	}
	if err := view.Store.PutSchedule(r.Context(), normalized); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	saved, found, err := view.Store.GetSchedule(r.Context(), normalized.ID)
	if err != nil || !found {
		writeJSON(w, http.StatusOK, normalized)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	if err := view.Store.DeleteSchedule(r.Context(), r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRunScheduleNow(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	sched, found, err := view.Store.GetSchedule(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "schedule not found"})
		return
	}
	var payload any = map[string]any{}
	if len(sched.Payload) > 0 {
		_ = json.Unmarshal(sched.Payload, &payload)
	}
	job, err := jobs.Enqueue(r.Context(), view.Store, sched.Kind, !sched.AllowApply, sched.AllowApply, sched.ID, payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	now := time.Now().UTC()
	sched.LastJobID = job.ID
	sched.LastStatus = store.JobQueued
	sched.LastRunAt = &now
	next := now.Add(time.Duration(sched.IntervalMinutes) * time.Minute)
	sched.NextRunAt = &next
	_ = view.Store.PutSchedule(r.Context(), sched)
	writeJSON(w, http.StatusAccepted, job)
}

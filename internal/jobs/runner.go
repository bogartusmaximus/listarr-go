package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/catalog"
	"github.com/bogartusmaximus/listarr-go/internal/store"
	"github.com/bogartusmaximus/listarr-go/internal/syncjob"
)

const (
	pollInterval = 1 * time.Second
	maxWorkers   = 1 // sequential: protects TorBox search budget + *arr rate
)

// RuntimeView is the live deps a job needs (hot-reloads with settings).
type RuntimeView struct {
	Sync     *syncjob.Runner
	Catalog  *catalog.Service
	Settings store.Settings
}

// Viewer supplies a point-in-time runtime view for each job.
type Viewer interface {
	JobRuntime() RuntimeView
}

// Runner drains queued jobs from the store.
type Runner struct {
	Store  store.Store
	Viewer Viewer
}

// Start loops until ctx is cancelled.
func (r *Runner) Start(ctx context.Context) {
	if r == nil || r.Store == nil {
		return
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.drainOnce(ctx)
		}
	}
}

func (r *Runner) drainOnce(ctx context.Context) {
	for i := 0; i < maxWorkers; i++ {
		job, ok, err := r.Store.ClaimNextQueuedJob(ctx)
		if err != nil {
			slog.Error("claim job", "err", err)
			return
		}
		if !ok {
			return
		}
		r.execute(ctx, &job)
	}
}

func (r *Runner) execute(ctx context.Context, job *store.Job) {
	var (
		result any
		err    error
	)
	switch job.Kind {
	case store.JobKindSync:
		result, err = r.runSync(ctx, job)
	case store.JobKindCatalogIngest:
		result, err = r.runIngest(ctx, job)
	case store.JobKindPlexWatched:
		result, err = r.runPlexWatched(ctx, job)
	default:
		err = fmt.Errorf("unsupported job kind %q", job.Kind)
	}
	finished := time.Now().UTC()
	job.FinishedAt = &finished
	if err != nil {
		job.Status = store.JobFailed
		job.Error = err.Error()
		job.Progress.Message = "failed"
	} else {
		job.Status = store.JobSucceeded
		job.Error = ""
		job.Progress.Message = "done"
		if result != nil {
			raw, mErr := json.Marshal(result)
			if mErr == nil {
				job.Result = raw
			}
		}
	}
	if uErr := r.Store.UpdateJob(ctx, *job); uErr != nil {
		slog.Error("update job", "id", job.ID, "err", uErr)
	}
	if job.ScheduleID != "" {
		r.touchSchedule(ctx, *job)
	}
}

func (r *Runner) runtime() RuntimeView {
	if r.Viewer == nil {
		return RuntimeView{}
	}
	return r.Viewer.JobRuntime()
}

func (r *Runner) runSync(ctx context.Context, job *store.Job) (any, error) {
	view := r.runtime()
	if view.Sync == nil {
		return nil, fmt.Errorf("sync runner is not configured")
	}
	var req syncjob.Request
	if err := json.Unmarshal(job.Payload, &req); err != nil {
		return nil, fmt.Errorf("sync payload: %w", err)
	}
	allowApply := ResolveAllowApply(view.Settings, req, job.AllowApply, job.ScheduleID != "")
	dryRun := job.DryRun || !allowApply
	res, err := view.Sync.Run(ctx, req, dryRun)
	if err != nil {
		return nil, err
	}
	job.Progress = store.JobProgress{Done: len(res.Items), Total: len(res.Items), Message: "sync complete"}
	_ = r.Store.UpdateJob(ctx, *job)
	_ = r.Store.SaveSyncRun(ctx, store.SyncRun{
		DryRun:         res.DryRun,
		Source:         req.Source,
		MediaType:      req.MediaType,
		SourceInstance: req.SourceInstance,
		TargetInstance: req.Target.Instance,
		Adds:           res.Adds,
		Skips:          res.Skips,
		Deferred:       res.Deferred,
		Errors:         res.Errors,
	})
	return res, nil
}

func (r *Runner) runIngest(ctx context.Context, job *store.Job) (any, error) {
	view := r.runtime()
	if view.Catalog == nil {
		return nil, fmt.Errorf("catalog service is not configured")
	}
	var req catalog.IngestRequest
	if err := json.Unmarshal(job.Payload, &req); err != nil {
		return nil, fmt.Errorf("ingest payload: %w", err)
	}
	return view.Catalog.IngestFromArr(ctx, req)
}

func (r *Runner) runPlexWatched(ctx context.Context, job *store.Job) (any, error) {
	view := r.runtime()
	if view.Catalog == nil {
		return nil, fmt.Errorf("catalog service is not configured")
	}
	var req struct {
		MediaType string `json:"mediaType"`
	}
	if len(job.Payload) > 0 {
		_ = json.Unmarshal(job.Payload, &req)
	}
	return view.Catalog.SyncWatchedFromPlex(ctx, req.MediaType)
}

func (r *Runner) touchSchedule(ctx context.Context, job store.Job) {
	sched, ok, err := r.Store.GetSchedule(ctx, job.ScheduleID)
	if err != nil || !ok {
		return
	}
	now := time.Now().UTC()
	sched.LastJobID = job.ID
	sched.LastStatus = job.Status
	sched.LastRunAt = &now
	next := now.Add(time.Duration(sched.IntervalMinutes) * time.Minute)
	sched.NextRunAt = &next
	_ = r.Store.PutSchedule(ctx, sched)
}

// Enqueue builds and stores a queued job.
func Enqueue(ctx context.Context, st store.Store, kind string, dryRun, allowApply bool, scheduleID string, payload any) (store.Job, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return store.Job{}, err
	}
	return st.EnqueueJob(ctx, store.Job{
		Kind:       kind,
		Status:     store.JobQueued,
		DryRun:     dryRun,
		AllowApply: allowApply,
		ScheduleID: scheduleID,
		Payload:    raw,
		Progress:   store.JobProgress{Message: "queued"},
	})
}

// ResolveAllowApply picks apply permission: matching SyncRoute wins; else job/schedule flag.
// Global Safe Mode is not a master kill for scheduled jobs.
func ResolveAllowApply(settings store.Settings, req syncjob.Request, jobAllowApply bool, fromSchedule bool) bool {
	if route, ok := MatchSyncRoute(settings.SyncRoutes, req); ok {
		return route.AllowApply
	}
	_ = fromSchedule
	return jobAllowApply
}

// MatchSyncRoute finds a named source→target route for a sync request.
func MatchSyncRoute(routes []store.SyncRoute, req syncjob.Request) (store.SyncRoute, bool) {
	src := strings.ToLower(strings.TrimSpace(req.Source))
	media := strings.ToLower(strings.TrimSpace(req.MediaType))
	srcInst := strings.ToLower(strings.TrimSpace(req.SourceInstance))
	tgt := strings.ToLower(strings.TrimSpace(req.Target.Instance))
	for _, route := range routes {
		if strings.ToLower(strings.TrimSpace(route.Source)) != src {
			continue
		}
		if strings.ToLower(strings.TrimSpace(route.MediaType)) != media {
			continue
		}
		if strings.ToLower(strings.TrimSpace(route.TargetInstance)) != tgt {
			continue
		}
		routeSrc := strings.ToLower(strings.TrimSpace(route.SourceInstance))
		if routeSrc != "" && routeSrc != srcInst {
			continue
		}
		return route, true
	}
	return store.SyncRoute{}, false
}

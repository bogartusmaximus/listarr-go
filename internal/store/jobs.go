package store

import (
	"context"
	"encoding/json"
	"time"
)

// Job kinds executed by the async worker.
const (
	JobKindSync           = "sync"
	JobKindCatalogIngest  = "catalog-ingest"
	JobKindPlexWatched    = "plex-watched"
)

// Job statuses.
const (
	JobQueued     = "queued"
	JobRunning    = "running"
	JobSucceeded  = "succeeded"
	JobFailed     = "failed"
	JobCancelled  = "cancelled"
)

// Job is one async unit of work (manual or scheduled).
type Job struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Status     string          `json:"status"`
	DryRun     bool            `json:"dryRun"`
	AllowApply bool            `json:"allowApply"` // when false, sync apply is forced dry-run
	ScheduleID string          `json:"scheduleId,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	Progress   JobProgress     `json:"progress"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
	StartedAt  *time.Time      `json:"startedAt,omitempty"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
}

// JobProgress is coarse progress for UI polling.
type JobProgress struct {
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Message string `json:"message,omitempty"`
}

// Schedule is a recurring job definition. Safe Mode is per-schedule via AllowApply.
type Schedule struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Enabled         bool            `json:"enabled"`
	Kind            string          `json:"kind"`
	IntervalMinutes int             `json:"intervalMinutes"`
	AllowApply      bool            `json:"allowApply"` // false = always dry-run / preview
	Payload         json.RawMessage `json:"payload,omitempty"`
	LastJobID       string          `json:"lastJobId,omitempty"`
	LastStatus      string          `json:"lastStatus,omitempty"`
	LastRunAt       *time.Time      `json:"lastRunAt,omitempty"`
	NextRunAt       *time.Time      `json:"nextRunAt,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// SyncRoute is a named source→target pairing with its own apply policy.
type SyncRoute struct {
	Name             string `json:"name"`
	Source           string `json:"source"` // tmdb|arr-library|listarr-go
	MediaType        string `json:"mediaType"`
	SourceInstance   string `json:"sourceInstance,omitempty"`
	TargetInstance   string `json:"targetInstance"`
	RootFolderPath   string `json:"rootFolderPath,omitempty"`
	QualityProfileID int    `json:"qualityProfileId,omitempty"`
	AllowApply       bool   `json:"allowApply"`
}

// JobStore persists async jobs.
type JobStore interface {
	EnqueueJob(ctx context.Context, job Job) (Job, error)
	GetJob(ctx context.Context, id string) (Job, bool, error)
	ListJobs(ctx context.Context, limit int) ([]Job, error)
	UpdateJob(ctx context.Context, job Job) error
	ClaimNextQueuedJob(ctx context.Context) (Job, bool, error)
}

// ScheduleStore persists recurring schedules.
type ScheduleStore interface {
	ListSchedules(ctx context.Context) ([]Schedule, error)
	GetSchedule(ctx context.Context, id string) (Schedule, bool, error)
	PutSchedule(ctx context.Context, schedule Schedule) error
	DeleteSchedule(ctx context.Context, id string) error
}

// NewJobID returns a unique job id.
func NewJobID() string {
	return "job-" + time.Now().UTC().Format("20060102T150405.000000000")
}

// NewScheduleID returns a unique schedule id.
func NewScheduleID() string {
	return "sched-" + time.Now().UTC().Format("20060102T150405.000000000")
}

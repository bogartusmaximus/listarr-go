package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *polarsStore) jobsPath() string {
	return filepath.Join(s.dir, "jobs.json")
}

func (s *polarsStore) schedulesPath() string {
	return filepath.Join(s.dir, "schedules.json")
}

func (s *polarsStore) loadJobs() error {
	raw, err := os.ReadFile(s.jobsPath())
	if err != nil {
		if os.IsNotExist(err) {
			s.jobs = make([]Job, 0, 32)
			return nil
		}
		return err
	}
	var jobs []Job
	if err := json.Unmarshal(raw, &jobs); err != nil {
		return fmt.Errorf("polars jobs: %w", err)
	}
	if jobs == nil {
		jobs = []Job{}
	}
	s.jobs = jobs
	return nil
}

func (s *polarsStore) loadSchedules() error {
	raw, err := os.ReadFile(s.schedulesPath())
	if err != nil {
		if os.IsNotExist(err) {
			s.schedules = make([]Schedule, 0, 8)
			return nil
		}
		return err
	}
	var rows []Schedule
	if err := json.Unmarshal(raw, &rows); err != nil {
		return fmt.Errorf("polars schedules: %w", err)
	}
	if rows == nil {
		rows = []Schedule{}
	}
	s.schedules = rows
	return nil
}

func (s *polarsStore) flushJobsLocked() error {
	return writeJSONAtomic(s.jobsPath(), s.jobs)
}

func (s *polarsStore) flushSchedulesLocked() error {
	return writeJSONAtomic(s.schedulesPath(), s.schedules)
}

func writeJSONAtomic(path string, v any) error {
	tmp := path + ".tmp"
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(tmp, raw, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *polarsStore) EnqueueJob(_ context.Context, job Job) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job.ID == "" {
		job.ID = NewJobID()
	}
	if job.Status == "" {
		job.Status = JobQueued
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	s.jobs = append(s.jobs, job)
	if err := s.flushJobsLocked(); err != nil {
		return Job{}, err
	}
	return cloneJob(job), nil
}

func (s *polarsStore) GetJob(_ context.Context, id string) (Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, job := range s.jobs {
		if job.ID == id {
			return cloneJob(job), true, nil
		}
	}
	return Job{}, false, nil
}

func (s *polarsStore) ListJobs(_ context.Context, limit int) ([]Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	n := len(s.jobs)
	if limit > n {
		limit = n
	}
	out := make([]Job, limit)
	for i := 0; i < limit; i++ {
		out[i] = cloneJob(s.jobs[n-1-i])
	}
	return out, nil
}

func (s *polarsStore) UpdateJob(_ context.Context, job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		if s.jobs[i].ID == job.ID {
			s.jobs[i] = job
			return s.flushJobsLocked()
		}
	}
	return fmt.Errorf("job %q not found", job.ID)
}

func (s *polarsStore) ClaimNextQueuedJob(_ context.Context) (Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		if s.jobs[i].Status != JobQueued {
			continue
		}
		now := time.Now().UTC()
		s.jobs[i].Status = JobRunning
		s.jobs[i].StartedAt = &now
		s.jobs[i].Progress.Message = "running"
		if err := s.flushJobsLocked(); err != nil {
			return Job{}, false, err
		}
		return cloneJob(s.jobs[i]), true, nil
	}
	return Job{}, false, nil
}

func (s *polarsStore) ListSchedules(_ context.Context) ([]Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Schedule, len(s.schedules))
	for i, row := range s.schedules {
		out[i] = cloneSchedule(row)
	}
	return out, nil
}

func (s *polarsStore) GetSchedule(_ context.Context, id string) (Schedule, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.schedules {
		if row.ID == id {
			return cloneSchedule(row), true, nil
		}
	}
	return Schedule{}, false, nil
}

func (s *polarsStore) PutSchedule(_ context.Context, schedule Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if schedule.ID == "" {
		schedule.ID = NewScheduleID()
	}
	now := time.Now().UTC()
	if schedule.CreatedAt.IsZero() {
		schedule.CreatedAt = now
	}
	schedule.UpdatedAt = now
	for i := range s.schedules {
		if s.schedules[i].ID == schedule.ID {
			schedule.CreatedAt = s.schedules[i].CreatedAt
			s.schedules[i] = schedule
			return s.flushSchedulesLocked()
		}
	}
	s.schedules = append(s.schedules, schedule)
	return s.flushSchedulesLocked()
}

func (s *polarsStore) DeleteSchedule(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Schedule, 0, len(s.schedules))
	found := false
	for _, row := range s.schedules {
		if row.ID == id {
			found = true
			continue
		}
		out = append(out, row)
	}
	if !found {
		return fmt.Errorf("schedule %q not found", id)
	}
	s.schedules = out
	return s.flushSchedulesLocked()
}

func cloneJob(in Job) Job {
	out := in
	if in.Payload != nil {
		out.Payload = append(json.RawMessage(nil), in.Payload...)
	}
	if in.Result != nil {
		out.Result = append(json.RawMessage(nil), in.Result...)
	}
	if in.StartedAt != nil {
		t := *in.StartedAt
		out.StartedAt = &t
	}
	if in.FinishedAt != nil {
		t := *in.FinishedAt
		out.FinishedAt = &t
	}
	return out
}

func cloneSchedule(in Schedule) Schedule {
	out := in
	if in.Payload != nil {
		out.Payload = append(json.RawMessage(nil), in.Payload...)
	}
	if in.LastRunAt != nil {
		t := *in.LastRunAt
		out.LastRunAt = &t
	}
	if in.NextRunAt != nil {
		t := *in.NextRunAt
		out.NextRunAt = &t
	}
	return out
}

func normalizeSchedule(in Schedule) (Schedule, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	if in.Name == "" {
		return Schedule{}, fmt.Errorf("schedule name is required")
	}
	switch in.Kind {
	case JobKindSync, JobKindCatalogIngest, JobKindPlexWatched:
	default:
		return Schedule{}, fmt.Errorf("schedule kind must be sync, catalog-ingest, or plex-watched")
	}
	if in.IntervalMinutes < 1 {
		return Schedule{}, fmt.Errorf("intervalMinutes must be >= 1")
	}
	if in.IntervalMinutes > 60*24*30 {
		return Schedule{}, fmt.Errorf("intervalMinutes too large")
	}
	return in, nil
}

// NormalizeSchedule validates and trims a schedule document.
func NormalizeSchedule(in Schedule) (Schedule, error) {
	return normalizeSchedule(in)
}

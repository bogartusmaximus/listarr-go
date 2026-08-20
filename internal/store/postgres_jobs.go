package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func (s *postgresStore) migrateJobs(ctx context.Context) error {
	const jobs = `
CREATE TABLE IF NOT EXISTS listarr_jobs (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  status TEXT NOT NULL,
  dry_run BOOLEAN NOT NULL DEFAULT FALSE,
  allow_apply BOOLEAN NOT NULL DEFAULT FALSE,
  schedule_id TEXT NOT NULL DEFAULT '',
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  progress JSONB NOT NULL DEFAULT '{}'::jsonb,
  result JSONB,
  error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ
)`
	if _, err := s.db.ExecContext(ctx, jobs); err != nil {
		return fmt.Errorf("postgres migrate jobs: %w", err)
	}
	const schedules = `
CREATE TABLE IF NOT EXISTS listarr_schedules (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  kind TEXT NOT NULL,
  interval_minutes INT NOT NULL,
  allow_apply BOOLEAN NOT NULL DEFAULT FALSE,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_job_id TEXT NOT NULL DEFAULT '',
  last_status TEXT NOT NULL DEFAULT '',
  last_run_at TIMESTAMPTZ,
  next_run_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`
	if _, err := s.db.ExecContext(ctx, schedules); err != nil {
		return fmt.Errorf("postgres migrate schedules: %w", err)
	}
	_, _ = s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS listarr_jobs_status_idx ON listarr_jobs (status, created_at)`)
	return nil
}

func (s *postgresStore) EnqueueJob(ctx context.Context, job Job) (Job, error) {
	if job.ID == "" {
		job.ID = NewJobID()
	}
	if job.Status == "" {
		job.Status = JobQueued
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	if job.Payload == nil {
		job.Payload = json.RawMessage(`{}`)
	}
	progress, _ := json.Marshal(job.Progress)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO listarr_jobs (
  id, kind, status, dry_run, allow_apply, schedule_id, payload, progress, result, error,
  created_at, started_at, finished_at
) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,$10,$11,$12,$13)`,
		job.ID, job.Kind, job.Status, job.DryRun, job.AllowApply, job.ScheduleID, []byte(job.Payload),
		progress, nullJSON(job.Result), job.Error, job.CreatedAt, nullTime(job.StartedAt), nullTime(job.FinishedAt),
	)
	if err != nil {
		return Job{}, fmt.Errorf("postgres enqueue job: %w", err)
	}
	return job, nil
}

func nullJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}

func (s *postgresStore) GetJob(ctx context.Context, id string) (Job, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, kind, status, dry_run, allow_apply, schedule_id, payload, progress, result, error,
       created_at, started_at, finished_at
FROM listarr_jobs WHERE id = $1`, id)
	job, err := scanJob(row)
	if err == sql.ErrNoRows {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	return job, true, nil
}

func (s *postgresStore) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, kind, status, dry_run, allow_apply, schedule_id, payload, progress, result, error,
       created_at, started_at, finished_at
FROM listarr_jobs ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Job, 0, limit)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (s *postgresStore) UpdateJob(ctx context.Context, job Job) error {
	if job.Payload == nil {
		job.Payload = json.RawMessage(`{}`)
	}
	progress, _ := json.Marshal(job.Progress)
	_, err := s.db.ExecContext(ctx, `
UPDATE listarr_jobs SET
  kind=$2, status=$3, dry_run=$4, allow_apply=$5, schedule_id=$6, payload=$7::jsonb, progress=$8::jsonb,
  result=$9::jsonb, error=$10, started_at=$11, finished_at=$12
WHERE id=$1`,
		job.ID, job.Kind, job.Status, job.DryRun, job.AllowApply, job.ScheduleID, []byte(job.Payload),
		progress, nullJSON(job.Result), job.Error, nullTime(job.StartedAt), nullTime(job.FinishedAt),
	)
	if err != nil {
		return fmt.Errorf("postgres update job: %w", err)
	}
	return nil
}

func (s *postgresStore) ClaimNextQueuedJob(ctx context.Context) (Job, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
SELECT id, kind, status, dry_run, allow_apply, schedule_id, payload, progress, result, error,
       created_at, started_at, finished_at
FROM listarr_jobs
WHERE status = $1
ORDER BY created_at ASC
LIMIT 1
FOR UPDATE SKIP LOCKED`, JobQueued)
	job, err := scanJob(row)
	if err == sql.ErrNoRows {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	now := time.Now().UTC()
	job.Status = JobRunning
	job.StartedAt = &now
	job.Progress.Message = "running"
	progress, _ := json.Marshal(job.Progress)
	_, err = tx.ExecContext(ctx, `
UPDATE listarr_jobs SET status=$2, started_at=$3, progress=$4::jsonb WHERE id=$1`,
		job.ID, job.Status, now, progress,
	)
	if err != nil {
		return Job{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, false, err
	}
	return job, true, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanJob(row scannable) (Job, error) {
	var (
		job                    Job
		payload, progress, res []byte
		started, finished      sql.NullTime
	)
	err := row.Scan(
		&job.ID, &job.Kind, &job.Status, &job.DryRun, &job.AllowApply, &job.ScheduleID,
		&payload, &progress, &res, &job.Error, &job.CreatedAt, &started, &finished,
	)
	if err != nil {
		return Job{}, err
	}
	job.Payload = append(json.RawMessage(nil), payload...)
	_ = json.Unmarshal(progress, &job.Progress)
	if len(res) > 0 {
		job.Result = append(json.RawMessage(nil), res...)
	}
	if started.Valid {
		t := started.Time.UTC()
		job.StartedAt = &t
	}
	if finished.Valid {
		t := finished.Time.UTC()
		job.FinishedAt = &t
	}
	return job, nil
}

func (s *postgresStore) ListSchedules(ctx context.Context) ([]Schedule, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, enabled, kind, interval_minutes, allow_apply, payload,
       last_job_id, last_status, last_run_at, next_run_at, created_at, updated_at
FROM listarr_schedules ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Schedule, 0)
	for rows.Next() {
		sched, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sched)
	}
	return out, rows.Err()
}

func (s *postgresStore) GetSchedule(ctx context.Context, id string) (Schedule, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, enabled, kind, interval_minutes, allow_apply, payload,
       last_job_id, last_status, last_run_at, next_run_at, created_at, updated_at
FROM listarr_schedules WHERE id = $1`, id)
	sched, err := scanSchedule(row)
	if err == sql.ErrNoRows {
		return Schedule{}, false, nil
	}
	if err != nil {
		return Schedule{}, false, err
	}
	return sched, true, nil
}

func (s *postgresStore) PutSchedule(ctx context.Context, schedule Schedule) error {
	if schedule.ID == "" {
		schedule.ID = NewScheduleID()
	}
	now := time.Now().UTC()
	if schedule.CreatedAt.IsZero() {
		schedule.CreatedAt = now
	}
	schedule.UpdatedAt = now
	if schedule.Payload == nil {
		schedule.Payload = json.RawMessage(`{}`)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO listarr_schedules (
  id, name, enabled, kind, interval_minutes, allow_apply, payload,
  last_job_id, last_status, last_run_at, next_run_at, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10,$11,$12,$13)
ON CONFLICT (id) DO UPDATE SET
  name=EXCLUDED.name, enabled=EXCLUDED.enabled, kind=EXCLUDED.kind,
  interval_minutes=EXCLUDED.interval_minutes, allow_apply=EXCLUDED.allow_apply,
  payload=EXCLUDED.payload, last_job_id=EXCLUDED.last_job_id, last_status=EXCLUDED.last_status,
  last_run_at=EXCLUDED.last_run_at, next_run_at=EXCLUDED.next_run_at, updated_at=EXCLUDED.updated_at`,
		schedule.ID, schedule.Name, schedule.Enabled, schedule.Kind, schedule.IntervalMinutes, schedule.AllowApply,
		[]byte(schedule.Payload), schedule.LastJobID, schedule.LastStatus,
		nullTime(schedule.LastRunAt), nullTime(schedule.NextRunAt), schedule.CreatedAt, schedule.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres put schedule: %w", err)
	}
	return nil
}

func (s *postgresStore) DeleteSchedule(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM listarr_schedules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("schedule %q not found", id)
	}
	return nil
}

func scanSchedule(row scannable) (Schedule, error) {
	var (
		sched              Schedule
		payload            []byte
		lastRun, nextRun   sql.NullTime
	)
	err := row.Scan(
		&sched.ID, &sched.Name, &sched.Enabled, &sched.Kind, &sched.IntervalMinutes, &sched.AllowApply,
		&payload, &sched.LastJobID, &sched.LastStatus, &lastRun, &nextRun, &sched.CreatedAt, &sched.UpdatedAt,
	)
	if err != nil {
		return Schedule{}, err
	}
	sched.Payload = append(json.RawMessage(nil), payload...)
	if lastRun.Valid {
		t := lastRun.Time.UTC()
		sched.LastRunAt = &t
	}
	if nextRun.Valid {
		t := nextRun.Time.UTC()
		sched.NextRunAt = &t
	}
	return sched, nil
}
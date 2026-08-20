package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/jobs"
	"github.com/bogartusmaximus/listarr-go/internal/store"
)

const tickEvery = 30 * time.Second

// Service enqueues due schedules as async jobs.
type Service struct {
	Store store.Store
}

// Start polls schedules until ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	if s == nil || s.Store == nil {
		return
	}
	ticker := time.NewTicker(tickEvery)
	defer ticker.Stop()
	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Service) tick(ctx context.Context) {
	rows, err := s.Store.ListSchedules(ctx)
	if err != nil {
		slog.Error("list schedules", "err", err)
		return
	}
	now := time.Now().UTC()
	for _, sched := range rows {
		if !sched.Enabled {
			continue
		}
		if sched.NextRunAt != nil && sched.NextRunAt.After(now) {
			continue
		}
		if err := s.enqueueDue(ctx, sched, now); err != nil {
			slog.Error("enqueue schedule", "id", sched.ID, "err", err)
		}
	}
}

func (s *Service) enqueueDue(ctx context.Context, sched store.Schedule, now time.Time) error {
	dryRun := !sched.AllowApply
	job, err := jobs.Enqueue(ctx, s.Store, sched.Kind, dryRun, sched.AllowApply, sched.ID, decodePayload(sched.Payload))
	if err != nil {
		return err
	}
	sched.LastJobID = job.ID
	sched.LastStatus = store.JobQueued
	sched.LastRunAt = &now
	next := now.Add(time.Duration(sched.IntervalMinutes) * time.Minute)
	sched.NextRunAt = &next
	return s.Store.PutSchedule(ctx, sched)
}

func decodePayload(raw []byte) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return map[string]any{}
	}
	return v
}

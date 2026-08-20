package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/store"
)

func TestPolarsJobsAndSchedules(t *testing.T) {
	st, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()

	job, err := st.EnqueueJob(ctx, store.Job{
		Kind:       store.JobKindSync,
		Status:     store.JobQueued,
		DryRun:     true,
		AllowApply: false,
		Payload:    json.RawMessage(`{"source":"tmdb"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.ID == "" {
		t.Fatal("expected job id")
	}
	claimed, ok, err := st.ClaimNextQueuedJob(ctx)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if claimed.ID != job.ID || claimed.Status != store.JobRunning {
		t.Fatalf("claimed=%+v", claimed)
	}
	claimed.Status = store.JobSucceeded
	claimed.Progress = store.JobProgress{Message: "done", Done: 1, Total: 1}
	if err := st.UpdateJob(ctx, claimed); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	sched := store.Schedule{
		Name:            "nightly",
		Enabled:         true,
		Kind:            store.JobKindCatalogIngest,
		IntervalMinutes: 60,
		AllowApply:      true,
		Payload:         json.RawMessage(`{"sourceInstance":"local","mediaType":"movie"}`),
		NextRunAt:       &now,
	}
	normalized, err := store.NormalizeSchedule(sched)
	if err != nil {
		t.Fatal(err)
	}
	normalized.ID = store.NewScheduleID()
	if err := st.PutSchedule(ctx, normalized); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListSchedules(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("schedules=%v err=%v", rows, err)
	}
	if !rows[0].AllowApply {
		t.Fatal("expected allowApply")
	}
	if err := st.DeleteSchedule(ctx, rows[0].ID); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogFilterExtras(t *testing.T) {
	st, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	_, err = st.UpsertCatalogTitles(ctx, []store.CatalogTitle{
		{MediaType: store.CatalogMovie, TMDBID: 1, Title: "Alpha", Year: 2020, Monitored: true, CollectionName: "Saga", SourceInstances: []string{"local"}},
		{MediaType: store.CatalogMovie, TMDBID: 2, Title: "Beta", Year: 1999, Monitored: false, Watched: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	mon := true
	rows, total, err := st.ListCatalogTitles(ctx, store.CatalogFilter{Monitored: &mon, Collection: "sag", Sort: "year"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 || rows[0].Title != "Alpha" {
		t.Fatalf("filter got total=%d rows=%+v", total, rows)
	}
	n, err := st.BulkUpdateCatalogTitles(ctx, store.CatalogBulkUpdate{IDs: []string{rows[0].ID}, Monitored: boolPtr(false)})
	if err != nil || n != 1 {
		t.Fatalf("bulk n=%d err=%v", n, err)
	}
}

func boolPtr(v bool) *bool { return &v }

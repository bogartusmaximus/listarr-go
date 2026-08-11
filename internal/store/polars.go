package store

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// polarsStore keeps runs in memory and mirrors them to CSV for Polars tests.
type polarsStore struct {
	dir  string
	mu   sync.Mutex
	runs []SyncRun
}

func openPolars(dir string) (Store, error) {
	if dir == "" {
		dir = filepath.Join("data", "polars")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("polars dir: %w", err)
	}
	s := &polarsStore{dir: dir, runs: make([]SyncRun, 0, 64)}
	if err := s.loadCSV(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *polarsStore) Backend() Backend { return BackendPolars }

func (s *polarsStore) Close() error { return nil }

func (s *polarsStore) Ping(context.Context) error { return nil }

func (s *polarsStore) SaveSyncRun(_ context.Context, run SyncRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run.ID == "" {
		run.ID = fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	s.runs = append(s.runs, run)
	return s.flushCSVLocked()
}

func (s *polarsStore) ListSyncRuns(_ context.Context, limit int) ([]SyncRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if limit > len(s.runs) {
		limit = len(s.runs)
	}
	out := make([]SyncRun, limit)
	// newest last in file; return newest first
	for i := 0; i < limit; i++ {
		out[i] = s.runs[len(s.runs)-1-i]
	}
	return out, nil
}

// RunsCSVPath is the Polars-readable activity dump.
func (s *polarsStore) RunsCSVPath() string {
	return filepath.Join(s.dir, "sync_runs.csv")
}

func (s *polarsStore) loadCSV() error {
	path := s.RunsCSVPath()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return err
	}
	if len(rows) <= 1 {
		return nil
	}
	for _, row := range rows[1:] {
		if len(row) < 11 {
			continue
		}
		created, _ := time.Parse(time.RFC3339, row[10])
		s.runs = append(s.runs, SyncRun{
			ID:             row[0],
			DryRun:         row[1] == "true",
			Source:         row[2],
			MediaType:      row[3],
			SourceInstance: row[4],
			TargetInstance: row[5],
			Adds:           atoi(row[6]),
			Skips:          atoi(row[7]),
			Deferred:       atoi(row[8]),
			Errors:         atoi(row[9]),
			CreatedAt:      created,
		})
	}
	return nil
}

func (s *polarsStore) flushCSVLocked() error {
	path := s.RunsCSVPath()
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{
		"id", "dry_run", "source", "media_type", "source_instance", "target_instance",
		"adds", "skips", "deferred_search", "errors", "created_at",
	})
	for _, run := range s.runs {
		_ = w.Write([]string{
			run.ID,
			strconv.FormatBool(run.DryRun),
			run.Source,
			run.MediaType,
			run.SourceInstance,
			run.TargetInstance,
			strconv.Itoa(run.Adds),
			strconv.Itoa(run.Skips),
			strconv.Itoa(run.Deferred),
			strconv.Itoa(run.Errors),
			run.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

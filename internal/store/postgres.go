package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type postgresStore struct {
	db             *sql.DB
	polarsCacheDir string
}

func openPostgres(databaseURL, polarsCacheDir string) (Store, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("postgres backend requires LISTARR_DATABASE_URL (or DATABASE_URL)")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(time.Hour)
	s := &postgresStore{db: db, polarsCacheDir: strings.TrimSpace(polarsCacheDir)}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Ping(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *postgresStore) Backend() Backend { return BackendPostgres }

func (s *postgresStore) Close() error { return s.db.Close() }

func (s *postgresStore) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("postgres ping: %w", err)
	}
	return nil
}

func (s *postgresStore) migrate(ctx context.Context) error {
	const runs = `
CREATE TABLE IF NOT EXISTS listarr_sync_runs (
  id TEXT PRIMARY KEY,
  dry_run BOOLEAN NOT NULL,
  source TEXT NOT NULL,
  media_type TEXT NOT NULL,
  source_instance TEXT NOT NULL DEFAULT '',
  target_instance TEXT NOT NULL DEFAULT '',
  adds INT NOT NULL DEFAULT 0,
  skips INT NOT NULL DEFAULT 0,
  deferred_search INT NOT NULL DEFAULT 0,
  errors INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`
	if _, err := s.db.ExecContext(ctx, runs); err != nil {
		return fmt.Errorf("postgres migrate sync runs: %w", err)
	}
	const settings = `
CREATE TABLE IF NOT EXISTS listarr_settings (
  id SMALLINT PRIMARY KEY CHECK (id = 1),
  document JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`
	if _, err := s.db.ExecContext(ctx, settings); err != nil {
		return fmt.Errorf("postgres migrate settings: %w", err)
	}
	return s.migrateCatalog(ctx)
}

func (s *postgresStore) GetSettings(ctx context.Context) (Settings, bool, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT document FROM listarr_settings WHERE id = 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return Settings{}, false, nil
	}
	if err != nil {
		return Settings{}, false, fmt.Errorf("postgres get settings: %w", err)
	}
	var set Settings
	if err := json.Unmarshal(raw, &set); err != nil {
		return Settings{}, false, fmt.Errorf("postgres settings decode: %w", err)
	}
	if set.ArrInstances == nil {
		set.ArrInstances = []ArrInstanceSettings{}
	}
	return set, true, nil
}

func (s *postgresStore) PutSettings(ctx context.Context, settings Settings) error {
	if settings.UpdatedAt.IsZero() {
		settings.UpdatedAt = time.Now().UTC()
	}
	if settings.ArrInstances == nil {
		settings.ArrInstances = []ArrInstanceSettings{}
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO listarr_settings (id, document, updated_at)
VALUES (1, $1::jsonb, $2)
ON CONFLICT (id) DO UPDATE SET document = EXCLUDED.document, updated_at = EXCLUDED.updated_at`,
		raw, settings.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres put settings: %w", err)
	}
	return nil
}

func (s *postgresStore) SaveSyncRun(ctx context.Context, run SyncRun) error {
	if run.ID == "" {
		run.ID = fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO listarr_sync_runs (
  id, dry_run, source, media_type, source_instance, target_instance,
  adds, skips, deferred_search, errors, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (id) DO NOTHING`,
		run.ID, run.DryRun, run.Source, run.MediaType, run.SourceInstance, run.TargetInstance,
		run.Adds, run.Skips, run.Deferred, run.Errors, run.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres save sync run: %w", err)
	}
	return nil
}

func (s *postgresStore) ListSyncRuns(ctx context.Context, limit int) ([]SyncRun, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, dry_run, source, media_type, source_instance, target_instance,
       adds, skips, deferred_search, errors, created_at
FROM listarr_sync_runs
ORDER BY created_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres list sync runs: %w", err)
	}
	defer rows.Close()

	out := make([]SyncRun, 0, limit)
	for rows.Next() {
		var run SyncRun
		if err := rows.Scan(
			&run.ID, &run.DryRun, &run.Source, &run.MediaType, &run.SourceInstance, &run.TargetInstance,
			&run.Adds, &run.Skips, &run.Deferred, &run.Errors, &run.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

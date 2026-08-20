package store

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *postgresStore) migrateCatalog(ctx context.Context) error {
	const titles = `
CREATE TABLE IF NOT EXISTS listarr_catalog_titles (
  id TEXT PRIMARY KEY,
  media_type TEXT NOT NULL CHECK (media_type IN ('movie','tv')),
  tmdb_id INT NOT NULL DEFAULT 0,
  imdb_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  year INT NOT NULL DEFAULT 0,
  overview TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL DEFAULT '',
  monitored BOOLEAN NOT NULL DEFAULT FALSE,
  collection_tmdb_id INT NOT NULL DEFAULT 0,
  collection_name TEXT NOT NULL DEFAULT '',
  seasons JSONB NOT NULL DEFAULT '[]'::jsonb,
  watched BOOLEAN NOT NULL DEFAULT FALSE,
  watched_at TIMESTAMPTZ,
  plex_rating_key TEXT NOT NULL DEFAULT '',
  source_instances JSONB NOT NULL DEFAULT '[]'::jsonb,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT listarr_catalog_tmdb_or_imdb CHECK (tmdb_id > 0 OR imdb_id <> '')
)`
	if _, err := s.db.ExecContext(ctx, titles); err != nil {
		return fmt.Errorf("postgres migrate catalog titles: %w", err)
	}
	indexes := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS listarr_catalog_tmdb_uidx
           ON listarr_catalog_titles (media_type, tmdb_id) WHERE tmdb_id > 0`,
		`CREATE UNIQUE INDEX IF NOT EXISTS listarr_catalog_imdb_uidx
           ON listarr_catalog_titles (media_type, imdb_id) WHERE imdb_id <> ''`,
		`CREATE INDEX IF NOT EXISTS listarr_catalog_title_idx ON listarr_catalog_titles (title)`,
		`CREATE INDEX IF NOT EXISTS listarr_catalog_watched_idx ON listarr_catalog_titles (watched)`,
	}
	for _, stmt := range indexes {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("postgres migrate catalog index: %w", err)
		}
	}
	return nil
}

func (s *postgresStore) UpsertCatalogTitles(ctx context.Context, titles []CatalogTitle) (CatalogUpsertStats, error) {
	stats := CatalogUpsertStats{}
	for _, raw := range titles {
		incoming, err := NormalizeCatalogTitle(raw)
		if err != nil {
			return stats, err
		}
		existing, found, err := s.findCatalogByIDs(ctx, incoming.MediaType, incoming.TMDBID, incoming.IMDBID)
		if err != nil {
			return stats, err
		}
		if !found {
			incoming.ID = newCatalogID(incoming)
			if err := s.insertCatalogTitle(ctx, incoming); err != nil {
				return stats, err
			}
			stats.Added++
			continue
		}
		merged := MergeCatalogTitle(existing, incoming)
		merged.ID = existing.ID
		if err := s.updateCatalogTitle(ctx, merged); err != nil {
			return stats, err
		}
		stats.Updated++
	}
	n, err := s.countCatalog(ctx)
	if err != nil {
		return stats, err
	}
	stats.Total = n
	_ = s.mirrorCatalogCSV(ctx)
	return stats, nil
}

func (s *postgresStore) findCatalogByIDs(ctx context.Context, mediaType string, tmdbID int, imdbID string) (CatalogTitle, bool, error) {
	imdbID = strings.TrimSpace(imdbID)
	if tmdbID > 0 {
		t, ok, err := s.scanCatalogWhere(ctx, `media_type = $1 AND tmdb_id = $2`, mediaType, tmdbID)
		if err != nil || ok {
			return t, ok, err
		}
	}
	if imdbID != "" {
		return s.scanCatalogWhere(ctx, `media_type = $1 AND imdb_id = $2`, mediaType, imdbID)
	}
	return CatalogTitle{}, false, nil
}

func (s *postgresStore) scanCatalogWhere(ctx context.Context, where string, args ...any) (CatalogTitle, bool, error) {
	q := `SELECT id, media_type, tmdb_id, imdb_id, title, year, overview, path, monitored,
	             collection_tmdb_id, collection_name, seasons, watched, watched_at,
	             plex_rating_key, source_instances, updated_at
	      FROM listarr_catalog_titles WHERE ` + where + ` LIMIT 1`
	row := s.db.QueryRowContext(ctx, q, args...)
	t, err := scanCatalogTitle(row)
	if err == sql.ErrNoRows {
		return CatalogTitle{}, false, nil
	}
	if err != nil {
		return CatalogTitle{}, false, err
	}
	return t, true, nil
}

type catalogScanner interface {
	Scan(dest ...any) error
}

func scanCatalogTitle(row catalogScanner) (CatalogTitle, error) {
	var (
		t          CatalogTitle
		seasonsRaw []byte
		sourcesRaw []byte
		watchedAt  sql.NullTime
	)
	err := row.Scan(
		&t.ID, &t.MediaType, &t.TMDBID, &t.IMDBID, &t.Title, &t.Year, &t.Overview, &t.Path, &t.Monitored,
		&t.CollectionTMDBID, &t.CollectionName, &seasonsRaw, &t.Watched, &watchedAt,
		&t.PlexRatingKey, &sourcesRaw, &t.UpdatedAt,
	)
	if err != nil {
		return CatalogTitle{}, err
	}
	if len(seasonsRaw) > 0 {
		_ = json.Unmarshal(seasonsRaw, &t.Seasons)
	}
	if t.Seasons == nil {
		t.Seasons = []CatalogSeason{}
	}
	if len(sourcesRaw) > 0 {
		_ = json.Unmarshal(sourcesRaw, &t.SourceInstances)
	}
	if t.SourceInstances == nil {
		t.SourceInstances = []string{}
	}
	if watchedAt.Valid {
		ts := watchedAt.Time.UTC()
		t.WatchedAt = &ts
	}
	return t, nil
}

func (s *postgresStore) insertCatalogTitle(ctx context.Context, t CatalogTitle) error {
	seasons, err := json.Marshal(t.Seasons)
	if err != nil {
		return err
	}
	sources, err := json.Marshal(t.SourceInstances)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO listarr_catalog_titles (
  id, media_type, tmdb_id, imdb_id, title, year, overview, path, monitored,
  collection_tmdb_id, collection_name, seasons, watched, watched_at,
  plex_rating_key, source_instances, updated_at
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$14,$15,$16::jsonb,$17
)`,
		t.ID, t.MediaType, t.TMDBID, t.IMDBID, t.Title, t.Year, t.Overview, t.Path, t.Monitored,
		t.CollectionTMDBID, t.CollectionName, seasons, t.Watched, nullTime(t.WatchedAt),
		t.PlexRatingKey, sources, t.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres insert catalog: %w", err)
	}
	return nil
}

func (s *postgresStore) updateCatalogTitle(ctx context.Context, t CatalogTitle) error {
	seasons, err := json.Marshal(t.Seasons)
	if err != nil {
		return err
	}
	sources, err := json.Marshal(t.SourceInstances)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE listarr_catalog_titles SET
  media_type=$2, tmdb_id=$3, imdb_id=$4, title=$5, year=$6, overview=$7, path=$8, monitored=$9,
  collection_tmdb_id=$10, collection_name=$11, seasons=$12::jsonb, watched=$13, watched_at=$14,
  plex_rating_key=$15, source_instances=$16::jsonb, updated_at=$17
WHERE id=$1`,
		t.ID, t.MediaType, t.TMDBID, t.IMDBID, t.Title, t.Year, t.Overview, t.Path, t.Monitored,
		t.CollectionTMDBID, t.CollectionName, seasons, t.Watched, nullTime(t.WatchedAt),
		t.PlexRatingKey, sources, t.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres update catalog: %w", err)
	}
	return nil
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

func (s *postgresStore) countCatalog(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM listarr_catalog_titles`).Scan(&n)
	return n, err
}

func (s *postgresStore) ListCatalogTitles(ctx context.Context, filter CatalogFilter) ([]CatalogTitle, int, error) {
	where := []string{"1=1"}
	args := make([]any, 0, 6)
	argN := 1
	if filter.MediaType != "" {
		where = append(where, fmt.Sprintf("media_type = $%d", argN))
		args = append(args, filter.MediaType)
		argN++
	}
	if filter.Watched != nil {
		where = append(where, fmt.Sprintf("watched = $%d", argN))
		args = append(args, *filter.Watched)
		argN++
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		where = append(where, fmt.Sprintf("(title ILIKE $%d OR imdb_id ILIKE $%d OR CAST(tmdb_id AS TEXT) LIKE $%d)", argN, argN, argN))
		args = append(args, "%"+q+"%")
		argN++
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	countQ := `SELECT COUNT(*) FROM listarr_catalog_titles WHERE ` + whereSQL
	if err := s.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("postgres catalog count: %w", err)
	}
	limit := clampCatalogLimit(filter.Limit)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	listArgs := append(append([]any{}, args...), limit, offset)
	listQ := fmt.Sprintf(`
SELECT id, media_type, tmdb_id, imdb_id, title, year, overview, path, monitored,
       collection_tmdb_id, collection_name, seasons, watched, watched_at,
       plex_rating_key, source_instances, updated_at
FROM listarr_catalog_titles
WHERE %s
ORDER BY title ASC, id ASC
LIMIT $%d OFFSET $%d`, whereSQL, argN, argN+1)
	rows, err := s.db.QueryContext(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres catalog list: %w", err)
	}
	defer rows.Close()
	out := make([]CatalogTitle, 0, limit)
	for rows.Next() {
		t, err := scanCatalogTitle(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

func (s *postgresStore) GetCatalogTitle(ctx context.Context, id string) (CatalogTitle, bool, error) {
	t, ok, err := s.scanCatalogWhere(ctx, `id = $1`, strings.TrimSpace(id))
	if err != nil {
		return CatalogTitle{}, false, fmt.Errorf("postgres get catalog: %w", err)
	}
	return t, ok, nil
}

func (s *postgresStore) ApplyCatalogWatched(ctx context.Context, patches []CatalogWatchedPatch) (int, error) {
	updated := 0
	for _, patch := range patches {
		media := strings.ToLower(strings.TrimSpace(patch.MediaType))
		existing, found, err := s.findCatalogByIDs(ctx, media, patch.TMDBID, patch.IMDBID)
		if err != nil {
			return updated, err
		}
		if !found {
			continue
		}
		existing.Watched = patch.Watched
		existing.WatchedAt = patch.WatchedAt
		if patch.PlexRatingKey != "" {
			existing.PlexRatingKey = patch.PlexRatingKey
		}
		existing.UpdatedAt = time.Now().UTC()
		if err := s.updateCatalogTitle(ctx, existing); err != nil {
			return updated, err
		}
		updated++
	}
	_ = s.mirrorCatalogCSV(ctx)
	return updated, nil
}

func (s *postgresStore) mirrorCatalogCSV(ctx context.Context) error {
	if s.polarsCacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(s.polarsCacheDir, 0o750); err != nil {
		return err
	}
	titles, _, err := s.ListCatalogTitles(ctx, CatalogFilter{Limit: 2000, Offset: 0})
	if err != nil {
		return err
	}
	path := filepath.Join(s.polarsCacheDir, "catalog_titles.csv")
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	_ = w.Write(catalogCSVHeader())
	for _, t := range titles {
		_ = w.Write(catalogCSVRow(t))
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

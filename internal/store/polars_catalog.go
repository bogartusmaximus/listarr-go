package store

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *polarsStore) catalogPath() string {
	return filepath.Join(s.dir, "catalog_titles.csv")
}

func (s *polarsStore) loadCatalog() error {
	path := s.catalogPath()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.catalog = make([]CatalogTitle, 0, 64)
			return nil
		}
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("polars catalog csv: %w", err)
	}
	s.catalog = make([]CatalogTitle, 0, len(rows))
	if len(rows) <= 1 {
		return nil
	}
	for _, row := range rows[1:] {
		if len(row) < 17 {
			continue
		}
		t, err := parseCatalogCSVRow(row)
		if err != nil {
			continue
		}
		s.catalog = append(s.catalog, t)
	}
	return nil
}

func parseCatalogCSVRow(row []string) (CatalogTitle, error) {
	updated, _ := time.Parse(time.RFC3339, row[15])
	var watchedAt *time.Time
	if strings.TrimSpace(row[13]) != "" {
		t, err := time.Parse(time.RFC3339, row[13])
		if err == nil {
			watchedAt = &t
		}
	}
	var seasons []CatalogSeason
	if strings.TrimSpace(row[11]) != "" {
		_ = json.Unmarshal([]byte(row[11]), &seasons)
	}
	if seasons == nil {
		seasons = []CatalogSeason{}
	}
	var sources []string
	if strings.TrimSpace(row[16]) != "" {
		_ = json.Unmarshal([]byte(row[16]), &sources)
	}
	if sources == nil {
		sources = []string{}
	}
	return CatalogTitle{
		ID:               row[0],
		MediaType:        row[1],
		TMDBID:           atoi(row[2]),
		IMDBID:           row[3],
		Title:            row[4],
		Year:             atoi(row[5]),
		Overview:         row[6],
		Path:             row[7],
		Monitored:        row[8] == "true",
		CollectionTMDBID: atoi(row[9]),
		CollectionName:   row[10],
		Seasons:          seasons,
		Watched:          row[12] == "true",
		WatchedAt:        watchedAt,
		PlexRatingKey:    row[14],
		SourceInstances:  sources,
		UpdatedAt:        updated,
	}, nil
}

func catalogCSVHeader() []string {
	return []string{
		"id", "media_type", "tmdb_id", "imdb_id", "title", "year", "overview", "path",
		"monitored", "collection_tmdb_id", "collection_name", "seasons_json",
		"watched", "watched_at", "plex_rating_key", "updated_at", "source_instances_json",
	}
}

func catalogCSVRow(t CatalogTitle) []string {
	seasons, _ := json.Marshal(t.Seasons)
	sources, _ := json.Marshal(t.SourceInstances)
	watchedAt := ""
	if t.WatchedAt != nil {
		watchedAt = t.WatchedAt.UTC().Format(time.RFC3339)
	}
	return []string{
		t.ID,
		t.MediaType,
		strconv.Itoa(t.TMDBID),
		t.IMDBID,
		t.Title,
		strconv.Itoa(t.Year),
		t.Overview,
		t.Path,
		strconv.FormatBool(t.Monitored),
		strconv.Itoa(t.CollectionTMDBID),
		t.CollectionName,
		string(seasons),
		strconv.FormatBool(t.Watched),
		watchedAt,
		t.PlexRatingKey,
		t.UpdatedAt.UTC().Format(time.RFC3339),
		string(sources),
	}
}

func (s *polarsStore) flushCatalogLocked() error {
	path := s.catalogPath()
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	_ = w.Write(catalogCSVHeader())
	for _, t := range s.catalog {
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

func (s *polarsStore) UpsertCatalogTitles(_ context.Context, titles []CatalogTitle) (CatalogUpsertStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := CatalogUpsertStats{}
	for _, raw := range titles {
		incoming, err := NormalizeCatalogTitle(raw)
		if err != nil {
			return stats, err
		}
		idx := findCatalogIndex(s.catalog, incoming.MediaType, incoming.TMDBID, incoming.IMDBID)
		if idx < 0 {
			incoming.ID = newCatalogID(incoming)
			incoming.Seasons = cloneSeasons(incoming.Seasons)
			incoming.SourceInstances = cloneStrings(incoming.SourceInstances)
			s.catalog = append(s.catalog, incoming)
			stats.Added++
			continue
		}
		merged := MergeCatalogTitle(s.catalog[idx], incoming)
		merged.ID = s.catalog[idx].ID
		s.catalog[idx] = merged
		stats.Updated++
	}
	stats.Total = len(s.catalog)
	if err := s.flushCatalogLocked(); err != nil {
		return stats, err
	}
	return stats, nil
}

func newCatalogID(t CatalogTitle) string {
	return CatalogIdentityKey(t.MediaType, t.TMDBID, t.IMDBID)
}

func findCatalogIndex(rows []CatalogTitle, mediaType string, tmdbID int, imdbID string) int {
	imdbID = strings.TrimSpace(imdbID)
	for i, row := range rows {
		if row.MediaType != mediaType {
			continue
		}
		if tmdbID > 0 && row.TMDBID == tmdbID {
			return i
		}
		if imdbID != "" && row.IMDBID == imdbID {
			return i
		}
	}
	return -1
}

func (s *polarsStore) ListCatalogTitles(_ context.Context, filter CatalogFilter) ([]CatalogTitle, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	matched := make([]CatalogTitle, 0, len(s.catalog))
	q := strings.ToLower(strings.TrimSpace(filter.Query))
	for _, row := range s.catalog {
		if filter.MediaType != "" && row.MediaType != filter.MediaType {
			continue
		}
		if filter.Watched != nil && row.Watched != *filter.Watched {
			continue
		}
		if q != "" {
			hay := strings.ToLower(row.Title + " " + row.IMDBID + " " + strconv.Itoa(row.TMDBID))
			if !strings.Contains(hay, q) {
				continue
			}
		}
		matched = append(matched, cloneCatalogTitle(row))
	}
	total := len(matched)
	limit := ClampCatalogLimit(filter.Limit)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []CatalogTitle{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return matched[offset:end], total, nil
}

func (s *polarsStore) GetCatalogTitle(_ context.Context, id string) (CatalogTitle, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	for _, row := range s.catalog {
		if row.ID == id {
			return cloneCatalogTitle(row), true, nil
		}
	}
	return CatalogTitle{}, false, nil
}

func (s *polarsStore) ApplyCatalogWatched(_ context.Context, patches []CatalogWatchedPatch) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	updated := 0
	for _, patch := range patches {
		media := strings.ToLower(strings.TrimSpace(patch.MediaType))
		idx := findCatalogIndex(s.catalog, media, patch.TMDBID, patch.IMDBID)
		if idx < 0 {
			continue
		}
		row := s.catalog[idx]
		row.Watched = patch.Watched
		row.WatchedAt = patch.WatchedAt
		if patch.PlexRatingKey != "" {
			row.PlexRatingKey = patch.PlexRatingKey
		}
		row.UpdatedAt = time.Now().UTC()
		s.catalog[idx] = row
		updated++
	}
	if err := s.flushCatalogLocked(); err != nil {
		return updated, err
	}
	return updated, nil
}

func cloneCatalogTitle(in CatalogTitle) CatalogTitle {
	out := in
	out.Seasons = cloneSeasons(in.Seasons)
	out.SourceInstances = cloneStrings(in.SourceInstances)
	if in.WatchedAt != nil {
		t := *in.WatchedAt
		out.WatchedAt = &t
	}
	return out
}

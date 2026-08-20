package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/plex"
	"github.com/bogartusmaximus/listarr-go/internal/store"
)

// Service ingests *arr libraries and Plex watched state into the SoT catalog.
type Service struct {
	Store store.Store
	Arr   *arr.Registry
	Plex  *plex.Client
}

// IngestRequest pulls titles from a named *arr instance into the catalog.
type IngestRequest struct {
	SourceInstance string            `json:"sourceInstance"`
	MediaType      string            `json:"mediaType"` // movie|tv
	Filter         arr.LibraryFilter `json:"sourceFilter"`
	MaxItems       int               `json:"maxItems,omitempty"` // 0 = ingest all (up to IngestHardCap)
}

const IngestHardCap = 20000

// IngestResult summarizes an ingest run.
type IngestResult struct {
	SourceInstance string                   `json:"sourceInstance"`
	MediaType      string                   `json:"mediaType"`
	Available      int                      `json:"available"`
	Fetched        int                      `json:"fetched"`
	Truncated      bool                     `json:"truncated,omitempty"`
	Upsert         store.CatalogUpsertStats `json:"upsert"`
}

// IngestFromArr exports a Radarr/Sonarr library into the listarr-go catalog.
func (s *Service) IngestFromArr(ctx context.Context, req IngestRequest) (IngestResult, error) {
	if s == nil || s.Store == nil {
		return IngestResult{}, fmt.Errorf("catalog store is not configured")
	}
	if s.Arr == nil {
		return IngestResult{}, fmt.Errorf("arr registry is not configured")
	}
	req.MediaType = strings.ToLower(strings.TrimSpace(req.MediaType))
	req.SourceInstance = strings.TrimSpace(req.SourceInstance)
	if req.SourceInstance == "" {
		return IngestResult{}, fmt.Errorf("sourceInstance is required")
	}
	if req.MediaType != store.CatalogMovie && req.MediaType != store.CatalogTV {
		return IngestResult{}, fmt.Errorf("mediaType must be movie or tv")
	}
	titles, available, err := s.fetchArrTitles(ctx, req)
	if err != nil {
		return IngestResult{}, err
	}
	stats, err := s.Store.UpsertCatalogTitles(ctx, titles)
	if err != nil {
		return IngestResult{}, err
	}
	return IngestResult{
		SourceInstance: req.SourceInstance,
		MediaType:      req.MediaType,
		Available:      available,
		Fetched:        len(titles),
		Truncated:      available > len(titles),
		Upsert:         stats,
	}, nil
}

func ingestCap(maxItems int) int {
	if maxItems <= 0 {
		return IngestHardCap
	}
	if maxItems > IngestHardCap {
		return IngestHardCap
	}
	return maxItems
}

func (s *Service) fetchArrTitles(ctx context.Context, req IngestRequest) ([]store.CatalogTitle, int, error) {
	max := ingestCap(req.MaxItems)
	switch req.MediaType {
	case store.CatalogMovie:
		src, err := s.Arr.Radarr(req.SourceInstance)
		if err != nil {
			return nil, 0, err
		}
		rows, err := src.ExportMovies(ctx, req.Filter)
		if err != nil {
			return nil, 0, err
		}
		available := len(rows)
		if available > max {
			rows = rows[:max]
		}
		titles := make([]store.CatalogTitle, 0, len(rows))
		for _, row := range rows {
			titles = append(titles, movieToCatalog(row, req.SourceInstance))
		}
		return titles, available, nil
	case store.CatalogTV:
		src, err := s.Arr.Sonarr(req.SourceInstance)
		if err != nil {
			return nil, 0, err
		}
		rows, err := src.ExportSeries(ctx, req.Filter)
		if err != nil {
			return nil, 0, err
		}
		available := len(rows)
		if available > max {
			rows = rows[:max]
		}
		titles := make([]store.CatalogTitle, 0, len(rows))
		for _, row := range rows {
			titles = append(titles, seriesToCatalog(row, req.SourceInstance))
		}
		return titles, available, nil
	default:
		return nil, 0, fmt.Errorf("mediaType must be movie or tv")
	}
}

func movieToCatalog(row arr.MovieRef, source string) store.CatalogTitle {
	t := store.CatalogTitle{
		MediaType:       store.CatalogMovie,
		TMDBID:          row.TMDBID,
		IMDBID:          strings.TrimSpace(row.IMDBID),
		Title:           row.Title,
		Year:            row.Year,
		Overview:        row.Overview,
		Path:            row.Path,
		Monitored:       row.Monitored,
		SourceInstances: []string{source},
		Seasons:         []store.CatalogSeason{},
	}
	if row.Collection != nil {
		t.CollectionTMDBID = row.Collection.TMDBID
		t.CollectionName = row.Collection.Title
	}
	return t
}

func seriesToCatalog(row arr.SeriesRef, source string) store.CatalogTitle {
	seasons := make([]store.CatalogSeason, 0, len(row.Seasons))
	for _, season := range row.Seasons {
		cs := store.CatalogSeason{
			SeasonNumber: season.SeasonNumber,
			Monitored:    season.Monitored,
		}
		if season.Statistics != nil {
			cs.EpisodeCount = season.Statistics.EpisodeCount
		}
		seasons = append(seasons, cs)
	}
	return store.CatalogTitle{
		MediaType:       store.CatalogTV,
		TMDBID:          row.TMDBID,
		IMDBID:          strings.TrimSpace(row.IMDBID),
		Title:           row.Title,
		Year:            row.Year,
		Overview:        row.Overview,
		Path:            row.Path,
		Monitored:       row.Monitored,
		Seasons:         seasons,
		SourceInstances: []string{source},
	}
}

// WatchedSyncResult summarizes a Plex watched pull.
type WatchedSyncResult struct {
	Fetched int `json:"fetched"`
	Updated int `json:"updated"`
}

// SyncWatchedFromPlex matches Plex viewCount against catalog titles by TMDB/IMDB.
func (s *Service) SyncWatchedFromPlex(ctx context.Context, mediaType string) (WatchedSyncResult, error) {
	if s == nil || s.Store == nil {
		return WatchedSyncResult{}, fmt.Errorf("catalog store is not configured")
	}
	if s.Plex == nil {
		return WatchedSyncResult{}, fmt.Errorf("plex client is not configured")
	}
	items, err := s.Plex.ListWatched(ctx, mediaType, false)
	if err != nil {
		return WatchedSyncResult{}, err
	}
	patches := make([]store.CatalogWatchedPatch, 0, len(items))
	for _, item := range items {
		media := store.CatalogMovie
		if item.Type == "show" {
			media = store.CatalogTV
		}
		patches = append(patches, store.CatalogWatchedPatch{
			TMDBID:        item.TMDBID,
			IMDBID:        item.IMDBID,
			MediaType:     media,
			Watched:       item.ViewCount > 0,
			WatchedAt:     item.LastViewedAt,
			PlexRatingKey: item.RatingKey,
		})
	}
	updated, err := s.Store.ApplyCatalogWatched(ctx, patches)
	if err != nil {
		return WatchedSyncResult{}, err
	}
	return WatchedSyncResult{Fetched: len(items), Updated: updated}, nil
}

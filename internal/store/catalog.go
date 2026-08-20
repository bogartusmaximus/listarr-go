package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Catalog media kinds stored in the listarr-go source of truth.
const (
	CatalogMovie = "movie"
	CatalogTV    = "tv"
)

// CatalogSeason is one season on a TV title.
type CatalogSeason struct {
	SeasonNumber int  `json:"seasonNumber"`
	Monitored    bool `json:"monitored"`
	EpisodeCount int  `json:"episodeCount,omitempty"`
	Watched      bool `json:"watched,omitempty"`
}

// CatalogTitle is one unique movie or show in the listarr-go SoT.
// Uniqueness is by (mediaType,tmdbId) and/or (mediaType,imdbId).
type CatalogTitle struct {
	ID                string          `json:"id"`
	MediaType         string          `json:"mediaType"` // movie|tv
	TMDBID            int             `json:"tmdbId,omitempty"`
	IMDBID            string          `json:"imdbId,omitempty"`
	Title             string          `json:"title"`
	Year              int             `json:"year,omitempty"`
	Overview          string          `json:"overview,omitempty"`
	Path              string          `json:"path,omitempty"`
	Monitored         bool            `json:"monitored"`
	CollectionTMDBID  int             `json:"collectionTmdbId,omitempty"`
	CollectionName    string          `json:"collectionName,omitempty"`
	Seasons           []CatalogSeason `json:"seasons,omitempty"`
	Watched           bool            `json:"watched"`
	WatchedAt         *time.Time      `json:"watchedAt,omitempty"`
	PlexRatingKey     string          `json:"plexRatingKey,omitempty"`
	SourceInstances   []string        `json:"sourceInstances,omitempty"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

// CatalogFilter selects titles for browse / sync export.
type CatalogFilter struct {
	MediaType string // movie|tv|"" (all)
	Query     string
	Watched   *bool
	Limit     int
	Offset    int
}

// CatalogUpsertStats summarizes an ingest write.
type CatalogUpsertStats struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Total   int `json:"total"`
}

// CatalogWatchedPatch updates watched flags (from Plex).
type CatalogWatchedPatch struct {
	TMDBID        int        `json:"tmdbId,omitempty"`
	IMDBID        string     `json:"imdbId,omitempty"`
	MediaType     string     `json:"mediaType"`
	Watched       bool       `json:"watched"`
	WatchedAt     *time.Time `json:"watchedAt,omitempty"`
	PlexRatingKey string     `json:"plexRatingKey,omitempty"`
}

// CatalogStore is the listarr-go media SoT surface (postgres primary, polars cache).
type CatalogStore interface {
	UpsertCatalogTitles(ctx context.Context, titles []CatalogTitle) (CatalogUpsertStats, error)
	ListCatalogTitles(ctx context.Context, filter CatalogFilter) ([]CatalogTitle, int, error)
	GetCatalogTitle(ctx context.Context, id string) (CatalogTitle, bool, error)
	ApplyCatalogWatched(ctx context.Context, patches []CatalogWatchedPatch) (int, error)
}

// NormalizeCatalogTitle trims IDs and drops empty seasons.
func NormalizeCatalogTitle(t CatalogTitle) (CatalogTitle, error) {
	t.MediaType = strings.ToLower(strings.TrimSpace(t.MediaType))
	if t.MediaType != CatalogMovie && t.MediaType != CatalogTV {
		return CatalogTitle{}, fmt.Errorf("mediaType must be movie or tv")
	}
	t.IMDBID = strings.TrimSpace(t.IMDBID)
	t.Title = strings.TrimSpace(t.Title)
	t.Path = strings.TrimSpace(t.Path)
	t.Overview = strings.TrimSpace(t.Overview)
	t.CollectionName = strings.TrimSpace(t.CollectionName)
	t.PlexRatingKey = strings.TrimSpace(t.PlexRatingKey)
	if t.TMDBID < 1 && t.IMDBID == "" {
		return CatalogTitle{}, fmt.Errorf("tmdbId or imdbId is required")
	}
	if t.Title == "" {
		t.Title = fallbackTitle(t)
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = time.Now().UTC()
	}
	if t.Seasons == nil {
		t.Seasons = []CatalogSeason{}
	}
	if t.SourceInstances == nil {
		t.SourceInstances = []string{}
	}
	return t, nil
}

func fallbackTitle(t CatalogTitle) string {
	if t.TMDBID > 0 {
		return fmt.Sprintf("tmdb:%d", t.TMDBID)
	}
	return t.IMDBID
}

// CatalogIdentityKey prefers TMDB, else IMDB, for stable map keys.
func CatalogIdentityKey(mediaType string, tmdbID int, imdbID string) string {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	imdbID = strings.TrimSpace(imdbID)
	if tmdbID > 0 {
		return fmt.Sprintf("%s:tmdb:%d", mediaType, tmdbID)
	}
	return fmt.Sprintf("%s:imdb:%s", mediaType, imdbID)
}

func clampCatalogLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 2000 {
		return 2000
	}
	return limit
}

func cloneSeasons(in []CatalogSeason) []CatalogSeason {
	if len(in) == 0 {
		return []CatalogSeason{}
	}
	out := make([]CatalogSeason, len(in))
	copy(out, in)
	return out
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func mergeSourceInstances(existing, incoming []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(existing)+len(incoming))
	for _, name := range existing {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, name := range incoming {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// MergeCatalogTitle overlays ingest fields onto an existing row (preserves watched).
func MergeCatalogTitle(existing, incoming CatalogTitle) CatalogTitle {
	out := existing
	if incoming.TMDBID > 0 {
		out.TMDBID = incoming.TMDBID
	}
	if incoming.IMDBID != "" {
		out.IMDBID = incoming.IMDBID
	}
	if incoming.Title != "" {
		out.Title = incoming.Title
	}
	if incoming.Year > 0 {
		out.Year = incoming.Year
	}
	if incoming.Overview != "" {
		out.Overview = incoming.Overview
	}
	if incoming.Path != "" {
		out.Path = incoming.Path
	}
	out.Monitored = incoming.Monitored
	if incoming.CollectionTMDBID > 0 || incoming.CollectionName != "" {
		out.CollectionTMDBID = incoming.CollectionTMDBID
		out.CollectionName = incoming.CollectionName
	}
	if len(incoming.Seasons) > 0 {
		out.Seasons = cloneSeasons(incoming.Seasons)
	}
	out.SourceInstances = mergeSourceInstances(existing.SourceInstances, incoming.SourceInstances)
	out.UpdatedAt = time.Now().UTC()
	return out
}

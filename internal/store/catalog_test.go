package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/store"
)

func TestPolarsCatalogUpsertUniqueAndWatched(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	stats, err := s.UpsertCatalogTitles(context.Background(), []store.CatalogTitle{
		{
			MediaType: store.CatalogMovie,
			TMDBID:    550,
			IMDBID:    "tt0137523",
			Title:     "Fight Club",
			Year:      1999,
			Monitored: true,
			CollectionTMDBID: 10,
			CollectionName:   "Fight Club Collection",
			SourceInstances:  []string{"local"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 1 || stats.Total != 1 {
		t.Fatalf("upsert %#v", stats)
	}

	// Same TMDB → update, not duplicate.
	stats, err = s.UpsertCatalogTitles(context.Background(), []store.CatalogTitle{
		{
			MediaType:       store.CatalogMovie,
			TMDBID:          550,
			Title:           "Fight Club (remaster)",
			SourceInstances: []string{"remote"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Updated != 1 || stats.Added != 0 || stats.Total != 1 {
		t.Fatalf("second upsert %#v", stats)
	}

	titles, total, err := s.ListCatalogTitles(context.Background(), store.CatalogFilter{MediaType: "movie", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(titles) != 1 {
		t.Fatalf("list total=%d len=%d", total, len(titles))
	}
	if titles[0].Title != "Fight Club (remaster)" {
		t.Fatalf("title=%q", titles[0].Title)
	}
	if len(titles[0].SourceInstances) != 2 {
		t.Fatalf("sources=%v", titles[0].SourceInstances)
	}
	if titles[0].CollectionName != "Fight Club Collection" {
		t.Fatalf("collection not preserved: %#v", titles[0])
	}

	watchedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	n, err := s.ApplyCatalogWatched(context.Background(), []store.CatalogWatchedPatch{{
		MediaType: store.CatalogMovie,
		TMDBID:    550,
		Watched:   true,
		WatchedAt: &watchedAt,
	}})
	if err != nil || n != 1 {
		t.Fatalf("watched n=%d err=%v", n, err)
	}
	want := true
	titles, _, err = s.ListCatalogTitles(context.Background(), store.CatalogFilter{Watched: &want, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 1 || !titles[0].Watched {
		t.Fatalf("watched list %#v", titles)
	}

	csvPath := filepath.Join(dir, "catalog_titles.csv")
	raw, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 40 {
		t.Fatalf("catalog csv too short: %q", raw)
	}
}

func TestPolarsCatalogTVSeasons(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	_, err = s.UpsertCatalogTitles(context.Background(), []store.CatalogTitle{{
		MediaType: store.CatalogTV,
		TMDBID:    1396,
		IMDBID:    "tt0903747",
		Title:     "Breaking Bad",
		Seasons: []store.CatalogSeason{
			{SeasonNumber: 1, Monitored: true, EpisodeCount: 7},
			{SeasonNumber: 2, Monitored: true, EpisodeCount: 13},
		},
		SourceInstances: []string{"sonarr-local"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := s.GetCatalogTitle(context.Background(), "tv:tmdb:1396")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if len(got.Seasons) != 2 || got.Seasons[1].EpisodeCount != 13 {
		t.Fatalf("seasons %#v", got.Seasons)
	}
}

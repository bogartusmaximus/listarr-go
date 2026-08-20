package catalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/catalog"
	"github.com/bogartusmaximus/listarr-go/internal/store"
)

func TestIngestFromArrMovies(t *testing.T) {
	radarrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"title": "Fight Club", "tmdbId": 550, "imdbId": "tt0137523", "year": 1999,
				"monitored": true, "path": "/movies/Fight Club",
				"collection": map[string]any{"tmdbId": 10, "title": "Fight Club Collection"},
			},
			{
				"title": "No TMDB", "tmdbId": 0, "imdbId": "tt0000001", "year": 1900, "monitored": true,
			},
		})
	}))
	t.Cleanup(radarrSrv.Close)

	radarr, err := arr.NewRadarr(radarrSrv.URL, "k", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	reg := arr.NewRegistry()
	_ = reg.RegisterRadarr("local", radarr)

	dir := t.TempDir()
	st, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	svc := catalog.Service{Store: st, Arr: reg}
	res, err := svc.IngestFromArr(context.Background(), catalog.IngestRequest{
		SourceInstance: "local",
		MediaType:      "movie",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 2 || res.Available != 2 || res.Upsert.Added != 2 {
		t.Fatalf("%+v", res)
	}
	titles, total, err := st.ListCatalogTitles(context.Background(), store.CatalogFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(titles) != 2 {
		t.Fatalf("total=%d titles=%d", total, len(titles))
	}
}

func TestIngestMaxItemsCaps(t *testing.T) {
	radarrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows := make([]map[string]any, 0, 5)
		for i := 1; i <= 5; i++ {
			rows = append(rows, map[string]any{"title": "M", "tmdbId": i, "monitored": true})
		}
		_ = json.NewEncoder(w).Encode(rows)
	}))
	t.Cleanup(radarrSrv.Close)
	radarr, err := arr.NewRadarr(radarrSrv.URL, "k", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	reg := arr.NewRegistry()
	_ = reg.RegisterRadarr("local", radarr)
	dir := t.TempDir()
	st, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := catalog.Service{Store: st, Arr: reg}
	res, err := svc.IngestFromArr(context.Background(), catalog.IngestRequest{
		SourceInstance: "local",
		MediaType:      "movie",
		MaxItems:       2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Available != 5 || res.Fetched != 2 || !res.Truncated {
		t.Fatalf("%+v", res)
	}
}

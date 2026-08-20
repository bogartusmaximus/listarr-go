package plex_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/httpx"
	"github.com/bogartusmaximus/listarr-go/internal/plex"
)

func TestListWatchedParsesGuids(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/library/sections", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[{"key":"1","type":"movie"}]}}`))
	})
	mux.HandleFunc("/library/sections/1/all", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("X-Plex-Container-Start") != "0" {
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{
			"ratingKey":"99","title":"Fight Club","type":"movie","viewCount":2,"lastViewedAt":1700000000,
			"Guid":[{"id":"imdb://tt0137523"},{"id":"tmdb://550"}]
		}]}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := plex.New(srv.URL, "token", "client-id", httpx.New(0))
	if err != nil {
		t.Fatal(err)
	}
	items, err := client.ListWatched(context.Background(), "movie", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("%+v", items)
	}
	if items[0].TMDBID != 550 || items[0].IMDBID != "tt0137523" || items[0].ViewCount != 2 {
		t.Fatalf("%+v", items[0])
	}
}

package tmdb_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/httpx"
	"github.com/bogartusmaximus/listarr-go/internal/tmdb"
)

func TestDiscoverMovies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/discover/movie" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("api_key") == "" {
			t.Fatal("missing api_key")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":550,"title":"Fight Club","release_date":"1999-10-15","vote_average":8.4,"overview":"x"}]}`))
	}))
	t.Cleanup(srv.Close)

	c, err := tmdb.New("test-key", srv.URL, httpx.New(0))
	if err != nil {
		t.Fatal(err)
	}
	items, err := c.DiscoverMovies(context.Background(), tmdb.DiscoverQuery{Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].TMDBID != 550 || items[0].Year != 1999 {
		t.Fatalf("%+v", items)
	}
}

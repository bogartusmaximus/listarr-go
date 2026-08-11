package arr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
)

func TestRadarrListAndAdd(t *testing.T) {
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "radarr-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/movie":
			_, _ = w.Write([]byte(`[{"title":"Existing","tmdbId":100}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/movie/lookup":
			_, _ = w.Write([]byte(`[{"title":"New","tmdbId":550,"year":1999}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/movie":
			_ = json.NewDecoder(r.Body).Decode(&posted)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := arr.NewRadarr(srv.URL, "radarr-key", nil)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := client.ListMovies(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := existing[100]; !ok {
		t.Fatalf("missing existing: %+v", existing)
	}
	lookup, err := client.LookupByTMDB(context.Background(), 550)
	if err != nil {
		t.Fatal(err)
	}
	err = client.AddMovie(context.Background(), lookup, arr.Target{
		RootFolderPath:   "/data/movies",
		QualityProfileID: 1,
		Monitored:        true,
		SearchOnAdd:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	opts, _ := posted["addOptions"].(map[string]any)
	if opts["searchForMovie"] != true {
		t.Fatalf("posted=%+v", posted)
	}
}

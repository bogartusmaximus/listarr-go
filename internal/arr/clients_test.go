package arr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
)

func TestConnectionSendsAuthCookie(t *testing.T) {
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		if r.Header.Get("X-Api-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"appName":"Radarr","version":"5.0.0"}`))
	}))
	t.Cleanup(srv.Close)

	cookie := `_oauth2_proxy=example-token|123|sig`
	got, err := arr.TestConnection(context.Background(), arr.KindRadarr, srv.URL, "k", cookie, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.OK {
		t.Fatalf("%+v", got)
	}
	if gotCookie != cookie {
		t.Fatalf("cookie=%q", gotCookie)
	}
}

func TestConnectionOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "k" || r.URL.Path != "/api/v3/system/status" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"appName":"Radarr","version":"5.0.0"}`))
	}))
	t.Cleanup(srv.Close)

	got, err := arr.TestConnection(context.Background(), arr.KindRadarr, srv.URL, "k", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.AppName != "Radarr" || got.Version != "5.0.0" {
		t.Fatalf("%+v", got)
	}
}

func TestConnectionBadKind(t *testing.T) {
	_, err := arr.TestConnection(context.Background(), arr.Kind("lidarr"), "http://127.0.0.1:7878", "k", "", nil)
	if err == nil {
		t.Fatal("expected kind error")
	}
}

func TestRadarrListAndAdd(t *testing.T) {
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "radarr-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/movie":
			_, _ = w.Write([]byte(`[{"title":"Existing","tmdbId":100,"monitored":true,"tags":[1]}]`))
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

	client, err := arr.NewRadarr(srv.URL, "radarr-key", "", nil)
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

func TestRadarrListRootsAndProfiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/rootfolder":
			_, _ = w.Write([]byte(`[{"id":1,"path":"/data/movies","accessible":true}]`))
		case "/api/v3/qualityprofile":
			_, _ = w.Write([]byte(`[{"id":6,"name":"HD-1080p"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := arr.NewRadarr(srv.URL, "k", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	roots, err := client.ListRootFolders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0].Path != "/data/movies" {
		t.Fatalf("roots=%+v", roots)
	}
	profiles, err := client.ListQualityProfiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ID != 6 || profiles[0].Name != "HD-1080p" {
		t.Fatalf("profiles=%+v", profiles)
	}
}

func TestExportMoviesFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"title":"A","tmdbId":1,"monitored":true,"tags":[2],"path":"/movies/A"},
			{"title":"B","tmdbId":2,"monitored":true,"tags":[3],"path":"/movies/B"}
		]`))
	}))
	t.Cleanup(srv.Close)
	client, _ := arr.NewRadarr(srv.URL, "k", "", nil)
	rows, err := client.ExportMovies(context.Background(), arr.LibraryFilter{TagIDs: []int{2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TMDBID != 1 {
		t.Fatalf("%+v", rows)
	}
}

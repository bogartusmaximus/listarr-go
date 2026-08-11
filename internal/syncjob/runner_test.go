package syncjob_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/httpx"
	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
	"github.com/bogartusmaximus/listarr-go/internal/syncjob"
	"github.com/bogartusmaximus/listarr-go/internal/tmdb"
)

func TestPreviewSkipsExistingAndDoesNotConsumeBudget(t *testing.T) {
	tmdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[
			{"id":100,"title":"Existing","release_date":"2020-01-01","vote_average":7},
			{"id":200,"title":"New","release_date":"2021-01-01","vote_average":8}
		]}`))
	}))
	t.Cleanup(tmdbSrv.Close)

	radarrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"title":"Existing","tmdbId":100,"monitored":true}]`))
	}))
	t.Cleanup(radarrSrv.Close)

	tmdbClient, err := tmdb.New("k", tmdbSrv.URL, httpx.New(0))
	if err != nil {
		t.Fatal(err)
	}
	radarr, err := arr.NewRadarr(radarrSrv.URL, "k", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	reg := arr.NewRegistry()
	_ = reg.RegisterRadarr("radarr", radarr)
	budget := ratelimit.NewHourlyBudget(1)
	runner := syncjob.Runner{Deps: syncjob.Dependencies{
		TMDB: tmdbClient, Arr: reg, SearchBudget: budget,
	}}

	res, err := runner.Run(context.Background(), syncjob.Request{
		Source:    "tmdb",
		MediaType: "movie",
		Discover:  &tmdb.DiscoverQuery{Page: 1},
		Target: arr.Target{
			RootFolderPath:   "/data/movies",
			QualityProfileID: 1,
			Monitored:        true,
			SearchOnAdd:      true,
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Skips != 1 || res.Adds != 1 {
		t.Fatalf("%+v", res)
	}
	if budget.Remaining() != 1 {
		t.Fatalf("preview consumed budget: remaining=%d", budget.Remaining())
	}
}

func TestApplyRespectsSearchBudget(t *testing.T) {
	var posts int
	radarrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/movie":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/movie/lookup":
			_, _ = w.Write([]byte(`[{"title":"X","tmdbId":1}]`))
		case r.Method == http.MethodPost:
			posts++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			opts, _ := body["addOptions"].(map[string]any)
			if posts == 1 && opts["searchForMovie"] != true {
				t.Fatalf("first should search: %+v", body)
			}
			if posts == 2 && opts["searchForMovie"] != false {
				t.Fatalf("second should defer search: %+v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(radarrSrv.Close)

	radarr, err := arr.NewRadarr(radarrSrv.URL, "k", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	reg := arr.NewRegistry()
	_ = reg.RegisterRadarr("radarr", radarr)
	budget := ratelimit.NewHourlyBudget(1)
	runner := syncjob.Runner{Deps: syncjob.Dependencies{Arr: reg, SearchBudget: budget}}
	res, err := runner.Run(context.Background(), syncjob.Request{
		Source:    "tmdb",
		MediaType: "movie",
		TMDBIDs:   []int{1, 2},
		Target: arr.Target{
			RootFolderPath:   "/data/movies",
			QualityProfileID: 1,
			Monitored:        true,
			SearchOnAdd:      true,
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Adds != 2 || res.Deferred != 1 || posts != 2 {
		t.Fatalf("res=%+v posts=%d", res, posts)
	}
}

func TestArrLibraryDualInstancePreview(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"title":"A","tmdbId":1,"monitored":true,"tags":[9],"path":"/data/movies/A"},
			{"title":"B","tmdbId":2,"monitored":false,"tags":[],"path":"/data/movies/B"},
			{"title":"C","tmdbId":3,"monitored":true,"tags":[9],"path":"/data/movies/C"}
		]`))
	}))
	t.Cleanup(local.Close)

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"title":"A","tmdbId":1,"monitored":true}]`))
	}))
	t.Cleanup(remote.Close)

	localClient, _ := arr.NewRadarr(local.URL, "k", "", nil)
	remoteClient, _ := arr.NewRadarr(remote.URL, "k", "", nil)
	reg := arr.NewRegistry()
	_ = reg.RegisterRadarr("local", localClient)
	_ = reg.RegisterRadarr("remote", remoteClient)

	runner := syncjob.Runner{Deps: syncjob.Dependencies{Arr: reg}}
	res, err := runner.Run(context.Background(), syncjob.Request{
		Source:         "arr-library",
		MediaType:      "movie",
		SourceInstance: "local",
		SourceFilter:   arr.LibraryFilter{MonitoredOnly: true, TagIDs: []int{9}},
		Target: arr.Target{
			Instance:         "remote",
			RootFolderPath:   "/data/movies",
			QualityProfileID: 1,
			Monitored:        true,
			SearchOnAdd:      false,
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	// local monitored+tag9 => A,C; remote already has A => skip A, add C
	if res.Skips != 1 || res.Adds != 1 {
		t.Fatalf("%+v", res)
	}
}

package jobs

import (
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/store"
	"github.com/bogartusmaximus/listarr-go/internal/syncjob"
)

func TestMatchSyncRoute(t *testing.T) {
	routes := []store.SyncRoute{
		{Name: "movies", Source: "listarr-go", MediaType: "movie", TargetInstance: "radarr-remote", AllowApply: true},
		{Name: "tv-dry", Source: "arr-library", MediaType: "tv", SourceInstance: "sonarr-local", TargetInstance: "sonarr-remote", AllowApply: false},
	}
	got, ok := MatchSyncRoute(routes, syncjob.Request{
		Source:    "listarr-go",
		MediaType: "movie",
		Target:    arr.Target{Instance: "radarr-remote"},
	})
	if !ok || !got.AllowApply {
		t.Fatalf("movie route: ok=%v allow=%v", ok, got.AllowApply)
	}
	got, ok = MatchSyncRoute(routes, syncjob.Request{
		Source:         "arr-library",
		MediaType:      "tv",
		SourceInstance: "sonarr-local",
		Target:         arr.Target{Instance: "sonarr-remote"},
	})
	if !ok || got.AllowApply {
		t.Fatalf("tv route should deny apply: ok=%v allow=%v", ok, got.AllowApply)
	}
	_, ok = MatchSyncRoute(routes, syncjob.Request{
		Source:    "tmdb",
		MediaType: "movie",
		Target:    arr.Target{Instance: "radarr-remote"},
	})
	if ok {
		t.Fatal("expected no match for tmdb")
	}
}

func TestResolveAllowApplyRouteWins(t *testing.T) {
	settings := store.Settings{
		SafeMode: true,
		SyncRoutes: []store.SyncRoute{
			{Source: "listarr-go", MediaType: "movie", TargetInstance: "radarr", AllowApply: true},
		},
	}
	allow := ResolveAllowApply(settings, syncjob.Request{
		Source:    "listarr-go",
		MediaType: "movie",
		Target:    arr.Target{Instance: "radarr"},
	}, false, true)
	if !allow {
		t.Fatal("route AllowApply should win over job=false and safeMode")
	}
	allow = ResolveAllowApply(settings, syncjob.Request{
		Source:    "tmdb",
		MediaType: "movie",
		Target:    arr.Target{Instance: "radarr"},
	}, true, true)
	if !allow {
		t.Fatal("schedule jobAllowApply should apply when no route")
	}
}

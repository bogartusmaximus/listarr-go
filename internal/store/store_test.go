package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/store"
)

func TestPolarsSaveAndList(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	err = s.SaveSyncRun(context.Background(), store.SyncRun{
		DryRun:         true,
		Source:         "arr-library",
		MediaType:      "movie",
		SourceInstance: "local",
		TargetInstance: "remote",
		Adds:           2,
		Skips:          1,
		CreatedAt:      time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	runs, err := s.ListSyncRuns(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Adds != 2 || runs[0].SourceInstance != "local" {
		t.Fatalf("%+v", runs)
	}
	csvPath := filepath.Join(dir, "sync_runs.csv")
	raw, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 20 {
		t.Fatalf("csv too short: %q", raw)
	}
}

func TestOpenStubs(t *testing.T) {
	if _, err := store.Open(store.Config{Backend: store.BackendSQLite}); err == nil {
		t.Fatal("expected sqlite stub error")
	}
	if _, err := store.Open(store.Config{Backend: store.BackendMySQL}); err == nil {
		t.Fatal("expected mysql stub error")
	}
}

func TestPostgresRequiresURL(t *testing.T) {
	if _, err := store.Open(store.Config{Backend: store.BackendPostgres}); err == nil {
		t.Fatal("expected missing url error")
	}
}

func TestPolarsSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	_, found, err := s.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected no settings yet")
	}

	want := store.Settings{
		APIKey:              "k",
		InstanceName:        "listarr",
		ApplyEnabled:        true,
		TorboxSearchPerHour: 30,
		TMDBAPIKey:          "tmdb",
		ArrInstances: []store.ArrInstanceSettings{
			{Name: "local", Kind: "radarr", URL: "http://127.0.0.1:7878", APIKey: "arr"},
		},
		UpdatedAt: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
	}
	if err := s.PutSettings(context.Background(), want); err != nil {
		t.Fatal(err)
	}

	got, found, err := s.GetSettings(context.Background())
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got.APIKey != want.APIKey || got.TMDBAPIKey != want.TMDBAPIKey || len(got.ArrInstances) != 1 {
		t.Fatalf("%+v", got)
	}
	if got.ArrInstances[0].URL != want.ArrInstances[0].URL {
		t.Fatalf("instance=%+v", got.ArrInstances[0])
	}

	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 20 {
		t.Fatalf("settings.json too short: %q", raw)
	}

	s2, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	got2, found, err := s2.GetSettings(context.Background())
	if err != nil || !found || got2.APIKey != "k" {
		t.Fatalf("reload found=%v got=%+v err=%v", found, got2, err)
	}
}

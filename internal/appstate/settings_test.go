package appstate_test

import (
	"context"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/appstate"
	"github.com/bogartusmaximus/listarr-go/internal/config"
	"github.com/bogartusmaximus/listarr-go/internal/store"
)

func TestLoadOrSeedPersistsOnce(t *testing.T) {
	st, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := config.Config{
		APIKey:              "seed-key",
		InstanceName:        "seeded",
		TorboxSearchPerHour: 60,
		TMDBAPIKey:          "tmdb",
	}
	first, err := appstate.LoadOrSeed(context.Background(), st, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if first.APIKey != "seed-key" || first.TMDBAPIKey != "tmdb" {
		t.Fatalf("%+v", first)
	}

	cfg.APIKey = "ignored-after-seed"
	cfg.TMDBAPIKey = "ignored"
	second, err := appstate.LoadOrSeed(context.Background(), st, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if second.APIKey != "seed-key" || second.TMDBAPIKey != "tmdb" {
		t.Fatalf("store should win: %+v", second)
	}
}

func TestLoadOrSeedRequiresAPIKeyWhenEmpty(t *testing.T) {
	st, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, err = appstate.LoadOrSeed(context.Background(), st, config.Config{
		InstanceName:        "x",
		TorboxSearchPerHour: 60,
	})
	if err == nil {
		t.Fatal("expected seed error without api key")
	}
}

func TestValidateRejectsEmptyAPIKey(t *testing.T) {
	err := appstate.Validate(store.Settings{
		InstanceName:        "x",
		TorboxSearchPerHour: 1,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

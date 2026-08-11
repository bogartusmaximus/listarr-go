package appstate_test

import (
	"context"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/appstate"
	"github.com/bogartusmaximus/listarr-go/internal/config"
	"github.com/bogartusmaximus/listarr-go/internal/store"
)

func TestGenerateAPIKey(t *testing.T) {
	a, err := appstate.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	b, err := appstate.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 32 {
		t.Fatalf("len=%d want 32 hex chars", len(a))
	}
	if a == b {
		t.Fatal("expected unique keys")
	}
}

func TestLoadOrSeedGeneratesAPIKey(t *testing.T) {
	st, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := config.Config{
		InstanceName:        "seeded",
		TorboxSearchPerHour: 60,
		TMDBAPIKey:          "tmdb",
	}
	first, generated, err := appstate.LoadOrSeed(context.Background(), st, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Fatal("expected generated api key on empty store")
	}
	if len(first.APIKey) != 32 || first.TMDBAPIKey != "tmdb" {
		t.Fatalf("%+v", first)
	}

	cfg.TMDBAPIKey = "ignored"
	second, generated, err := appstate.LoadOrSeed(context.Background(), st, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if generated {
		t.Fatal("should not regenerate when store has key")
	}
	if second.APIKey != first.APIKey || second.TMDBAPIKey != "tmdb" {
		t.Fatalf("store should win: %+v", second)
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

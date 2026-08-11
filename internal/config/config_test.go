package config_test

import (
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/config"
)

func TestLoadRequiresAPIKey(t *testing.T) {
	t.Setenv("LISTARR_API_KEY", "")
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when LISTARR_API_KEY empty")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("LISTARR_API_KEY", "test-key")
	t.Setenv("LISTARR_LISTEN", "")
	t.Setenv("LISTARR_INSTANCE_NAME", "")
	t.Setenv("LISTARR_APPLY", "")
	t.Setenv("LISTARR_TORBOX_SEARCH_PER_HOUR", "")
	t.Setenv("LISTARR_STORE_BACKEND", "")
	t.Setenv("LISTARR_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("LISTARR_POLARS_DIR", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != config.DefaultListen {
		t.Fatalf("listen=%q", cfg.Listen)
	}
	if cfg.ApplyEnabled {
		t.Fatal("apply must default off")
	}
	if cfg.TorboxSearchPerHour != config.DefaultTorboxSearchPerHr {
		t.Fatalf("perHour=%d", cfg.TorboxSearchPerHour)
	}
	if cfg.StoreBackend != config.DefaultStoreBackend {
		t.Fatalf("storeBackend=%q", cfg.StoreBackend)
	}
}

func TestLoadApplyOptIn(t *testing.T) {
	t.Setenv("LISTARR_API_KEY", "test-key")
	t.Setenv("LISTARR_APPLY", "1")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ApplyEnabled {
		t.Fatal("expected apply enabled")
	}
}

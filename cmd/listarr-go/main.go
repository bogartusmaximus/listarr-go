package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/api"
	"github.com/bogartusmaximus/listarr-go/internal/appstate"
	"github.com/bogartusmaximus/listarr-go/internal/config"
	"github.com/bogartusmaximus/listarr-go/internal/httpx"
	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
	"github.com/bogartusmaximus/listarr-go/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	st, err := store.Open(store.Config{
		Backend:     store.Backend(cfg.StoreBackend),
		DatabaseURL: cfg.DatabaseURL,
		PolarsDir:   cfg.PolarsDir,
	})
	if err != nil {
		slog.Error("store", "err", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	settings, generatedKey, err := appstate.LoadOrSeed(ctx, st, cfg)
	cancel()
	if err != nil {
		slog.Error("settings", "err", err)
		os.Exit(1)
	}
	if generatedKey {
		slog.Info("generated initial API key (view in Settings)")
	}

	budget := ratelimit.NewHourlyBudget(settings.TorboxSearchPerHour)
	httpClient := httpx.New(120 * time.Second)
	rt := &appstate.Runtime{
		Store:        st,
		HTTPClient:   httpClient,
		SearchBudget: budget,
	}
	if err := rt.Apply(settings); err != nil {
		slog.Error("apply settings", "err", err)
		os.Exit(1)
	}

	srv := api.New(rt)

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      180 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	view := rt.View()
	slog.Info("listarr-go listening",
		"addr", cfg.Listen,
		"instance", view.InstanceName,
		"safeMode", view.SafeMode,
		"torboxSearchPerHour", budget.Limit(),
		"storeBackend", st.Backend(),
		"tmdbConfigured", view.TMDB != nil,
		"arrInstances", view.Arr.Len(),
		"version", api.Version,
	)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("listen failed", "err", err)
		os.Exit(1)
	}
}

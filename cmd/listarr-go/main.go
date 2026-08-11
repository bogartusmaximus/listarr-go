package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/api"
	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/config"
	"github.com/bogartusmaximus/listarr-go/internal/httpx"
	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
	"github.com/bogartusmaximus/listarr-go/internal/syncjob"
	"github.com/bogartusmaximus/listarr-go/internal/tmdb"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	budget := ratelimit.NewHourlyBudget(cfg.TorboxSearchPerHour)
	httpClient := httpx.New(20 * time.Second)

	reg, err := arr.LoadRegistryFromEnv(httpClient)
	if err != nil {
		slog.Error("arr registry", "err", err)
		os.Exit(1)
	}

	deps := syncjob.Dependencies{Arr: reg, SearchBudget: budget}
	var tmdbClient *tmdb.Client
	if cfg.TMDBAPIKey != "" {
		tmdbClient, err = tmdb.New(cfg.TMDBAPIKey, "", httpClient)
		if err != nil {
			slog.Error("tmdb", "err", err)
			os.Exit(1)
		}
		deps.TMDB = tmdbClient
	}

	var runner *syncjob.Runner
	if deps.TMDB != nil || reg.Len() > 0 {
		runner = &syncjob.Runner{Deps: deps}
	}

	srv := api.New(api.Config{
		APIKey:       cfg.APIKey,
		InstanceName: cfg.InstanceName,
		ApplyEnabled: cfg.ApplyEnabled,
		SearchBudget: budget,
		Runner:       runner,
		TMDB:         tmdbClient,
		Arr:          reg,
	})

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("listarr-go listening",
		"addr", cfg.Listen,
		"instance", cfg.InstanceName,
		"applyEnabled", cfg.ApplyEnabled,
		"torboxSearchPerHour", cfg.TorboxSearchPerHour,
		"tmdbConfigured", tmdbClient != nil,
		"arrInstances", reg.Len(),
		"version", api.Version,
	)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("listen failed", "err", err)
		os.Exit(1)
	}
}

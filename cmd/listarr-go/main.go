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
	deps := syncjob.Dependencies{SearchBudget: budget}
	httpClient := httpx.New(20 * time.Second)

	var tmdbClient *tmdb.Client
	if cfg.TMDBAPIKey != "" {
		tmdbClient, err = tmdb.New(cfg.TMDBAPIKey, "", httpClient)
		if err != nil {
			slog.Error("tmdb", "err", err)
			os.Exit(1)
		}
		deps.TMDB = tmdbClient
	}
	if cfg.RadarrURL != "" && cfg.RadarrAPIKey != "" {
		radarr, err := arr.NewRadarr(cfg.RadarrURL, cfg.RadarrAPIKey, httpClient)
		if err != nil {
			slog.Error("radarr", "err", err)
			os.Exit(1)
		}
		deps.Radarr = radarr
	}
	if cfg.SonarrURL != "" && cfg.SonarrAPIKey != "" {
		sonarr, err := arr.NewSonarr(cfg.SonarrURL, cfg.SonarrAPIKey, httpClient)
		if err != nil {
			slog.Error("sonarr", "err", err)
			os.Exit(1)
		}
		deps.Sonarr = sonarr
	}

	var runner *syncjob.Runner
	if deps.TMDB != nil || deps.Radarr != nil || deps.Sonarr != nil {
		runner = &syncjob.Runner{Deps: deps}
	}

	srv := api.New(api.Config{
		APIKey:       cfg.APIKey,
		InstanceName: cfg.InstanceName,
		ApplyEnabled: cfg.ApplyEnabled,
		SearchBudget: budget,
		Runner:       runner,
		TMDB:         tmdbClient,
	})

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("listarr-go listening",
		"addr", cfg.Listen,
		"instance", cfg.InstanceName,
		"applyEnabled", cfg.ApplyEnabled,
		"torboxSearchPerHour", cfg.TorboxSearchPerHour,
		"tmdbConfigured", tmdbClient != nil,
		"radarrConfigured", deps.Radarr != nil,
		"sonarrConfigured", deps.Sonarr != nil,
		"version", api.Version,
	)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("listen failed", "err", err)
		os.Exit(1)
	}
}

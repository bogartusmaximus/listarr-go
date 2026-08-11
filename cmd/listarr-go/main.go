package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/api"
	"github.com/bogartusmaximus/listarr-go/internal/config"
	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	budget := ratelimit.NewHourlyBudget(cfg.TorboxSearchPerHour)
	srv := api.New(api.Config{
		APIKey:       cfg.APIKey,
		InstanceName: cfg.InstanceName,
		ApplyEnabled: cfg.ApplyEnabled,
		SearchBudget: budget,
	})

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("listarr-go listening",
		"addr", cfg.Listen,
		"instance", cfg.InstanceName,
		"applyEnabled", cfg.ApplyEnabled,
		"torboxSearchPerHour", cfg.TorboxSearchPerHour,
		"version", api.Version,
	)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("listen failed", "err", err)
		os.Exit(1)
	}
}

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultListen            = "127.0.0.1:8787"
	DefaultInstanceName      = "listarr"
	DefaultTorboxSearchPerHr = 60
	DefaultStoreBackend      = "polars"
)

// Config is process configuration loaded from the environment.
// Operator settings (API key, *arr instances, …) seed the store on first boot;
// listen address and store backend remain env-only.
type Config struct {
	APIKey              string
	Listen              string
	InstanceName        string
	ApplyEnabled        bool
	TorboxSearchPerHour int
	TMDBAPIKey          string
	StoreBackend        string
	DatabaseURL         string
	PolarsDir           string
}

// Load reads environment variables used for bootstrap and first-boot seed.
// LISTARR_API_KEY is required only when the store has no settings yet
// (enforced by appstate.LoadOrSeed).
func Load() (Config, error) {
	apiKey := strings.TrimSpace(os.Getenv("LISTARR_API_KEY"))

	listen := strings.TrimSpace(os.Getenv("LISTARR_LISTEN"))
	if listen == "" {
		listen = DefaultListen
	}

	instance := strings.TrimSpace(os.Getenv("LISTARR_INSTANCE_NAME"))
	if instance == "" {
		instance = DefaultInstanceName
	}

	perHour, err := parsePositiveIntEnv("LISTARR_TORBOX_SEARCH_PER_HOUR", DefaultTorboxSearchPerHr)
	if err != nil {
		return Config{}, err
	}

	backend := strings.ToLower(strings.TrimSpace(os.Getenv("LISTARR_STORE_BACKEND")))
	if backend == "" {
		backend = DefaultStoreBackend
	}

	dbURL := strings.TrimSpace(os.Getenv("LISTARR_DATABASE_URL"))
	if dbURL == "" {
		dbURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}

	polarsDir := strings.TrimSpace(os.Getenv("LISTARR_POLARS_DIR"))

	return Config{
		APIKey:              apiKey,
		Listen:              listen,
		InstanceName:        instance,
		ApplyEnabled:        os.Getenv("LISTARR_APPLY") == "1",
		TorboxSearchPerHour: perHour,
		TMDBAPIKey:          strings.TrimSpace(os.Getenv("LISTARR_TMDB_API_KEY")),
		StoreBackend:        backend,
		DatabaseURL:         dbURL,
		PolarsDir:           polarsDir,
	}, nil
}

func parsePositiveIntEnv(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	if n < 1 {
		return 0, fmt.Errorf("%s must be >= 1", name)
	}
	return n, nil
}

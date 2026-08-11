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
)

// Config is process configuration loaded from the environment.
// Service URLs are optional and never defaulted to private hosts.
type Config struct {
	APIKey              string
	Listen              string
	InstanceName        string
	ApplyEnabled        bool
	TorboxSearchPerHour int
	TMDBAPIKey          string
	RadarrURL           string
	RadarrAPIKey        string
	SonarrURL           string
	SonarrAPIKey        string
}

// Load reads environment variables and validates required fields.
func Load() (Config, error) {
	apiKey := strings.TrimSpace(os.Getenv("LISTARR_API_KEY"))
	if apiKey == "" {
		return Config{}, fmt.Errorf("LISTARR_API_KEY is required")
	}

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

	return Config{
		APIKey:              apiKey,
		Listen:              listen,
		InstanceName:        instance,
		ApplyEnabled:        os.Getenv("LISTARR_APPLY") == "1",
		TorboxSearchPerHour: perHour,
		TMDBAPIKey:          strings.TrimSpace(os.Getenv("LISTARR_TMDB_API_KEY")),
		RadarrURL:           strings.TrimSpace(os.Getenv("LISTARR_RADARR_URL")),
		RadarrAPIKey:        strings.TrimSpace(os.Getenv("LISTARR_RADARR_API_KEY")),
		SonarrURL:           strings.TrimSpace(os.Getenv("LISTARR_SONARR_URL")),
		SonarrAPIKey:        strings.TrimSpace(os.Getenv("LISTARR_SONARR_API_KEY")),
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

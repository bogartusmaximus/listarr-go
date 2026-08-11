package store

import (
	"context"
	"fmt"
	"time"
)

// Backend names supported by the factory.
type Backend string

const (
	BackendPostgres Backend = "postgres"
	BackendPolars   Backend = "polars"
	BackendSQLite   Backend = "sqlite"
	BackendMySQL    Backend = "mysql"
)

// SyncRun is one persisted preview/apply execution summary.
type SyncRun struct {
	ID             string    `json:"id"`
	DryRun         bool      `json:"dryRun"`
	Source         string    `json:"source"`
	MediaType      string    `json:"mediaType"`
	SourceInstance string    `json:"sourceInstance,omitempty"`
	TargetInstance string    `json:"targetInstance,omitempty"`
	Adds           int       `json:"adds"`
	Skips          int       `json:"skips"`
	Deferred       int       `json:"deferredSearch"`
	Errors         int       `json:"errors"`
	CreatedAt      time.Time `json:"createdAt"`
}

// ArrInstanceSettings is one named *arr target stored in settings.
type ArrInstanceSettings struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"` // radarr|sonarr
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
}

// Settings is the operator configuration document (SoT after first seed).
type Settings struct {
	APIKey              string                `json:"apiKey"`
	InstanceName        string                `json:"instanceName"`
	ApplyEnabled        bool                  `json:"applyEnabled"`
	TorboxSearchPerHour int                   `json:"torboxSearchPerHour"`
	TMDBAPIKey          string                `json:"tmdbApiKey"`
	ArrInstances        []ArrInstanceSettings `json:"arrInstances"`
	UpdatedAt           time.Time             `json:"updatedAt"`
}

// Store is the durable (or test) persistence surface.
type Store interface {
	Backend() Backend
	Close() error
	Ping(ctx context.Context) error
	SaveSyncRun(ctx context.Context, run SyncRun) error
	ListSyncRuns(ctx context.Context, limit int) ([]SyncRun, error)
	GetSettings(ctx context.Context) (Settings, bool, error)
	PutSettings(ctx context.Context, settings Settings) error
}

// Config selects and configures a backend. No private host defaults.
type Config struct {
	Backend     Backend
	DatabaseURL string // postgres (and future mysql/sqlite DSNs)
	PolarsDir   string // directory for polars CSV dumps
}

// Open constructs a Store. sqlite/mysql return explicit stub errors.
func Open(cfg Config) (Store, error) {
	switch cfg.Backend {
	case "", BackendPolars:
		return openPolars(cfg.PolarsDir)
	case BackendPostgres:
		return openPostgres(cfg.DatabaseURL)
	case BackendSQLite:
		return nil, fmt.Errorf("sqlite backend is stubbed; use postgres or polars (planned for community builds)")
	case BackendMySQL:
		return nil, fmt.Errorf("mysql backend is stubbed; use postgres or polars (planned for community builds)")
	default:
		return nil, fmt.Errorf("unknown store backend %q (want postgres|polars|sqlite|mysql)", cfg.Backend)
	}
}

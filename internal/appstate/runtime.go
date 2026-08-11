package appstate

import (
	"fmt"
	"sync"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/httpx"
	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
	"github.com/bogartusmaximus/listarr-go/internal/store"
	"github.com/bogartusmaximus/listarr-go/internal/syncjob"
	"github.com/bogartusmaximus/listarr-go/internal/tmdb"
)

// Runtime holds mutable operator settings applied to live HTTP handlers.
type Runtime struct {
	mu sync.RWMutex

	Store        store.Store
	HTTPClient   *httpx.Client
	SearchBudget *ratelimit.HourlyBudget

	APIKey       string
	InstanceName string
	ApplyEnabled bool
	Settings     store.Settings
	Arr          *arr.Registry
	TMDB         *tmdb.Client
	Runner       *syncjob.Runner
}

// Snapshot is a consistent view for request handlers.
type Snapshot struct {
	APIKey       string
	InstanceName string
	ApplyEnabled bool
	Settings     store.Settings
	Arr          *arr.Registry
	TMDB         *tmdb.Client
	Runner       *syncjob.Runner
	SearchBudget *ratelimit.HourlyBudget
	Store        store.Store
}

// View returns a point-in-time snapshot under read lock.
func (rt *Runtime) View() Snapshot {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return Snapshot{
		APIKey:       rt.APIKey,
		InstanceName: rt.InstanceName,
		ApplyEnabled: rt.ApplyEnabled,
		Settings:     cloneSettings(rt.Settings),
		Arr:          rt.Arr,
		TMDB:         rt.TMDB,
		Runner:       rt.Runner,
		SearchBudget: rt.SearchBudget,
		Store:        rt.Store,
	}
}

// Apply rebuilds live clients from settings (hot-reload).
func (rt *Runtime) Apply(set store.Settings) error {
	set = Normalize(set)
	if err := Validate(set); err != nil {
		return err
	}
	reg, err := arr.LoadRegistry(ArrConfigs(set), rt.HTTPClient)
	if err != nil {
		return err
	}
	var tmdbClient *tmdb.Client
	if set.TMDBAPIKey != "" {
		tmdbClient, err = tmdb.New(set.TMDBAPIKey, "", rt.HTTPClient)
		if err != nil {
			return fmt.Errorf("tmdb: %w", err)
		}
	}
	set.UpdatedAt = time.Now().UTC()

	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.APIKey = set.APIKey
	rt.InstanceName = set.InstanceName
	rt.ApplyEnabled = set.ApplyEnabled
	rt.Settings = cloneSettings(set)
	rt.Arr = reg
	rt.TMDB = tmdbClient
	if rt.SearchBudget != nil {
		rt.SearchBudget.SetLimit(set.TorboxSearchPerHour)
	}
	deps := syncjob.Dependencies{Arr: reg, SearchBudget: rt.SearchBudget, TMDB: tmdbClient}
	if tmdbClient != nil || reg.Len() > 0 {
		rt.Runner = &syncjob.Runner{Deps: deps}
	} else {
		rt.Runner = nil
	}
	return nil
}

func cloneSettings(in store.Settings) store.Settings {
	out := in
	if in.ArrInstances == nil {
		out.ArrInstances = []store.ArrInstanceSettings{}
		return out
	}
	out.ArrInstances = make([]store.ArrInstanceSettings, len(in.ArrInstances))
	copy(out.ArrInstances, in.ArrInstances)
	return out
}

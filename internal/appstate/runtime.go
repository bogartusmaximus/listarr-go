package appstate

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/catalog"
	"github.com/bogartusmaximus/listarr-go/internal/httpx"
	"github.com/bogartusmaximus/listarr-go/internal/jobs"
	"github.com/bogartusmaximus/listarr-go/internal/plex"
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
	SafeMode     bool
	Settings     store.Settings
	Arr          *arr.Registry
	TMDB         *tmdb.Client
	Runner       *syncjob.Runner
}

// Snapshot is a consistent view for request handlers.
type Snapshot struct {
	APIKey       string
	InstanceName string
	SafeMode     bool
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
		SafeMode:     rt.SafeMode,
		Settings:     cloneSettings(rt.Settings),
		Arr:          rt.Arr,
		TMDB:         rt.TMDB,
		Runner:       rt.Runner,
		SearchBudget: rt.SearchBudget,
		Store:        rt.Store,
	}
}

// JobRuntime implements jobs.Viewer for the async worker.
func (rt *Runtime) JobRuntime() jobs.RuntimeView {
	view := rt.View()
	return jobs.RuntimeView{
		Sync:     view.Runner,
		Catalog:  CatalogService(view, rt.HTTPClient),
		Settings: view.Settings,
	}
}

// CatalogService builds ingest/watched helpers from a snapshot.
func CatalogService(view Snapshot, httpClient *httpx.Client) *catalog.Service {
	if view.Store == nil {
		return nil
	}
	svc := &catalog.Service{Store: view.Store, Arr: view.Arr}
	plexCfg := view.Settings.Plex
	if strings.TrimSpace(plexCfg.ServerURL) != "" && strings.TrimSpace(plexCfg.Token) != "" {
		client, err := plex.New(plexCfg.ServerURL, plexCfg.Token, plexCfg.ClientIdentifier, httpClient)
		if err == nil {
			svc.Plex = client
		}
	}
	return svc
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
	rt.SafeMode = set.SafeMode
	rt.Settings = cloneSettings(set)
	rt.Arr = reg
	rt.TMDB = tmdbClient
	if rt.SearchBudget != nil {
		rt.SearchBudget.SetLimit(set.TorboxSearchPerHour)
	}
	deps := syncjob.Dependencies{Arr: reg, SearchBudget: rt.SearchBudget, TMDB: tmdbClient, Store: rt.Store}
	if tmdbClient != nil || reg.Len() > 0 || rt.Store != nil {
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
	} else {
		out.ArrInstances = make([]store.ArrInstanceSettings, len(in.ArrInstances))
		copy(out.ArrInstances, in.ArrInstances)
	}
	if in.SyncRoutes == nil {
		out.SyncRoutes = []store.SyncRoute{}
	} else {
		out.SyncRoutes = make([]store.SyncRoute, len(in.SyncRoutes))
		copy(out.SyncRoutes, in.SyncRoutes)
	}
	return out
}

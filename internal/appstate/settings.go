package appstate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/config"
	"github.com/bogartusmaximus/listarr-go/internal/store"
)

// SeedFromEnv builds Settings from process env (used when the store is empty).
// API key is never taken from env — callers must set it (usually via GenerateAPIKey).
func SeedFromEnv(cfg config.Config) (store.Settings, error) {
	instances, err := arr.InstancesFromEnv()
	if err != nil {
		return store.Settings{}, err
	}
	arrs := make([]store.ArrInstanceSettings, 0, len(instances))
	for _, c := range instances {
		arrs = append(arrs, store.ArrInstanceSettings{
			Name:       c.Name,
			Kind:       string(c.Kind),
			URL:        c.URL,
			APIKey:     c.APIKey,
			AuthCookie: c.AuthCookie,
		})
	}
	return store.Settings{
		InstanceName:        cfg.InstanceName,
		SafeMode:            !cfg.ApplyEnabled,
		TorboxSearchPerHour: cfg.TorboxSearchPerHour,
		TMDBAPIKey:          cfg.TMDBAPIKey,
		ArrInstances:        arrs,
		UpdatedAt:           time.Now().UTC(),
	}, nil
}

// LoadOrSeed returns store settings, seeding from env when missing.
// The second return value is true when a new API key was generated (log it once).
func LoadOrSeed(ctx context.Context, st store.Store, cfg config.Config) (store.Settings, bool, error) {
	set, found, err := st.GetSettings(ctx)
	if err != nil {
		return store.Settings{}, false, err
	}
	if found {
		if set.ArrInstances == nil {
			set.ArrInstances = []store.ArrInstanceSettings{}
		}
		if strings.TrimSpace(set.APIKey) != "" {
			return set, false, nil
		}
		key, err := GenerateAPIKey()
		if err != nil {
			return store.Settings{}, false, err
		}
		set.APIKey = key
		set.UpdatedAt = time.Now().UTC()
		if err := Validate(set); err != nil {
			return store.Settings{}, false, err
		}
		if err := st.PutSettings(ctx, set); err != nil {
			return store.Settings{}, false, err
		}
		return set, true, nil
	}

	seed, err := SeedFromEnv(cfg)
	if err != nil {
		return store.Settings{}, false, err
	}
	key, err := GenerateAPIKey()
	if err != nil {
		return store.Settings{}, false, err
	}
	seed.APIKey = key
	if err := Validate(seed); err != nil {
		return store.Settings{}, false, err
	}
	if err := st.PutSettings(ctx, seed); err != nil {
		return store.Settings{}, false, err
	}
	return seed, true, nil
}

// Validate checks operator settings before persist / apply.
func Validate(set store.Settings) error {
	if strings.TrimSpace(set.APIKey) == "" {
		return fmt.Errorf("apiKey is required")
	}
	if strings.TrimSpace(set.InstanceName) == "" {
		return fmt.Errorf("instanceName is required")
	}
	if set.TorboxSearchPerHour < 1 {
		return fmt.Errorf("torboxSearchPerHour must be >= 1")
	}
	seen := map[string]struct{}{}
	for i, inst := range set.ArrInstances {
		name := strings.ToLower(strings.TrimSpace(inst.Name))
		if name == "" {
			return fmt.Errorf("arrInstances[%d]: name is required", i)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate arr instance %q", name)
		}
		seen[name] = struct{}{}
		kind := arr.Kind(strings.ToLower(strings.TrimSpace(inst.Kind)))
		if kind != arr.KindRadarr && kind != arr.KindSonarr {
			return fmt.Errorf("arr instance %q: kind must be radarr or sonarr", name)
		}
		if strings.TrimSpace(inst.URL) == "" || strings.TrimSpace(inst.APIKey) == "" {
			return fmt.Errorf("arr instance %q: url and apiKey are required", name)
		}
	}
	return nil
}

// Normalize trims fields and lowercases instance names/kinds.
func Normalize(set store.Settings) store.Settings {
	set.APIKey = strings.TrimSpace(set.APIKey)
	set.InstanceName = strings.TrimSpace(set.InstanceName)
	set.TMDBAPIKey = strings.TrimSpace(set.TMDBAPIKey)
	if set.ArrInstances == nil {
		set.ArrInstances = []store.ArrInstanceSettings{}
	}
	out := make([]store.ArrInstanceSettings, 0, len(set.ArrInstances))
	for _, inst := range set.ArrInstances {
		out = append(out, store.ArrInstanceSettings{
			Name:       strings.ToLower(strings.TrimSpace(inst.Name)),
			Kind:       strings.ToLower(strings.TrimSpace(inst.Kind)),
			URL:        strings.TrimSpace(inst.URL),
			APIKey:     strings.TrimSpace(inst.APIKey),
			AuthCookie: strings.TrimSpace(inst.AuthCookie),
		})
	}
	set.ArrInstances = out
	return set
}

// ArrConfigs converts stored instances into arr.InstanceConfig values.
func ArrConfigs(set store.Settings) []arr.InstanceConfig {
	out := make([]arr.InstanceConfig, 0, len(set.ArrInstances))
	for _, inst := range set.ArrInstances {
		out = append(out, arr.InstanceConfig{
			Name:       inst.Name,
			Kind:       arr.Kind(inst.Kind),
			URL:        inst.URL,
			APIKey:     inst.APIKey,
			AuthCookie: inst.AuthCookie,
		})
	}
	return out
}

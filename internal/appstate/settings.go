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
func SeedFromEnv(cfg config.Config) (store.Settings, error) {
	instances, err := arr.InstancesFromEnv()
	if err != nil {
		return store.Settings{}, err
	}
	arrs := make([]store.ArrInstanceSettings, 0, len(instances))
	for _, c := range instances {
		arrs = append(arrs, store.ArrInstanceSettings{
			Name:   c.Name,
			Kind:   string(c.Kind),
			URL:    c.URL,
			APIKey: c.APIKey,
		})
	}
	return store.Settings{
		APIKey:              cfg.APIKey,
		InstanceName:        cfg.InstanceName,
		ApplyEnabled:        cfg.ApplyEnabled,
		TorboxSearchPerHour: cfg.TorboxSearchPerHour,
		TMDBAPIKey:          cfg.TMDBAPIKey,
		ArrInstances:        arrs,
		UpdatedAt:           time.Now().UTC(),
	}, nil
}

// LoadOrSeed returns store settings, seeding from env when missing.
func LoadOrSeed(ctx context.Context, st store.Store, cfg config.Config) (store.Settings, error) {
	set, found, err := st.GetSettings(ctx)
	if err != nil {
		return store.Settings{}, err
	}
	if found {
		if set.ArrInstances == nil {
			set.ArrInstances = []store.ArrInstanceSettings{}
		}
		return set, nil
	}
	seed, err := SeedFromEnv(cfg)
	if err != nil {
		return store.Settings{}, err
	}
	if strings.TrimSpace(seed.APIKey) == "" {
		return store.Settings{}, fmt.Errorf("LISTARR_API_KEY is required to seed an empty settings store")
	}
	if err := Validate(seed); err != nil {
		return store.Settings{}, err
	}
	if err := st.PutSettings(ctx, seed); err != nil {
		return store.Settings{}, err
	}
	return seed, nil
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
			Name:   strings.ToLower(strings.TrimSpace(inst.Name)),
			Kind:   strings.ToLower(strings.TrimSpace(inst.Kind)),
			URL:    strings.TrimSpace(inst.URL),
			APIKey: strings.TrimSpace(inst.APIKey),
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
			Name:   inst.Name,
			Kind:   arr.Kind(inst.Kind),
			URL:    inst.URL,
			APIKey: inst.APIKey,
		})
	}
	return out
}

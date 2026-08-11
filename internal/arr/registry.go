package arr

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/bogartusmaximus/listarr-go/internal/httpx"
)

// Kind identifies an *arr application family.
type Kind string

const (
	KindRadarr Kind = "radarr"
	KindSonarr Kind = "sonarr"
)

// InstanceMeta is a public-safe description (no URLs or keys).
type InstanceMeta struct {
	Name string `json:"name"`
	Kind Kind   `json:"kind"`
}

// Registry holds named *arr instances for multi-homed sync.
type Registry struct {
	radarr map[string]*Radarr
	sonarr map[string]*Sonarr
	meta   map[string]InstanceMeta
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		radarr: map[string]*Radarr{},
		sonarr: map[string]*Sonarr{},
		meta:   map[string]InstanceMeta{},
	}
}

// RegisterRadarr adds a named Radarr instance.
func (r *Registry) RegisterRadarr(name string, client *Radarr) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	if client == nil {
		return fmt.Errorf("radarr client is nil")
	}
	if _, ok := r.meta[name]; ok {
		return fmt.Errorf("instance %q already registered", name)
	}
	r.radarr[name] = client
	r.meta[name] = InstanceMeta{Name: name, Kind: KindRadarr}
	return nil
}

// RegisterSonarr adds a named Sonarr instance.
func (r *Registry) RegisterSonarr(name string, client *Sonarr) error {
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	if client == nil {
		return fmt.Errorf("sonarr client is nil")
	}
	if _, ok := r.meta[name]; ok {
		return fmt.Errorf("instance %q already registered", name)
	}
	r.sonarr[name] = client
	r.meta[name] = InstanceMeta{Name: name, Kind: KindSonarr}
	return nil
}

// List returns public-safe instance metadata sorted by name.
func (r *Registry) List() []InstanceMeta {
	out := make([]InstanceMeta, 0, len(r.meta))
	for _, m := range r.meta {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Radarr returns a named Radarr client.
func (r *Registry) Radarr(name string) (*Radarr, error) {
	name, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	c, ok := r.radarr[name]
	if !ok {
		return nil, fmt.Errorf("radarr instance %q not configured", name)
	}
	return c, nil
}

// Sonarr returns a named Sonarr client.
func (r *Registry) Sonarr(name string) (*Sonarr, error) {
	name, err := normalizeName(name)
	if err != nil {
		return nil, err
	}
	c, ok := r.sonarr[name]
	if !ok {
		return nil, fmt.Errorf("sonarr instance %q not configured", name)
	}
	return c, nil
}

// Len returns how many instances are registered.
func (r *Registry) Len() int {
	return len(r.meta)
}

// InstanceConfig is enough to build a client (used for env seed and store settings).
type InstanceConfig struct {
	Name       string
	Kind       Kind
	URL        string
	APIKey     string
	AuthCookie string
}

// LoadRegistry builds a registry from explicit instance configs.
func LoadRegistry(configs []InstanceConfig, httpClient *httpx.Client) (*Registry, error) {
	reg := NewRegistry()
	for _, c := range configs {
		if err := registerKind(reg, c.Name, c.Kind, c.URL, c.APIKey, c.AuthCookie, httpClient); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

// InstancesFromEnv reads LISTARR_ARR_<NAME>_{URL,API_KEY,KIND} plus legacy
// LISTARR_RADARR_* / LISTARR_SONARR_* aliases (names "radarr" / "sonarr").
func InstancesFromEnv() ([]InstanceConfig, error) {
	names := map[string]struct{}{}
	for _, env := range os.Environ() {
		const prefix = "LISTARR_ARR_"
		if !strings.HasPrefix(env, prefix) || !strings.Contains(env, "_URL=") {
			continue
		}
		rest := strings.TrimPrefix(strings.SplitN(env, "=", 2)[0], prefix)
		name := strings.TrimSuffix(rest, "_URL")
		if name == "" || name == rest {
			continue
		}
		names[strings.ToLower(name)] = struct{}{}
	}

	out := make([]InstanceConfig, 0, len(names)+2)
	seen := map[string]struct{}{}
	for name := range names {
		prefix := "LISTARR_ARR_" + strings.ToUpper(name) + "_"
		url := strings.TrimSpace(os.Getenv(prefix + "URL"))
		key := strings.TrimSpace(os.Getenv(prefix + "API_KEY"))
		kind := Kind(strings.ToLower(strings.TrimSpace(os.Getenv(prefix + "KIND"))))
		cookie := strings.TrimSpace(os.Getenv(prefix + "AUTH_COOKIE"))
		if url == "" && key == "" {
			continue
		}
		if url == "" || key == "" {
			return nil, fmt.Errorf("instance %q requires both URL and API_KEY", name)
		}
		if kind == "" {
			return nil, fmt.Errorf("instance %q requires KIND=radarr|sonarr", name)
		}
		out = append(out, InstanceConfig{Name: name, Kind: kind, URL: url, APIKey: key, AuthCookie: cookie})
		seen[strings.ToLower(name)] = struct{}{}
	}

	if url := strings.TrimSpace(os.Getenv("LISTARR_RADARR_URL")); url != "" {
		key := strings.TrimSpace(os.Getenv("LISTARR_RADARR_API_KEY"))
		if key == "" {
			return nil, fmt.Errorf("LISTARR_RADARR_API_KEY is required when LISTARR_RADARR_URL is set")
		}
		if _, exists := seen["radarr"]; !exists {
			out = append(out, InstanceConfig{Name: "radarr", Kind: KindRadarr, URL: url, APIKey: key})
			seen["radarr"] = struct{}{}
		}
	}
	if url := strings.TrimSpace(os.Getenv("LISTARR_SONARR_URL")); url != "" {
		key := strings.TrimSpace(os.Getenv("LISTARR_SONARR_API_KEY"))
		if key == "" {
			return nil, fmt.Errorf("LISTARR_SONARR_API_KEY is required when LISTARR_SONARR_URL is set")
		}
		if _, exists := seen["sonarr"]; !exists {
			out = append(out, InstanceConfig{Name: "sonarr", Kind: KindSonarr, URL: url, APIKey: key})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// LoadRegistryFromEnv reads named instances from the environment.
func LoadRegistryFromEnv(httpClient *httpx.Client) (*Registry, error) {
	cfgs, err := InstancesFromEnv()
	if err != nil {
		return nil, err
	}
	return LoadRegistry(cfgs, httpClient)
}

func registerKind(reg *Registry, name string, kind Kind, url, key, authCookie string, httpClient *httpx.Client) error {
	switch kind {
	case KindRadarr:
		c, err := NewRadarr(url, key, authCookie, httpClient)
		if err != nil {
			return fmt.Errorf("instance %q: %w", name, err)
		}
		return reg.RegisterRadarr(name, c)
	case KindSonarr:
		c, err := NewSonarr(url, key, authCookie, httpClient)
		if err != nil {
			return fmt.Errorf("instance %q: %w", name, err)
		}
		return reg.RegisterSonarr(name, c)
	default:
		return fmt.Errorf("instance %q: unsupported kind %q", name, kind)
	}
}

func normalizeName(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", fmt.Errorf("instance name is required")
	}
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return "", fmt.Errorf("instance name %q must be [a-z0-9_-]", name)
	}
	return name, nil
}

package arr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/httpx"
	"github.com/bogartusmaximus/listarr-go/internal/redact"
)

// Target describes where to place a title in Radarr/Sonarr.
type Target struct {
	RootFolderPath   string `json:"rootFolderPath"`
	QualityProfileID int    `json:"qualityProfileId"`
	Tags             []int  `json:"tags,omitempty"`
	Monitored        bool   `json:"monitored"`
	SearchOnAdd      bool   `json:"searchOnAdd"`
	SeasonFolder     bool   `json:"seasonFolder,omitempty"`
}

// MovieRef is a library membership check key.
type MovieRef struct {
	TMDBID int
	Title  string
}

// SeriesRef is a library membership check key.
type SeriesRef struct {
	TMDBID int
	Title  string
}

// Radarr talks to Radarr API v3.
type Radarr struct {
	base string
	http *httpx.Client
}

// NewRadarr requires an absolute base URL and API key.
func NewRadarr(baseURL, apiKey string, httpClient *httpx.Client) (*Radarr, error) {
	base, err := normalizeBase(baseURL)
	if err != nil {
		return nil, err
	}
	if apiKey == "" {
		return nil, fmt.Errorf("radarr api key is required")
	}
	hc := keyedClient(apiKey, httpClient)
	return &Radarr{base: base, http: hc}, nil
}

// ListMovies returns TMDB IDs already in the library.
func (r *Radarr) ListMovies(ctx context.Context) (map[int]MovieRef, error) {
	rawURL := r.base + "/api/v3/movie"
	resp, body, err := r.http.DoJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return nil, err
	}
	var rows []struct {
		Title  string `json:"title"`
		TMDBID int    `json:"tmdbId"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("radarr decode movie list: %w", err)
	}
	out := make(map[int]MovieRef, len(rows))
	for _, row := range rows {
		if row.TMDBID == 0 {
			continue
		}
		out[row.TMDBID] = MovieRef{TMDBID: row.TMDBID, Title: row.Title}
	}
	return out, nil
}

// LookupByTMDB resolves add payload fields via Radarr lookup.
func (r *Radarr) LookupByTMDB(ctx context.Context, tmdbID int) (map[string]any, error) {
	rawURL := r.base + "/api/v3/movie/lookup?term=" + url.QueryEscape(fmt.Sprintf("tmdb:%d", tmdbID))
	resp, body, err := r.http.DoJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return nil, err
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("radarr decode lookup: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("radarr lookup empty for tmdb:%d", tmdbID)
	}
	return rows[0], nil
}

// AddMovie posts a monitored movie. searchOnAdd is honored via addOptions.
func (r *Radarr) AddMovie(ctx context.Context, lookup map[string]any, target Target) error {
	lookup["rootFolderPath"] = target.RootFolderPath
	lookup["qualityProfileId"] = target.QualityProfileID
	lookup["monitored"] = target.Monitored
	lookup["tags"] = target.Tags
	lookup["addOptions"] = map[string]any{
		"searchForMovie": target.SearchOnAdd,
	}
	payload, err := json.Marshal(lookup)
	if err != nil {
		return err
	}
	rawURL := r.base + "/api/v3/movie"
	resp, body, err := r.http.DoJSON(ctx, http.MethodPost, rawURL, bytes.NewReader(payload), http.Header{
		"Content-Type": []string{"application/json"},
	})
	if err != nil {
		return err
	}
	return httpx.CheckStatus(resp, rawURL, body)
}

// Sonarr talks to Sonarr API v3.
type Sonarr struct {
	base string
	http *httpx.Client
}

// NewSonarr requires an absolute base URL and API key.
func NewSonarr(baseURL, apiKey string, httpClient *httpx.Client) (*Sonarr, error) {
	base, err := normalizeBase(baseURL)
	if err != nil {
		return nil, err
	}
	if apiKey == "" {
		return nil, fmt.Errorf("sonarr api key is required")
	}
	hc := keyedClient(apiKey, httpClient)
	return &Sonarr{base: base, http: hc}, nil
}

// ListSeries returns TMDB IDs already in the library.
func (s *Sonarr) ListSeries(ctx context.Context) (map[int]SeriesRef, error) {
	rawURL := s.base + "/api/v3/series"
	resp, body, err := s.http.DoJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return nil, err
	}
	var rows []struct {
		Title  string `json:"title"`
		TMDBID int    `json:"tmdbId"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("sonarr decode series list: %w", err)
	}
	out := make(map[int]SeriesRef, len(rows))
	for _, row := range rows {
		if row.TMDBID == 0 {
			continue
		}
		out[row.TMDBID] = SeriesRef{TMDBID: row.TMDBID, Title: row.Title}
	}
	return out, nil
}

// LookupByTMDB resolves add payload fields via Sonarr lookup.
func (s *Sonarr) LookupByTMDB(ctx context.Context, tmdbID int) (map[string]any, error) {
	rawURL := s.base + "/api/v3/series/lookup?term=" + url.QueryEscape(fmt.Sprintf("tmdb:%d", tmdbID))
	resp, body, err := s.http.DoJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return nil, err
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("sonarr decode lookup: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("sonarr lookup empty for tmdb:%d", tmdbID)
	}
	return rows[0], nil
}

// AddSeries posts a monitored series.
func (s *Sonarr) AddSeries(ctx context.Context, lookup map[string]any, target Target) error {
	lookup["rootFolderPath"] = target.RootFolderPath
	lookup["qualityProfileId"] = target.QualityProfileID
	lookup["monitored"] = target.Monitored
	lookup["seasonFolder"] = target.SeasonFolder
	lookup["tags"] = target.Tags
	lookup["addOptions"] = map[string]any{
		"searchForMissingEpisodes": target.SearchOnAdd,
	}
	payload, err := json.Marshal(lookup)
	if err != nil {
		return err
	}
	rawURL := s.base + "/api/v3/series"
	resp, body, err := s.http.DoJSON(ctx, http.MethodPost, rawURL, bytes.NewReader(payload), http.Header{
		"Content-Type": []string{"application/json"},
	})
	if err != nil {
		return err
	}
	return httpx.CheckStatus(resp, rawURL, body)
}

func keyedClient(apiKey string, template *httpx.Client) *httpx.Client {
	timeout := templateTimeout(template)
	hc := httpx.New(timeout)
	hc.APIKey = apiKey
	hc.Header = "X-Api-Key"
	return hc
}

func templateTimeout(template *httpx.Client) time.Duration {
	if template == nil {
		return 0
	}
	return template.Timeout
}

func normalizeBase(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("base url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid base url %q", redact.URLString(raw))
	}
	return strings.TrimRight(raw, "/"), nil
}

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

// MovieRef is a library membership / export record.
type MovieRef struct {
	TMDBID     int             `json:"tmdbId"`
	IMDBID     string          `json:"imdbId,omitempty"`
	Title      string          `json:"title"`
	Year       int             `json:"year,omitempty"`
	Overview   string          `json:"overview,omitempty"`
	Monitored  bool            `json:"monitored"`
	Tags       []int           `json:"tags,omitempty"`
	Path       string          `json:"path,omitempty"`
	Collection *CollectionRef  `json:"collection,omitempty"`
}

// CollectionRef is a Radarr movie collection membership.
type CollectionRef struct {
	TMDBID int    `json:"tmdbId,omitempty"`
	Title  string `json:"title,omitempty"`
}

// SeriesRef is a library membership / export record.
type SeriesRef struct {
	TMDBID    int         `json:"tmdbId"`
	IMDBID    string      `json:"imdbId,omitempty"`
	Title     string      `json:"title"`
	Year      int         `json:"year,omitempty"`
	Overview  string      `json:"overview,omitempty"`
	Monitored bool        `json:"monitored"`
	Tags      []int       `json:"tags,omitempty"`
	Path      string      `json:"path,omitempty"`
	Seasons   []SeasonRef `json:"seasons,omitempty"`
}

// SeasonRef is one Sonarr season row.
type SeasonRef struct {
	SeasonNumber int               `json:"seasonNumber"`
	Monitored    bool              `json:"monitored"`
	Statistics   *SeasonStatistics `json:"statistics,omitempty"`
}

// SeasonStatistics carries episode counts from Sonarr.
type SeasonStatistics struct {
	EpisodeCount     int `json:"episodeCount,omitempty"`
	EpisodeFileCount int `json:"episodeFileCount,omitempty"`
}

// ImportList is a configured *arr Import List (metadata only).
type ImportList struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	Enabled            bool   `json:"enabled"`
	EnableAutomaticAdd bool   `json:"enableAutomaticAdd"`
	Tags               []int  `json:"tags,omitempty"`
	ListType           string `json:"listType,omitempty"`
	Implementation     string `json:"implementation,omitempty"`
}

// LibraryFilter selects a subset of an *arr library (tag/list-shaped sync).
type LibraryFilter struct {
	MonitoredOnly  bool   `json:"monitoredOnly,omitempty"`
	TagIDs         []int  `json:"tagIds,omitempty"`
	PathContains   string `json:"pathContains,omitempty"`
	RequireAllTags bool   `json:"requireAllTags,omitempty"`
}

// Target describes where to place a title in Radarr/Sonarr.
type Target struct {
	Instance         string `json:"instance,omitempty"`
	RootFolderPath   string `json:"rootFolderPath"`
	QualityProfileID int    `json:"qualityProfileId"`
	Tags             []int  `json:"tags,omitempty"`
	Monitored        bool   `json:"monitored"`
	SearchOnAdd      bool   `json:"searchOnAdd"`
	SeasonFolder     bool   `json:"seasonFolder,omitempty"`
}

// RootFolder is an *arr library root path.
type RootFolder struct {
	Path string `json:"path"`
}

// QualityProfile is an *arr quality profile.
type QualityProfile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Radarr talks to Radarr API v3.
type Radarr struct {
	base string
	http *httpx.Client
}

// NewRadarr requires an absolute base URL and API key.
// authCookie is an optional Cookie header value (e.g. _oauth2_proxy=...).
func NewRadarr(baseURL, apiKey, authCookie string, httpClient *httpx.Client) (*Radarr, error) {
	base, err := normalizeBase(baseURL)
	if err != nil {
		return nil, err
	}
	if apiKey == "" {
		return nil, fmt.Errorf("radarr api key is required")
	}
	return &Radarr{base: base, http: keyedClient(apiKey, authCookie, httpClient)}, nil
}

// ListMovies returns TMDB-keyed movie refs from the library.
func (r *Radarr) ListMovies(ctx context.Context) (map[int]MovieRef, error) {
	rows, err := r.ExportMovies(ctx, LibraryFilter{})
	if err != nil {
		return nil, err
	}
	out := make(map[int]MovieRef, len(rows))
	for _, row := range rows {
		out[row.TMDBID] = row
	}
	return out, nil
}

// ExportMovies returns movies matching an optional filter.
func (r *Radarr) ExportMovies(ctx context.Context, filter LibraryFilter) ([]MovieRef, error) {
	rawURL := r.base + "/api/v3/movie"
	resp, body, err := r.http.DoJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("radarr movie list empty body (check auth cookie / reverse proxy)")
	}
	var rows []MovieRef
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("radarr decode movie list: %w (bytes=%d)", err, len(body))
	}
	out := make([]MovieRef, 0, len(rows))
	for _, row := range rows {
		if row.TMDBID == 0 && strings.TrimSpace(row.IMDBID) == "" {
			continue
		}
		if matchMovie(row, filter) {
			out = append(out, row)
		}
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
	lookup["addOptions"] = map[string]any{"searchForMovie": target.SearchOnAdd}
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

// ListImportLists returns Import List configurations (not title payloads).
func (r *Radarr) ListImportLists(ctx context.Context) ([]ImportList, error) {
	return fetchImportLists(ctx, r.http, r.base)
}

// ListRootFolders returns configured Radarr root folder paths.
func (r *Radarr) ListRootFolders(ctx context.Context) ([]RootFolder, error) {
	return fetchRootFolders(ctx, r.http, r.base)
}

// ListQualityProfiles returns Radarr quality profiles.
func (r *Radarr) ListQualityProfiles(ctx context.Context) ([]QualityProfile, error) {
	return fetchQualityProfiles(ctx, r.http, r.base)
}

// Sonarr talks to Sonarr API v3.
type Sonarr struct {
	base string
	http *httpx.Client
}

// NewSonarr requires an absolute base URL and API key.
// authCookie is an optional Cookie header value (e.g. _oauth2_proxy=...).
func NewSonarr(baseURL, apiKey, authCookie string, httpClient *httpx.Client) (*Sonarr, error) {
	base, err := normalizeBase(baseURL)
	if err != nil {
		return nil, err
	}
	if apiKey == "" {
		return nil, fmt.Errorf("sonarr api key is required")
	}
	return &Sonarr{base: base, http: keyedClient(apiKey, authCookie, httpClient)}, nil
}

// ListSeries returns TMDB-keyed series refs from the library.
func (s *Sonarr) ListSeries(ctx context.Context) (map[int]SeriesRef, error) {
	rows, err := s.ExportSeries(ctx, LibraryFilter{})
	if err != nil {
		return nil, err
	}
	out := make(map[int]SeriesRef, len(rows))
	for _, row := range rows {
		out[row.TMDBID] = row
	}
	return out, nil
}

// ExportSeries returns series matching an optional filter.
func (s *Sonarr) ExportSeries(ctx context.Context, filter LibraryFilter) ([]SeriesRef, error) {
	rawURL := s.base + "/api/v3/series"
	resp, body, err := s.http.DoJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("sonarr series list empty body (check auth cookie / reverse proxy)")
	}
	var rows []SeriesRef
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("sonarr decode series list: %w (bytes=%d)", err, len(body))
	}
	out := make([]SeriesRef, 0, len(rows))
	for _, row := range rows {
		if row.TMDBID == 0 && strings.TrimSpace(row.IMDBID) == "" {
			continue
		}
		if matchSeries(row, filter) {
			out = append(out, row)
		}
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
	lookup["addOptions"] = map[string]any{"searchForMissingEpisodes": target.SearchOnAdd}
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

// ListImportLists returns Import List configurations (not title payloads).
func (s *Sonarr) ListImportLists(ctx context.Context) ([]ImportList, error) {
	return fetchImportLists(ctx, s.http, s.base)
}

// ListRootFolders returns configured Sonarr root folder paths.
func (s *Sonarr) ListRootFolders(ctx context.Context) ([]RootFolder, error) {
	return fetchRootFolders(ctx, s.http, s.base)
}

// ListQualityProfiles returns Sonarr quality profiles.
func (s *Sonarr) ListQualityProfiles(ctx context.Context) ([]QualityProfile, error) {
	return fetchQualityProfiles(ctx, s.http, s.base)
}

func fetchRootFolders(ctx context.Context, hc *httpx.Client, base string) ([]RootFolder, error) {
	rawURL := base + "/api/v3/rootfolder"
	resp, body, err := hc.DoJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return nil, err
	}
	var rows []RootFolder
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("rootfolder decode: %w", err)
	}
	out := make([]RootFolder, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Path) == "" {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func fetchQualityProfiles(ctx context.Context, hc *httpx.Client, base string) ([]QualityProfile, error) {
	rawURL := base + "/api/v3/qualityprofile"
	resp, body, err := hc.DoJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return nil, err
	}
	var rows []QualityProfile
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("qualityprofile decode: %w", err)
	}
	out := make([]QualityProfile, 0, len(rows))
	for _, row := range rows {
		if row.ID < 1 {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func fetchImportLists(ctx context.Context, hc *httpx.Client, base string) ([]ImportList, error) {
	rawURL := base + "/api/v3/importlist"
	resp, body, err := hc.DoJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return nil, err
	}
	var rows []ImportList
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("importlist decode: %w", err)
	}
	return rows, nil
}

func matchMovie(row MovieRef, filter LibraryFilter) bool {
	if filter.MonitoredOnly && !row.Monitored {
		return false
	}
	if filter.PathContains != "" && !strings.Contains(row.Path, filter.PathContains) {
		return false
	}
	return matchTags(row.Tags, filter.TagIDs, filter.RequireAllTags)
}

func matchSeries(row SeriesRef, filter LibraryFilter) bool {
	if filter.MonitoredOnly && !row.Monitored {
		return false
	}
	if filter.PathContains != "" && !strings.Contains(row.Path, filter.PathContains) {
		return false
	}
	return matchTags(row.Tags, filter.TagIDs, filter.RequireAllTags)
}

func matchTags(have, want []int, requireAll bool) bool {
	if len(want) == 0 {
		return true
	}
	set := make(map[int]struct{}, len(have))
	for _, t := range have {
		set[t] = struct{}{}
	}
	if requireAll {
		for _, t := range want {
			if _, ok := set[t]; !ok {
				return false
			}
		}
		return true
	}
	for _, t := range want {
		if _, ok := set[t]; ok {
			return true
		}
	}
	return false
}

// ConnectionTest is a safe result from probing an *arr instance.
type ConnectionTest struct {
	OK      bool   `json:"ok"`
	Kind    Kind   `json:"kind"`
	AppName string `json:"appName,omitempty"`
	Version string `json:"version,omitempty"`
	Message string `json:"message,omitempty"`
}

// TestConnection calls GET /api/v3/system/status on the given base URL.
func TestConnection(ctx context.Context, kind Kind, baseURL, apiKey, authCookie string, httpClient *httpx.Client) (ConnectionTest, error) {
	kind = Kind(strings.ToLower(strings.TrimSpace(string(kind))))
	if kind != KindRadarr && kind != KindSonarr {
		return ConnectionTest{}, fmt.Errorf("kind must be radarr or sonarr")
	}
	base, err := normalizeBase(baseURL)
	if err != nil {
		return ConnectionTest{}, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return ConnectionTest{}, fmt.Errorf("api key is required")
	}
	hc := keyedClient(apiKey, authCookie, httpClient)
	rawURL := base + "/api/v3/system/status"
	resp, body, err := hc.DoJSON(ctx, http.MethodGet, rawURL, nil, nil)
	if err != nil {
		msg := redact.APIKey(err.Error(), authCookie)
		return ConnectionTest{OK: false, Kind: kind, Message: msg}, nil
	}
	if err := httpx.CheckStatusRedact(resp, rawURL, body, authCookie); err != nil {
		return ConnectionTest{OK: false, Kind: kind, Message: err.Error()}, nil
	}
	var status struct {
		AppName string `json:"appName"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return ConnectionTest{OK: false, Kind: kind, Message: "decode system status: " + err.Error()}, nil
	}
	return ConnectionTest{
		OK:      true,
		Kind:    kind,
		AppName: status.AppName,
		Version: status.Version,
		Message: "ok",
	}, nil
}

func keyedClient(apiKey, authCookie string, template *httpx.Client) *httpx.Client {
	timeout := time.Duration(0)
	if template != nil {
		timeout = template.Timeout
	}
	hc := httpx.New(timeout)
	hc.APIKey = apiKey
	hc.Header = "X-Api-Key"
	hc.Cookie = strings.TrimSpace(authCookie)
	return hc
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

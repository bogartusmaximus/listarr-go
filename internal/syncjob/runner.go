package syncjob

import (
	"context"
	"fmt"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
	"github.com/bogartusmaximus/listarr-go/internal/store"
	"github.com/bogartusmaximus/listarr-go/internal/tmdb"
)

const (
	MaxItemsDefault = 100
	MaxItemsHardCap = 2000
)

// Request is a preview/apply payload (no private defaults).
type Request struct {
	Source         string              `json:"source"` // tmdb | arr-library | listarr-go
	MediaType      string              `json:"mediaType"`
	Discover       *tmdb.DiscoverQuery `json:"discover,omitempty"`
	TMDBIDs        []int               `json:"tmdbIds,omitempty"`
	SourceInstance string              `json:"sourceInstance,omitempty"`
	SourceFilter   arr.LibraryFilter   `json:"sourceFilter,omitempty"`
	CatalogFilter  CatalogSourceFilter `json:"catalogFilter,omitempty"`
	MaxItems       int                 `json:"maxItems,omitempty"`
	Target         arr.Target          `json:"target"`
}

// CatalogSourceFilter selects listarr-go SoT rows for sync export.
type CatalogSourceFilter struct {
	WatchedOnly   bool   `json:"watchedOnly,omitempty"`
	UnwatchedOnly bool   `json:"unwatchedOnly,omitempty"`
	Query         string `json:"query,omitempty"`
}

// ItemResult is one title outcome.
type ItemResult struct {
	TMDBID   int    `json:"tmdbId"`
	Title    string `json:"title"`
	Action   string `json:"action"`
	Detail   string `json:"detail,omitempty"`
	Searched bool   `json:"searched,omitempty"`
}

// Result is the sync summary.
type Result struct {
	DryRun   bool         `json:"dryRun"`
	Items    []ItemResult `json:"items"`
	Adds     int          `json:"adds"`
	Skips    int          `json:"skips"`
	Deferred int          `json:"deferredSearch"`
	Errors   int          `json:"errors"`
}

// Dependencies for sync execution.
type Dependencies struct {
	TMDB         *tmdb.Client
	Arr          *arr.Registry
	SearchBudget *ratelimit.HourlyBudget
	Store        store.Store
}

// Runner executes preview/apply.
type Runner struct {
	Deps Dependencies
}

// Run executes sync. dryRun skips mutations.
func (r *Runner) Run(ctx context.Context, req Request, dryRun bool) (Result, error) {
	if err := validateRequest(req); err != nil {
		return Result{}, err
	}
	items, err := r.resolveItems(ctx, req)
	if err != nil {
		return Result{}, err
	}
	max := req.MaxItems
	if max <= 0 {
		max = MaxItemsDefault
	}
	if max > MaxItemsHardCap {
		max = MaxItemsHardCap
	}
	if len(items) > max {
		items = items[:max]
	}

	out := Result{DryRun: dryRun, Items: make([]ItemResult, 0, len(items))}
	switch req.MediaType {
	case "movie":
		return r.runMovies(ctx, items, req.Target, dryRun, out)
	case "tv":
		return r.runSeries(ctx, items, req.Target, dryRun, out)
	default:
		return Result{}, fmt.Errorf("unsupported mediaType %q", req.MediaType)
	}
}

func validateRequest(req Request) error {
	switch req.Source {
	case "tmdb":
		if req.Discover == nil && len(req.TMDBIDs) == 0 {
			return fmt.Errorf("discover or tmdbIds is required for source=tmdb")
		}
	case "arr-library":
		if req.SourceInstance == "" {
			return fmt.Errorf("sourceInstance is required for source=arr-library")
		}
	case "listarr-go":
		if req.CatalogFilter.WatchedOnly && req.CatalogFilter.UnwatchedOnly {
			return fmt.Errorf("catalogFilter watchedOnly and unwatchedOnly are mutually exclusive")
		}
	default:
		return fmt.Errorf("source must be tmdb, arr-library, or listarr-go")
	}
	if req.MediaType != "movie" && req.MediaType != "tv" {
		return fmt.Errorf("mediaType must be movie or tv")
	}
	if req.Target.RootFolderPath == "" {
		return fmt.Errorf("target.rootFolderPath is required")
	}
	if req.Target.QualityProfileID < 1 {
		return fmt.Errorf("target.qualityProfileId must be >= 1")
	}
	return nil
}

func (r *Runner) resolveItems(ctx context.Context, req Request) ([]tmdb.Item, error) {
	switch req.Source {
	case "tmdb":
		return r.resolveTMDB(ctx, req)
	case "arr-library":
		return r.resolveArrLibrary(ctx, req)
	case "listarr-go":
		return r.resolveListarrCatalog(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported source %q", req.Source)
	}
}

func (r *Runner) resolveListarrCatalog(ctx context.Context, req Request) ([]tmdb.Item, error) {
	if r.Deps.Store == nil {
		return nil, fmt.Errorf("catalog store is not configured")
	}
	filter := store.CatalogFilter{
		MediaType: req.MediaType,
		Query:     req.CatalogFilter.Query,
		Limit:     MaxItemsHardCap,
	}
	switch {
	case req.CatalogFilter.WatchedOnly:
		watched := true
		filter.Watched = &watched
	case req.CatalogFilter.UnwatchedOnly:
		watched := false
		filter.Watched = &watched
	}
	rows, _, err := r.Deps.Store.ListCatalogTitles(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]tmdb.Item, 0, len(rows))
	for _, row := range rows {
		if row.TMDBID < 1 {
			continue
		}
		out = append(out, tmdb.Item{TMDBID: row.TMDBID, MediaType: row.MediaType, Title: row.Title})
	}
	return out, nil
}

func (r *Runner) resolveTMDB(ctx context.Context, req Request) ([]tmdb.Item, error) {
	if len(req.TMDBIDs) > 0 {
		out := make([]tmdb.Item, 0, len(req.TMDBIDs))
		for _, id := range req.TMDBIDs {
			out = append(out, tmdb.Item{TMDBID: id, MediaType: req.MediaType, Title: fmt.Sprintf("tmdb:%d", id)})
		}
		return out, nil
	}
	if r.Deps.TMDB == nil {
		return nil, fmt.Errorf("tmdb client is not configured")
	}
	q := tmdb.DiscoverQuery{}
	if req.Discover != nil {
		q = *req.Discover
	}
	if req.MediaType == "movie" {
		return r.Deps.TMDB.DiscoverMovies(ctx, q)
	}
	return r.Deps.TMDB.DiscoverTV(ctx, q)
}

func (r *Runner) resolveArrLibrary(ctx context.Context, req Request) ([]tmdb.Item, error) {
	if r.Deps.Arr == nil {
		return nil, fmt.Errorf("arr registry is not configured")
	}
	if req.MediaType == "movie" {
		src, err := r.Deps.Arr.Radarr(req.SourceInstance)
		if err != nil {
			return nil, err
		}
		rows, err := src.ExportMovies(ctx, req.SourceFilter)
		if err != nil {
			return nil, err
		}
		out := make([]tmdb.Item, 0, len(rows))
		for _, row := range rows {
			out = append(out, tmdb.Item{TMDBID: row.TMDBID, MediaType: "movie", Title: row.Title})
		}
		return out, nil
	}
	src, err := r.Deps.Arr.Sonarr(req.SourceInstance)
	if err != nil {
		return nil, err
	}
	rows, err := src.ExportSeries(ctx, req.SourceFilter)
	if err != nil {
		return nil, err
	}
	out := make([]tmdb.Item, 0, len(rows))
	for _, row := range rows {
		out = append(out, tmdb.Item{TMDBID: row.TMDBID, MediaType: "tv", Title: row.Title})
	}
	return out, nil
}

func (r *Runner) targetRadarr(target arr.Target) (*arr.Radarr, error) {
	if r.Deps.Arr == nil {
		return nil, fmt.Errorf("arr registry is not configured")
	}
	name := target.Instance
	if name == "" {
		name = "radarr"
	}
	return r.Deps.Arr.Radarr(name)
}

func (r *Runner) targetSonarr(target arr.Target) (*arr.Sonarr, error) {
	if r.Deps.Arr == nil {
		return nil, fmt.Errorf("arr registry is not configured")
	}
	name := target.Instance
	if name == "" {
		name = "sonarr"
	}
	return r.Deps.Arr.Sonarr(name)
}

func (r *Runner) runMovies(ctx context.Context, items []tmdb.Item, target arr.Target, dryRun bool, out Result) (Result, error) {
	dst, err := r.targetRadarr(target)
	if err != nil {
		return Result{}, err
	}
	existing, err := dst.ListMovies(ctx)
	if err != nil {
		return Result{}, err
	}
	for _, item := range items {
		out.Items = append(out.Items, r.handleMovie(ctx, dst, item, existing, target, dryRun))
		tally(&out, out.Items[len(out.Items)-1])
	}
	return out, nil
}

func (r *Runner) handleMovie(ctx context.Context, dst *arr.Radarr, item tmdb.Item, existing map[int]arr.MovieRef, target arr.Target, dryRun bool) ItemResult {
	if _, ok := existing[item.TMDBID]; ok {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: "skip", Detail: "already in target radarr"}
	}
	search, action, detail := r.decideSearch(target.SearchOnAdd, dryRun)
	eff := target
	eff.SearchOnAdd = search
	if dryRun {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: action, Detail: detail, Searched: search}
	}
	lookup, err := dst.LookupByTMDB(ctx, item.TMDBID)
	if err != nil {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: "error", Detail: err.Error()}
	}
	if title, _ := lookup["title"].(string); title != "" {
		item.Title = title
	}
	if err := dst.AddMovie(ctx, lookup, eff); err != nil {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: "error", Detail: err.Error()}
	}
	return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: action, Detail: detail, Searched: search}
}

func (r *Runner) runSeries(ctx context.Context, items []tmdb.Item, target arr.Target, dryRun bool, out Result) (Result, error) {
	dst, err := r.targetSonarr(target)
	if err != nil {
		return Result{}, err
	}
	existing, err := dst.ListSeries(ctx)
	if err != nil {
		return Result{}, err
	}
	for _, item := range items {
		out.Items = append(out.Items, r.handleSeries(ctx, dst, item, existing, target, dryRun))
		tally(&out, out.Items[len(out.Items)-1])
	}
	return out, nil
}

func (r *Runner) handleSeries(ctx context.Context, dst *arr.Sonarr, item tmdb.Item, existing map[int]arr.SeriesRef, target arr.Target, dryRun bool) ItemResult {
	if _, ok := existing[item.TMDBID]; ok {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: "skip", Detail: "already in target sonarr"}
	}
	search, action, detail := r.decideSearch(target.SearchOnAdd, dryRun)
	eff := target
	eff.SearchOnAdd = search
	if dryRun {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: action, Detail: detail, Searched: search}
	}
	lookup, err := dst.LookupByTMDB(ctx, item.TMDBID)
	if err != nil {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: "error", Detail: err.Error()}
	}
	if title, _ := lookup["title"].(string); title != "" {
		item.Title = title
	}
	if err := dst.AddSeries(ctx, lookup, eff); err != nil {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: "error", Detail: err.Error()}
	}
	return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: action, Detail: detail, Searched: search}
}

func (r *Runner) decideSearch(want bool, dryRun bool) (search bool, action, detail string) {
	if !want {
		return false, "add", "searchOnAdd=false"
	}
	if r.Deps.SearchBudget == nil {
		return true, "add", "searchOnAdd=true"
	}
	if dryRun {
		if r.Deps.SearchBudget.Remaining() < 1 {
			return false, "defer_search", "hourly search budget exhausted"
		}
		return true, "add", "searchOnAdd=true"
	}
	if _, ok := r.Deps.SearchBudget.Allow(); !ok {
		return false, "defer_search", "hourly search budget exhausted"
	}
	return true, "add", "searchOnAdd=true"
}

func tally(out *Result, item ItemResult) {
	switch item.Action {
	case "add":
		out.Adds++
	case "skip":
		out.Skips++
	case "defer_search":
		out.Adds++
		out.Deferred++
	case "error":
		out.Errors++
	}
}

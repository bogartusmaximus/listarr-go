package syncjob

import (
	"context"
	"fmt"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
	"github.com/bogartusmaximus/listarr-go/internal/tmdb"
)

const MaxItemsPerRun = 50

// Request is a preview/apply payload (no private defaults).
type Request struct {
	Source    string              `json:"source"`    // tmdb
	MediaType string              `json:"mediaType"` // movie | tv
	Discover  *tmdb.DiscoverQuery `json:"discover,omitempty"`
	TMDBIDs   []int               `json:"tmdbIds,omitempty"`
	MaxItems  int                 `json:"maxItems,omitempty"`
	Target    arr.Target          `json:"target"`
}

// ItemResult is one title outcome.
type ItemResult struct {
	TMDBID   int    `json:"tmdbId"`
	Title    string `json:"title"`
	Action   string `json:"action"` // add | skip | defer_search | error
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

// Dependencies are optional clients; movie sync needs TMDB+Radarr, TV needs TMDB+Sonarr.
type Dependencies struct {
	TMDB         *tmdb.Client
	Radarr       *arr.Radarr
	Sonarr       *arr.Sonarr
	SearchBudget *ratelimit.HourlyBudget
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
	if max <= 0 || max > MaxItemsPerRun {
		max = MaxItemsPerRun
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
	if req.Source != "tmdb" {
		return fmt.Errorf("source must be tmdb")
	}
	if req.MediaType != "movie" && req.MediaType != "tv" {
		return fmt.Errorf("mediaType must be movie or tv")
	}
	if req.Discover == nil && len(req.TMDBIDs) == 0 {
		return fmt.Errorf("discover or tmdbIds is required")
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

func (r *Runner) runMovies(ctx context.Context, items []tmdb.Item, target arr.Target, dryRun bool, out Result) (Result, error) {
	if r.Deps.Radarr == nil {
		return Result{}, fmt.Errorf("radarr client is not configured")
	}
	existing, err := r.Deps.Radarr.ListMovies(ctx)
	if err != nil {
		return Result{}, err
	}
	for _, item := range items {
		out.Items = append(out.Items, r.handleMovie(ctx, item, existing, target, dryRun))
		last := out.Items[len(out.Items)-1]
		tally(&out, last)
	}
	return out, nil
}

func (r *Runner) handleMovie(ctx context.Context, item tmdb.Item, existing map[int]arr.MovieRef, target arr.Target, dryRun bool) ItemResult {
	if _, ok := existing[item.TMDBID]; ok {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: "skip", Detail: "already in radarr"}
	}
	search, action, detail := r.decideSearch(target.SearchOnAdd, dryRun)
	eff := target
	eff.SearchOnAdd = search
	if dryRun {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: action, Detail: detail, Searched: search}
	}
	lookup, err := r.Deps.Radarr.LookupByTMDB(ctx, item.TMDBID)
	if err != nil {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: "error", Detail: err.Error()}
	}
	if title, _ := lookup["title"].(string); title != "" {
		item.Title = title
	}
	if err := r.Deps.Radarr.AddMovie(ctx, lookup, eff); err != nil {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: "error", Detail: err.Error()}
	}
	return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: action, Detail: detail, Searched: search}
}

func (r *Runner) runSeries(ctx context.Context, items []tmdb.Item, target arr.Target, dryRun bool, out Result) (Result, error) {
	if r.Deps.Sonarr == nil {
		return Result{}, fmt.Errorf("sonarr client is not configured")
	}
	existing, err := r.Deps.Sonarr.ListSeries(ctx)
	if err != nil {
		return Result{}, err
	}
	for _, item := range items {
		out.Items = append(out.Items, r.handleSeries(ctx, item, existing, target, dryRun))
		last := out.Items[len(out.Items)-1]
		tally(&out, last)
	}
	return out, nil
}

func (r *Runner) handleSeries(ctx context.Context, item tmdb.Item, existing map[int]arr.SeriesRef, target arr.Target, dryRun bool) ItemResult {
	if _, ok := existing[item.TMDBID]; ok {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: "skip", Detail: "already in sonarr"}
	}
	search, action, detail := r.decideSearch(target.SearchOnAdd, dryRun)
	eff := target
	eff.SearchOnAdd = search
	if dryRun {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: action, Detail: detail, Searched: search}
	}
	lookup, err := r.Deps.Sonarr.LookupByTMDB(ctx, item.TMDBID)
	if err != nil {
		return ItemResult{TMDBID: item.TMDBID, Title: item.Title, Action: "error", Detail: err.Error()}
	}
	if title, _ := lookup["title"].(string); title != "" {
		item.Title = title
	}
	if err := r.Deps.Sonarr.AddSeries(ctx, lookup, eff); err != nil {
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

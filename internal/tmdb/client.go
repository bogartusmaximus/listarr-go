package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/bogartusmaximus/listarr-go/internal/httpx"
)

const DefaultBaseURL = "https://api.themoviedb.org/3"

// Item is a normalized discover result.
type Item struct {
	TMDBID      int     `json:"tmdbId"`
	MediaType   string  `json:"mediaType"` // movie | tv
	Title       string  `json:"title"`
	Year        int     `json:"year,omitempty"`
	Overview    string  `json:"overview,omitempty"`
	VoteAverage float64 `json:"voteAverage,omitempty"`
}

// DiscoverQuery filters TMDB discover endpoints.
type DiscoverQuery struct {
	Page           int     `json:"page,omitempty"`
	SortBy         string  `json:"sortBy,omitempty"`
	Language       string  `json:"language,omitempty"`
	Region         string  `json:"region,omitempty"`
	VoteAverageGte float64 `json:"voteAverageGte,omitempty"`
	VoteCountGte   int     `json:"voteCountGte,omitempty"`
	Year           int     `json:"year,omitempty"`
	WithGenres     string  `json:"withGenres,omitempty"`
	WithoutGenres  string  `json:"withoutGenres,omitempty"`
	IncludeAdult   bool    `json:"includeAdult,omitempty"`
}

// Client talks to TMDB v3.
type Client struct {
	base   string
	apiKey string
	http   *httpx.Client
}

// New builds a TMDB client. baseURL may be empty for the public API host.
func New(apiKey, baseURL string, httpClient *httpx.Client) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("tmdb api key is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = httpx.New(0)
	}
	return &Client{base: baseURL, apiKey: apiKey, http: httpClient}, nil
}

// DiscoverMovies calls /discover/movie.
func (c *Client) DiscoverMovies(ctx context.Context, q DiscoverQuery) ([]Item, error) {
	return c.discover(ctx, "movie", q)
}

// DiscoverTV calls /discover/tv.
func (c *Client) DiscoverTV(ctx context.Context, q DiscoverQuery) ([]Item, error) {
	return c.discover(ctx, "tv", q)
}

func (c *Client) discover(ctx context.Context, media string, q DiscoverQuery) ([]Item, error) {
	u, err := url.Parse(c.base + "/discover/" + media)
	if err != nil {
		return nil, err
	}
	vals := url.Values{}
	vals.Set("api_key", c.apiKey)
	page := q.Page
	if page < 1 {
		page = 1
	}
	vals.Set("page", strconv.Itoa(page))
	if q.SortBy != "" {
		vals.Set("sort_by", q.SortBy)
	} else {
		vals.Set("sort_by", "popularity.desc")
	}
	if q.Language != "" {
		vals.Set("language", q.Language)
	}
	if q.Region != "" {
		vals.Set("region", q.Region)
	}
	if q.VoteAverageGte > 0 {
		vals.Set("vote_average.gte", strconv.FormatFloat(q.VoteAverageGte, 'f', -1, 64))
	}
	if q.VoteCountGte > 0 {
		vals.Set("vote_count.gte", strconv.Itoa(q.VoteCountGte))
	}
	if q.Year > 0 {
		if media == "movie" {
			vals.Set("primary_release_year", strconv.Itoa(q.Year))
		} else {
			vals.Set("first_air_date_year", strconv.Itoa(q.Year))
		}
	}
	if q.WithGenres != "" {
		vals.Set("with_genres", q.WithGenres)
	}
	if q.WithoutGenres != "" {
		vals.Set("without_genres", q.WithoutGenres)
	}
	vals.Set("include_adult", strconv.FormatBool(q.IncludeAdult))
	u.RawQuery = vals.Encode()

	resp, body, err := c.http.DoJSON(ctx, http.MethodGet, u.String(), nil, nil)
	if err != nil {
		return nil, err
	}
	if err := httpx.CheckStatus(resp, u.String(), body); err != nil {
		return nil, err
	}
	return parseDiscover(media, body)
}

type discoverResponse struct {
	Results []discoverResult `json:"results"`
}

type discoverResult struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Name         string  `json:"name"`
	Overview     string  `json:"overview"`
	VoteAverage  float64 `json:"vote_average"`
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"`
}

func parseDiscover(media string, body []byte) ([]Item, error) {
	var payload discoverResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("tmdb decode: %w", err)
	}
	out := make([]Item, 0, len(payload.Results))
	for _, r := range payload.Results {
		title := r.Title
		yearSrc := r.ReleaseDate
		if media == "tv" {
			title = r.Name
			yearSrc = r.FirstAirDate
		}
		out = append(out, Item{
			TMDBID:      r.ID,
			MediaType:   media,
			Title:       title,
			Year:        yearFromDate(yearSrc),
			Overview:    r.Overview,
			VoteAverage: r.VoteAverage,
		})
	}
	return out, nil
}

func yearFromDate(s string) int {
	if len(s) < 4 {
		return 0
	}
	y, err := strconv.Atoi(s[:4])
	if err != nil {
		return 0
	}
	return y
}

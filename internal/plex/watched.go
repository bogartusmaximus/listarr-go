package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/httpx"
)

const (
	watchedPageSize = 100
	watchedMaxPages = 50 // hard cap: 5000 items per section type
)

// WatchedItem is one Plex library item with GUID identity and view state.
type WatchedItem struct {
	RatingKey    string     `json:"ratingKey"`
	Title        string     `json:"title"`
	Type         string     `json:"type"` // movie|show
	TMDBID       int        `json:"tmdbId,omitempty"`
	IMDBID       string     `json:"imdbId,omitempty"`
	ViewCount    int        `json:"viewCount"`
	LastViewedAt *time.Time `json:"lastViewedAt,omitempty"`
}

// ListWatched returns movies/shows that have viewCount > 0 (or all when includeUnwatched).
// mediaType is movie|tv|"" (both). Capped pages per section.
func (c *Client) ListWatched(ctx context.Context, mediaType string, includeUnwatched bool) ([]WatchedItem, error) {
	if c.serverURL == "" {
		return nil, fmt.Errorf("plex serverUrl is required")
	}
	if c.token == "" {
		return nil, fmt.Errorf("plex token is required")
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	wantMovie := mediaType == "" || mediaType == "movie"
	wantShow := mediaType == "" || mediaType == "tv" || mediaType == "show"

	sections, err := c.listSections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]WatchedItem, 0, 128)
	for _, sec := range sections {
		kind := strings.ToLower(sec.Type)
		var plexType int
		var outType string
		switch {
		case kind == "movie" && wantMovie:
			plexType = 1
			outType = "movie"
		case kind == "show" && wantShow:
			plexType = 2
			outType = "show"
		default:
			continue
		}
		items, err := c.listSectionItems(ctx, sec.Key, plexType, outType, includeUnwatched)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

type plexSection struct {
	Key  string
	Type string
}

func (c *Client) listSections(ctx context.Context) ([]plexSection, error) {
	rawURL := c.serverURL + "/library/sections"
	resp, body, err := c.http.DoJSON(ctx, http.MethodGet, rawURL, nil, c.plexHeaders())
	if err != nil {
		return nil, err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return nil, err
	}
	var payload struct {
		MediaContainer struct {
			Directory []struct {
				Key  string `json:"key"`
				Type string `json:"type"`
			} `json:"Directory"`
		} `json:"MediaContainer"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("plex decode sections: %w", err)
	}
	out := make([]plexSection, 0, len(payload.MediaContainer.Directory))
	for _, d := range payload.MediaContainer.Directory {
		if strings.TrimSpace(d.Key) == "" {
			continue
		}
		out = append(out, plexSection{Key: d.Key, Type: d.Type})
	}
	return out, nil
}

func (c *Client) listSectionItems(ctx context.Context, sectionKey string, plexType int, outType string, includeUnwatched bool) ([]WatchedItem, error) {
	out := make([]WatchedItem, 0, watchedPageSize)
	for page := 0; page < watchedMaxPages; page++ {
		start := page * watchedPageSize
		q := url.Values{}
		q.Set("type", strconv.Itoa(plexType))
		q.Set("includeGuids", "1")
		q.Set("X-Plex-Container-Start", strconv.Itoa(start))
		q.Set("X-Plex-Container-Size", strconv.Itoa(watchedPageSize))
		rawURL := fmt.Sprintf("%s/library/sections/%s/all?%s", c.serverURL, url.PathEscape(sectionKey), q.Encode())
		resp, body, err := c.http.DoJSON(ctx, http.MethodGet, rawURL, nil, c.plexHeaders())
		if err != nil {
			return nil, err
		}
		if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
			return nil, err
		}
		var payload struct {
			MediaContainer struct {
				Metadata []struct {
					RatingKey    string `json:"ratingKey"`
					Title        string `json:"title"`
					Type         string `json:"type"`
					ViewCount    int    `json:"viewCount"`
					LastViewedAt int64  `json:"lastViewedAt"`
					Guid         []struct {
						ID string `json:"id"`
					} `json:"Guid"`
				} `json:"Metadata"`
			} `json:"MediaContainer"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("plex decode library items: %w", err)
		}
		batch := payload.MediaContainer.Metadata
		if len(batch) == 0 {
			break
		}
		for _, meta := range batch {
			if !includeUnwatched && meta.ViewCount < 1 {
				continue
			}
			item := WatchedItem{
				RatingKey: meta.RatingKey,
				Title:     meta.Title,
				Type:      outType,
				ViewCount: meta.ViewCount,
			}
			if meta.LastViewedAt > 0 {
				t := time.Unix(meta.LastViewedAt, 0).UTC()
				item.LastViewedAt = &t
			}
			item.TMDBID, item.IMDBID = parsePlexGuids(meta.Guid)
			if item.TMDBID < 1 && item.IMDBID == "" {
				continue
			}
			out = append(out, item)
		}
		if len(batch) < watchedPageSize {
			break
		}
	}
	return out, nil
}

func parsePlexGuids(guids []struct {
	ID string `json:"id"`
}) (tmdbID int, imdbID string) {
	for _, g := range guids {
		id := strings.TrimSpace(g.ID)
		switch {
		case strings.HasPrefix(id, "tmdb://"):
			n, _ := strconv.Atoi(strings.TrimPrefix(id, "tmdb://"))
			if n > 0 {
				tmdbID = n
			}
		case strings.HasPrefix(id, "imdb://"):
			imdbID = strings.TrimPrefix(id, "imdb://")
		}
	}
	return tmdbID, imdbID
}

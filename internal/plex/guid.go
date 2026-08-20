package plex

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// plexGUIDList accepts Plex Guid as an array of {id}, a single {id}, a string, or []string.
type plexGUIDList []string

func (g *plexGUIDList) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*g = nil
		return nil
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		*g = []string{s}
		return nil
	case '{':
		id, err := guidObjectID(raw)
		if err != nil {
			return err
		}
		*g = []string{id}
		return nil
	case '[':
		return g.unmarshalArray(raw)
	default:
		return json.Unmarshal(raw, (*[]string)(g))
	}
}

func (g *plexGUIDList) unmarshalArray(raw []byte) error {
	var objs []json.RawMessage
	if err := json.Unmarshal(raw, &objs); err != nil {
		return err
	}
	out := make([]string, 0, len(objs))
	for _, item := range objs {
		item = bytes.TrimSpace(item)
		if len(item) == 0 {
			continue
		}
		if item[0] == '"' {
			var s string
			if err := json.Unmarshal(item, &s); err != nil {
				return err
			}
			out = append(out, s)
			continue
		}
		id, err := guidObjectID(item)
		if err != nil {
			return err
		}
		out = append(out, id)
	}
	*g = out
	return nil
}

func guidObjectID(raw []byte) (string, error) {
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", err
	}
	return obj.ID, nil
}

func parsePlexGuids(ids []string, extra ...string) (tmdbID int, imdbID string) {
	for _, id := range append(append([]string{}, ids...), extra...) {
		t, i := parseOnePlexGUID(id)
		if t > 0 {
			tmdbID = t
		}
		if i != "" {
			imdbID = i
		}
	}
	return tmdbID, imdbID
}

func parseOnePlexGUID(raw string) (tmdbID int, imdbID string) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return 0, ""
	}
	if q := strings.IndexByte(id, '?'); q >= 0 {
		id = id[:q]
	}
	switch {
	case strings.HasPrefix(id, "tmdb://"):
		return atoiPositive(strings.TrimPrefix(id, "tmdb://")), ""
	case strings.HasPrefix(id, "imdb://"):
		return 0, strings.TrimPrefix(id, "imdb://")
	case strings.Contains(id, "themoviedb://"):
		return atoiPositive(afterScheme(id, "themoviedb://")), ""
	case strings.Contains(id, "imdb://"):
		return 0, afterScheme(id, "imdb://")
	default:
		return 0, ""
	}
}

func afterScheme(raw, scheme string) string {
	i := strings.Index(raw, scheme)
	if i < 0 {
		return ""
	}
	return raw[i+len(scheme):]
}

func atoiPositive(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	if n < 1 {
		return 0
	}
	return n
}

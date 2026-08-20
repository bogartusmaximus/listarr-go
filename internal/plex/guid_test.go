package plex

import (
	"encoding/json"
	"testing"
)

func TestPlexGUIDListShapes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "array objects", raw: `[{"id":"tmdb://1"},{"id":"imdb://tt2"}]`, want: []string{"tmdb://1", "imdb://tt2"}},
		{name: "string", raw: `"tmdb://550"`, want: []string{"tmdb://550"}},
		{name: "single object", raw: `{"id":"imdb://tt0137523"}`, want: []string{"imdb://tt0137523"}},
		{name: "array strings", raw: `["tmdb://9"]`, want: []string{"tmdb://9"}},
		{name: "null", raw: `null`, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got plexGUIDList
			if err := json.Unmarshal([]byte(tc.raw), &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got=%v want=%v", got, tc.want)
				}
			}
		})
	}
}

func TestParseOnePlexGUID(t *testing.T) {
	tmdb, imdb := parseOnePlexGUID("com.plexapp.agents.themoviedb://550?lang=en")
	if tmdb != 550 {
		t.Fatalf("tmdb=%d", tmdb)
	}
	_, imdb = parseOnePlexGUID("com.plexapp.agents.imdb://tt0137523?lang=en")
	if imdb != "tt0137523" {
		t.Fatalf("imdb=%q", imdb)
	}
}

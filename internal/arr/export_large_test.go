package arr_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/httpx"
)

func TestExportMoviesLargeJSONWithCookie(t *testing.T) {
	cookie := `_oauth2_proxy=example-token|123|sig`
	var gotCookie, gotAccept string
	pad := strings.Repeat("x", 9<<20)
	payload := fmt.Sprintf(
		`[{"title":"X","tmdbId":1,"monitored":true,"tags":[],"path":"/m/X","overview":%q}]`,
		pad,
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		gotAccept = r.Header.Get("Accept")
		if r.URL.Path != "/api/v3/movie" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	client, err := arr.NewRadarr(srv.URL, "k", cookie, httpx.New(0))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := client.ExportMovies(context.Background(), arr.LibraryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TMDBID != 1 {
		t.Fatalf("%+v", rows)
	}
	if gotCookie != cookie {
		t.Fatalf("cookie=%q", gotCookie)
	}
	if gotAccept != "application/json" {
		t.Fatalf("Accept=%q", gotAccept)
	}
}

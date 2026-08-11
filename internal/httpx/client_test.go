package httpx_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/httpx"
)

func TestDoJSONReadsLargeBodies(t *testing.T) {
	// Previously capped at 8MiB; elfhosted movie libraries exceed that (~20MiB).
	pad := strings.Repeat("x", 9<<20)
	payload := fmt.Sprintf(`[{"overview":%q,"tmdbId":1}]`, pad)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	c := httpx.New(0)
	_, body, err := c.DoJSON(context.Background(), http.MethodGet, srv.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != len(payload) {
		t.Fatalf("got %d want %d", len(body), len(payload))
	}
}

func TestDoJSONSetsAcceptJSON(t *testing.T) {
	var accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	c := httpx.New(0)
	_, _, err := c.DoJSON(context.Background(), http.MethodGet, srv.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if accept != "application/json" {
		t.Fatalf("Accept=%q", accept)
	}
}

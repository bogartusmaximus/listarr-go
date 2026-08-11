package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/api"
	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
	"github.com/bogartusmaximus/listarr-go/internal/store"
)

func TestHealthNoAuth(t *testing.T) {
	srv := api.New(api.Config{APIKey: "secret"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestUIIndexNoAuth(t *testing.T) {
	srv := api.New(api.Config{APIKey: "secret"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type=%q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "listarr-go") || !strings.Contains(body, "/assets/app.js") {
		t.Fatalf("unexpected html: %s", body[:min(200, len(body))])
	}
}

func TestUIAssetsNoAuth(t *testing.T) {
	srv := api.New(api.Config{APIKey: "secret"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatusRequiresKeyAndReportsBudget(t *testing.T) {
	budget := ratelimit.NewHourlyBudget(60)
	srv := api.New(api.Config{
		APIKey:       "secret",
		InstanceName: "test",
		ApplyEnabled: false,
		SearchBudget: budget,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=401", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	req.Header.Set("X-Api-Key", "secret")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["applyEnabled"] != false {
		t.Fatalf("applyEnabled=%v", body["applyEnabled"])
	}
	if body["torboxSearchPerHour"] != float64(60) {
		t.Fatalf("perHour=%v", body["torboxSearchPerHour"])
	}
}

func TestApplyDisabledReturnsForbidden(t *testing.T) {
	srv := api.New(api.Config{APIKey: "secret", ApplyEnabled: false})
	rec := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"source":"tmdb","mediaType":"movie","tmdbIds":[1],"target":{"rootFolderPath":"/data","qualityProfileId":1}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/apply", body)
	req.Header.Set("X-Api-Key", "secret")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=403", rec.Code)
	}
}

func TestDiscoverWithoutTMDBUnavailable(t *testing.T) {
	srv := api.New(api.Config{APIKey: "secret"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover/movies", nil)
	req.Header.Set("X-Api-Key", "secret")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestActivityListsPolarsRuns(t *testing.T) {
	st, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SaveSyncRun(context.Background(), store.SyncRun{
		DryRun:    true,
		Source:    "arr-library",
		MediaType: "movie",
		Adds:      3,
		CreatedAt: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	srv := api.New(api.Config{APIKey: "secret", Store: st})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?limit=10", nil)
	req.Header.Set("X-Api-Key", "secret")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["backend"] != "polars" {
		t.Fatalf("backend=%v", body["backend"])
	}
	runs, ok := body["runs"].([]any)
	if !ok || len(runs) != 1 {
		t.Fatalf("runs=%v", body["runs"])
	}
}

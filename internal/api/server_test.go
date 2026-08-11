package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/api"
	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
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

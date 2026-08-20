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
	"github.com/bogartusmaximus/listarr-go/internal/appstate"
	"github.com/bogartusmaximus/listarr-go/internal/arr"
	"github.com/bogartusmaximus/listarr-go/internal/ratelimit"
	"github.com/bogartusmaximus/listarr-go/internal/store"
)

func testRT(apiKey string) *appstate.Runtime {
	return &appstate.Runtime{
		APIKey:       apiKey,
		InstanceName: "test",
		SafeMode:     true,
		Settings: store.Settings{
			APIKey:              apiKey,
			InstanceName:        "test",
			SafeMode:            true,
			TorboxSearchPerHour: 60,
			ArrInstances:        []store.ArrInstanceSettings{},
		},
	}
}

func TestUIBootstrapNoAuth(t *testing.T) {
	srv := api.New(testRT("secret-key"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ui/bootstrap", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["apiKey"] != "secret-key" {
		t.Fatalf("%+v", body)
	}
}

func TestHealthNoAuth(t *testing.T) {
	srv := api.New(testRT("secret"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestUIIndexNoAuth(t *testing.T) {
	srv := api.New(testRT("secret"))
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
	if !strings.Contains(body, "listarr-go") || !strings.Contains(body, "/assets/app.js") || !strings.Contains(body, "Settings") {
		t.Fatalf("unexpected html: %s", body[:min(200, len(body))])
	}
	if strings.Contains(body, "btnConnect") || strings.Contains(body, `id="apiKey"`) {
		t.Fatal("connect/API key paste UI should be removed")
	}
	for _, want := range []string{`id="stPlex"`, `id="syncSourceAdvanced"`, `id="syncTargetAdvanced"`, `id="syncBusy"`, `id="settingsHint"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("html missing %s", want)
		}
	}
	if strings.Contains(body, `id="btnSaveSettings"`) {
		t.Fatal("save button should be removed; settings autosave")
	}
	if !strings.Contains(body, `id="syncRootCustom"`) || !strings.Contains(body, `<select name="rootFolderPath" id="syncRoot"`) {
		t.Fatal("root folder should be a visible select with a custom-path input")
	}
	if strings.Contains(body, `id="syncRootList"`) || strings.Contains(body, "<datalist") {
		t.Fatal("root folder datalist should be removed")
	}
}

func TestUIAssetsNoAuth(t *testing.T) {
	srv := api.New(testRT("secret"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	css := rec.Body.String()
	if !strings.Contains(css, "align-content: start") || !strings.Contains(css, "align-items: start") {
		t.Fatal("sync fieldsets must pack content at the start so Advanced-off does not stretch gaps")
	}
}

func TestStatusRequiresKeyAndReportsBudget(t *testing.T) {
	budget := ratelimit.NewHourlyBudget(60)
	rt := testRT("secret")
	rt.SearchBudget = budget
	srv := api.New(rt)

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
	if body["safeMode"] != true {
		t.Fatalf("safeMode=%v", body["safeMode"])
	}
	if body["applyEnabled"] != false {
		t.Fatalf("applyEnabled=%v", body["applyEnabled"])
	}
	if body["torboxSearchPerHour"] != float64(60) {
		t.Fatalf("perHour=%v", body["torboxSearchPerHour"])
	}
	if _, ok := body["apiKey"]; ok {
		t.Fatal("status must not echo apiKey")
	}
	if _, ok := body["tmdbApiKey"]; ok {
		t.Fatal("status must not echo tmdbApiKey")
	}
	if body["plexConfigured"] != false {
		t.Fatalf("plexConfigured=%v", body["plexConfigured"])
	}
	if _, ok := body["plexToken"]; ok {
		t.Fatal("status must not echo plexToken")
	}
}

func TestStatusReportsPlexConfigured(t *testing.T) {
	rt := testRT("secret")
	rt.Settings.Plex.Token = "tok"
	rt.Settings.Plex.AccountUsername = "bogart"
	srv := api.New(rt)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	req.Header.Set("X-Api-Key", "secret")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["plexConfigured"] != true {
		t.Fatalf("plexConfigured=%v", body["plexConfigured"])
	}
	if body["plexAccountUsername"] != "bogart" {
		t.Fatalf("plexAccountUsername=%v", body["plexAccountUsername"])
	}
}

func TestSafeModeBlocksApply(t *testing.T) {
	srv := api.New(testRT("secret"))
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
	srv := api.New(testRT("secret"))
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

	rt := testRT("secret")
	rt.Store = st
	srv := api.New(rt)
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

func TestSettingsGetPutRoundTrip(t *testing.T) {
	st, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	budget := ratelimit.NewHourlyBudget(60)
	rt := testRT("secret")
	rt.Store = st
	rt.SearchBudget = budget
	srv := api.New(rt)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=401", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.Header.Set("X-Api-Key", "secret")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rec.Code, rec.Body.String())
	}

	payload := store.Settings{
		APIKey:              "secret-2",
		InstanceName:        "homelab",
		SafeMode:            false,
		TorboxSearchPerHour: 42,
		TMDBAPIKey:          "tmdb-key",
		ArrInstances:        []store.ArrInstanceSettings{},
	}
	raw, _ := json.Marshal(payload)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(raw))
	req.Header.Set("X-Api-Key", "secret")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", rec.Code, rec.Body.String())
	}
	var saved store.Settings
	if err := json.NewDecoder(rec.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.APIKey != "secret-2" || saved.TMDBAPIKey != "tmdb-key" || saved.SafeMode {
		t.Fatalf("%+v", saved)
	}
	if budget.Limit() != 42 {
		t.Fatalf("budget limit=%d", budget.Limit())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.Header.Set("X-Api-Key", "secret")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old key should fail: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.Header.Set("X-Api-Key", "secret-2")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("new key status=%d", rec.Code)
	}

	got, found, err := st.GetSettings(context.Background())
	if err != nil || !found || got.InstanceName != "homelab" {
		t.Fatalf("store found=%v got=%+v err=%v", found, got, err)
	}
}

func TestArrOptionsListsRootsAndProfiles(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/rootfolder":
			_, _ = w.Write([]byte(`[{"path":"/data/movies"}]`))
		case "/api/v3/qualityprofile":
			_, _ = w.Write([]byte(`[{"id":4,"name":"HD-1080p"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)

	client, err := arr.NewRadarr(upstream.URL, "k", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	reg := arr.NewRegistry()
	if err := reg.RegisterRadarr("mediavault-radarr", client); err != nil {
		t.Fatal(err)
	}
	rt := testRT("secret")
	rt.Arr = reg
	srv := api.New(rt)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/arr/mediavault-radarr/options", nil)
	req.Header.Set("X-Api-Key", "secret")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["instance"] != "mediavault-radarr" || body["kind"] != "radarr" {
		t.Fatalf("%+v", body)
	}
	roots, _ := body["rootFolders"].([]any)
	if len(roots) != 1 {
		t.Fatalf("rootFolders=%v", body["rootFolders"])
	}
	profiles, _ := body["qualityProfiles"].([]any)
	if len(profiles) != 1 {
		t.Fatalf("qualityProfiles=%v", body["qualityProfiles"])
	}
}

func TestArrTestValidation(t *testing.T) {
	srv := api.New(testRT("secret"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/arr/test", bytes.NewBufferString(`{"kind":"lidarr","url":"http://127.0.0.1:7878","apiKey":"k"}`))
	req.Header.Set("X-Api-Key", "secret")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSettingsPutValidation(t *testing.T) {
	st, err := store.Open(store.Config{Backend: store.BackendPolars, PolarsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	rt := testRT("secret")
	rt.Store = st
	srv := api.New(rt)

	raw, _ := json.Marshal(store.Settings{
		APIKey:              "",
		InstanceName:        "x",
		TorboxSearchPerHour: 60,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(raw))
	req.Header.Set("X-Api-Key", "secret")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=400", rec.Code)
	}
}

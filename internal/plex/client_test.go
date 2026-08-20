package plex_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/httpx"
	"github.com/bogartusmaximus/listarr-go/internal/plex"
)

func TestAuthURL(t *testing.T) {
	url := plex.AuthURL("cid-1", "ABCD")
	if !strings.Contains(url, "clientID=cid-1") || !strings.Contains(url, "code=ABCD") {
		t.Fatalf("auth url=%s", url)
	}
}

func TestListLibraryPaths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("X-Plex-Token") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{
			"MediaContainer": {
				"Directory": [
					{"key":"1","title":"Movies","type":"movie","Location":[{"path":"/data/movies"},{"path":"/data/movies-4k"}]},
					{"key":"2","title":"TV","type":"show","Location":[{"path":"/data/tv"}]},
					{"key":"3","title":"Music","type":"artist","Location":[{"path":"/data/music"}]}
				]
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	client, err := plex.New(srv.URL, "tok", "cid", httpx.New(0))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := client.ListLibraryPaths(context.Background(), "movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0].Path != "/data/movies" || paths[1].Path != "/data/movies-4k" {
		t.Fatalf("%+v", paths)
	}
	all, err := client.ListLibraryPaths(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("want 4 got %+v", all)
	}
}

func TestListLibraryPathsSingleLocationObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{
			"MediaContainer": {
				"Directory": {
					"key":"1",
					"title":"Movies",
					"type":"movie",
					"Location":{"path":"/data/movies-4k"}
				}
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	client, err := plex.New(srv.URL, "tok", "cid", httpx.New(0))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := client.ListLibraryPaths(context.Background(), "movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0].Path != "/data/movies-4k" || paths[0].SectionTitle != "Movies" {
		t.Fatalf("%+v", paths)
	}
}

func TestListLibraryPathsStringLocation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{
			"MediaContainer": {
				"Directory": [
					{"key":"1","title":"Movies","type":"movie","Location":"/mnt/media/movies"}
				]
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	client, err := plex.New(srv.URL, "tok", "cid", httpx.New(0))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := client.ListLibraryPaths(context.Background(), "movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0].Path != "/mnt/media/movies" {
		t.Fatalf("%+v", paths)
	}
}

func TestTestConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/identity" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"MediaContainer":{"friendlyName":"Homelab Plex","machineIdentifier":"abc"}}`))
	}))
	t.Cleanup(srv.Close)

	client, err := plex.New(srv.URL, "tok", "cid", nil)
	if err != nil {
		t.Fatal(err)
	}
	name, err := client.TestConnection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if name != "Homelab Plex" {
		t.Fatalf("name=%q", name)
	}
}

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/appstate"
	"github.com/bogartusmaximus/listarr-go/internal/httpx"
	"github.com/bogartusmaximus/listarr-go/internal/plex"
	"github.com/bogartusmaximus/listarr-go/internal/store"
)

func (s *Server) handlePlexPinCreate(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	set := view.Settings
	if strings.TrimSpace(set.Plex.ClientIdentifier) == "" {
		id, err := appstate.GenerateClientID()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
			return
		}
		set.Plex.ClientIdentifier = id
		if err := s.persistSettings(r, set); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		set = s.rt.View().Settings
	}

	client, err := plex.New(set.Plex.ServerURL, set.Plex.Token, set.Plex.ClientIdentifier, s.rt.HTTPClient)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	pin, err := client.CreatePin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":               pin.ID,
		"code":             pin.Code,
		"authUrl":          plex.AuthURL(set.Plex.ClientIdentifier, pin.Code),
		"clientIdentifier": set.Plex.ClientIdentifier,
	})
}

func (s *Server) handlePlexPinPoll(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid pin id"})
		return
	}
	set := view.Settings
	if strings.TrimSpace(set.Plex.ClientIdentifier) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "plex client identifier missing; start PIN auth first"})
		return
	}
	client, err := plex.New(set.Plex.ServerURL, set.Plex.Token, set.Plex.ClientIdentifier, s.rt.HTTPClient)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	token, err := client.CheckPin(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
		return
	}
	if token == "" {
		writeJSON(w, http.StatusOK, map[string]any{"linked": false})
		return
	}

	set.Plex.Token = token
	identClient, err := plex.New(set.Plex.ServerURL, token, set.Plex.ClientIdentifier, s.rt.HTTPClient)
	if err == nil {
		if name, err := identClient.AccountIdentity(r.Context()); err == nil {
			set.Plex.AccountUsername = name
		}
	}
	if err := s.persistSettings(r, set); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	got := s.rt.View().Settings.Plex
	writeJSON(w, http.StatusOK, map[string]any{
		"linked":           true,
		"accountUsername":  got.AccountUsername,
		"clientIdentifier": got.ClientIdentifier,
	})
}

func (s *Server) handlePlexUnlink(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	if view.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "store is not configured"})
		return
	}
	set := view.Settings
	set.Plex.Token = ""
	set.Plex.AccountUsername = ""
	if err := s.persistSettings(r, set); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"linked": false})
}

func (s *Server) handlePlexTest(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	set := view.Settings
	var body store.PlexSettings
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	serverURL := strings.TrimSpace(body.ServerURL)
	token := strings.TrimSpace(body.Token)
	if serverURL == "" {
		serverURL = set.Plex.ServerURL
	}
	if token == "" {
		token = set.Plex.Token
	}
	client, err := plex.New(serverURL, token, set.Plex.ClientIdentifier, s.httpClient())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	name, err := client.TestConnection(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "serverName": name})
}

func (s *Server) handlePlexLibraries(w http.ResponseWriter, r *http.Request) {
	view := s.rt.View()
	set := view.Settings
	client, err := plex.New(set.Plex.ServerURL, set.Plex.Token, set.Plex.ClientIdentifier, s.httpClient())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	media := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mediaType")))
	typeFilter := ""
	switch media {
	case "movie", "movies":
		typeFilter = "movie"
	case "tv", "show", "shows":
		typeFilter = "show"
	case "":
		typeFilter = ""
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "mediaType must be movie, tv, or empty"})
		return
	}
	paths, err := client.ListLibraryPaths(r.Context(), typeFilter)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"libraries": paths})
}

func (s *Server) persistSettings(r *http.Request, set store.Settings) error {
	view := s.rt.View()
	if view.Store == nil {
		return fmt.Errorf("store is not configured")
	}
	set = appstate.Normalize(set)
	if err := appstate.Validate(set); err != nil {
		return err
	}
	set.UpdatedAt = time.Now().UTC()
	if err := s.rt.Apply(set); err != nil {
		return err
	}
	return view.Store.PutSettings(r.Context(), set)
}

func (s *Server) httpClient() *httpx.Client {
	if s.rt != nil {
		return s.rt.HTTPClient
	}
	return nil
}

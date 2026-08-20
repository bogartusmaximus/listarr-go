package plex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/bogartusmaximus/listarr-go/internal/httpx"
)

const (
	plexTVPins     = "https://plex.tv/api/v2/pins"
	plexProduct    = "listarr-go"
	plexDeviceName = "listarr-go"
)

// Client talks to plex.tv (PIN auth) and a Plex Media Server.
type Client struct {
	http      *httpx.Client
	serverURL string
	token     string
	clientID  string
}

// New builds a PMS client. serverURL and token may be empty for PIN-only use.
func New(serverURL, token, clientID string, httpClient *httpx.Client) (*Client, error) {
	if httpClient == nil {
		httpClient = httpx.New(0)
	}
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL != "" {
		u, err := url.Parse(serverURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("plex serverUrl must be an absolute URL")
		}
	}
	return &Client{
		http:      httpClient,
		serverURL: serverURL,
		token:     strings.TrimSpace(token),
		clientID:  strings.TrimSpace(clientID),
	}, nil
}

// Pin is a plex.tv link PIN awaiting user approval.
type Pin struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
}

// AuthURL is the browser link the operator opens to approve the PIN.
func AuthURL(clientID, code string) string {
	q := url.Values{}
	q.Set("clientID", clientID)
	q.Set("code", code)
	q.Set("context[device][product]", plexProduct)
	return "https://app.plex.tv/auth#?" + q.Encode()
}

func (c *Client) plexHeaders() http.Header {
	h := make(http.Header)
	h.Set("Accept", "application/json")
	h.Set("X-Plex-Product", plexProduct)
	h.Set("X-Plex-Version", "0.6.2")
	h.Set("X-Plex-Client-Identifier", c.clientID)
	h.Set("X-Plex-Device", plexDeviceName)
	h.Set("X-Plex-Device-Name", plexDeviceName)
	h.Set("X-Plex-Platform", "Go")
	h.Set("X-Plex-Provides", "controller")
	if c.token != "" {
		h.Set("X-Plex-Token", c.token)
	}
	return h
}

// CreatePin starts a strong PIN login against plex.tv.
func (c *Client) CreatePin(ctx context.Context) (Pin, error) {
	if c.clientID == "" {
		return Pin{}, fmt.Errorf("plex client identifier is required")
	}
	rawURL := plexTVPins + "?strong=true"
	resp, body, err := c.http.DoJSON(ctx, http.MethodPost, rawURL, nil, c.plexHeaders())
	if err != nil {
		return Pin{}, err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return Pin{}, err
	}
	var pin Pin
	if err := json.Unmarshal(body, &pin); err != nil {
		return Pin{}, fmt.Errorf("plex decode pin: %w", err)
	}
	if pin.ID == 0 || pin.Code == "" {
		return Pin{}, fmt.Errorf("plex pin response missing id/code")
	}
	return pin, nil
}

// CheckPin polls a PIN. authToken is empty until the operator links the device.
func (c *Client) CheckPin(ctx context.Context, id int64) (authToken string, err error) {
	if c.clientID == "" {
		return "", fmt.Errorf("plex client identifier is required")
	}
	if id < 1 {
		return "", fmt.Errorf("plex pin id is required")
	}
	rawURL := fmt.Sprintf("%s/%d", plexTVPins, id)
	resp, body, err := c.http.DoJSON(ctx, http.MethodGet, rawURL, nil, c.plexHeaders())
	if err != nil {
		return "", err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return "", err
	}
	var payload struct {
		AuthToken string `json:"authToken"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("plex decode pin status: %w", err)
	}
	return strings.TrimSpace(payload.AuthToken), nil
}

// AccountIdentity returns a display name for the linked token (best-effort).
func (c *Client) AccountIdentity(ctx context.Context) (string, error) {
	if c.token == "" {
		return "", fmt.Errorf("plex token is required")
	}
	rawURL := "https://plex.tv/api/v2/user"
	resp, body, err := c.http.DoJSON(ctx, http.MethodGet, rawURL, nil, c.plexHeaders())
	if err != nil {
		return "", err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return "", err
	}
	var user struct {
		Username string `json:"username"`
		Title    string `json:"title"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		return "", fmt.Errorf("plex decode user: %w", err)
	}
	for _, v := range []string{user.Username, user.Title, user.Email} {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), nil
		}
	}
	return "", nil
}

// LibraryPath is one folder path from a Plex library section.
type LibraryPath struct {
	SectionID    string `json:"sectionId"`
	SectionTitle string `json:"sectionTitle"`
	Type         string `json:"type"` // movie|show|artist|photo|…
	Path         string `json:"path"`
}

// ListLibraryPaths returns unique library folder paths from the configured PMS.
func (c *Client) ListLibraryPaths(ctx context.Context, typeFilter string) ([]LibraryPath, error) {
	if c.serverURL == "" {
		return nil, fmt.Errorf("plex serverUrl is required")
	}
	if c.token == "" {
		return nil, fmt.Errorf("plex token is required")
	}
	typeFilter = strings.ToLower(strings.TrimSpace(typeFilter))
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
			Directory json.RawMessage `json:"Directory"`
		} `json:"MediaContainer"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("plex decode library sections: %w", err)
	}
	dirs, err := decodePlexDirectories(payload.MediaContainer.Directory)
	if err != nil {
		return nil, err
	}
	return collectLibraryPaths(dirs, typeFilter)
}

type plexDirectory struct {
	Key      string          `json:"key"`
	Title    string          `json:"title"`
	Type     string          `json:"type"`
	Location json.RawMessage `json:"Location"`
}

type plexLocation struct {
	Path string `json:"path"`
}

func decodePlexDirectories(raw json.RawMessage) ([]plexDirectory, error) {
	return decodeJSONList[plexDirectory](raw)
}

func collectLibraryPaths(dirs []plexDirectory, typeFilter string) ([]LibraryPath, error) {
	out := make([]LibraryPath, 0)
	seen := map[string]struct{}{}
	for _, dir := range dirs {
		kind := strings.ToLower(strings.TrimSpace(dir.Type))
		if typeFilter != "" && kind != typeFilter {
			continue
		}
		locs, err := decodePlexLocations(dir.Location)
		if err != nil {
			return nil, err
		}
		for _, loc := range locs {
			path := strings.TrimSpace(loc.Path)
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			out = append(out, LibraryPath{
				SectionID:    dir.Key,
				SectionTitle: dir.Title,
				Type:         kind,
				Path:         path,
			})
		}
	}
	return out, nil
}

func decodePlexLocations(raw json.RawMessage) ([]plexLocation, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	if trimmed[0] == '"' {
		var path string
		if err := json.Unmarshal(trimmed, &path); err != nil {
			return nil, fmt.Errorf("plex decode location path: %w", err)
		}
		path = strings.TrimSpace(path)
		if path == "" {
			return nil, nil
		}
		return []plexLocation{{Path: path}}, nil
	}
	return decodeJSONList[plexLocation](raw)
}

func decodeJSONList[T any](raw json.RawMessage) ([]T, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var rows []T
		if err := json.Unmarshal(trimmed, &rows); err != nil {
			return nil, fmt.Errorf("plex decode list: %w", err)
		}
		return rows, nil
	}
	var row T
	if err := json.Unmarshal(trimmed, &row); err != nil {
		return nil, fmt.Errorf("plex decode item: %w", err)
	}
	return []T{row}, nil
}

// TestConnection hits /identity on the configured PMS.
func (c *Client) TestConnection(ctx context.Context) (serverName string, err error) {
	if c.serverURL == "" {
		return "", fmt.Errorf("plex serverUrl is required")
	}
	if c.token == "" {
		return "", fmt.Errorf("plex token is required")
	}
	rawURL := c.serverURL + "/identity"
	resp, body, err := c.http.DoJSON(ctx, http.MethodGet, rawURL, nil, c.plexHeaders())
	if err != nil {
		return "", err
	}
	if err := httpx.CheckStatus(resp, rawURL, body); err != nil {
		return "", err
	}
	var payload struct {
		MediaContainer struct {
			FriendlyName      string `json:"friendlyName"`
			MachineIdentifier string `json:"machineIdentifier"`
		} `json:"MediaContainer"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("plex decode identity: %w", err)
	}
	name := strings.TrimSpace(payload.MediaContainer.FriendlyName)
	if name == "" {
		name = strings.TrimSpace(payload.MediaContainer.MachineIdentifier)
	}
	return name, nil
}

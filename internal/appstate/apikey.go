package appstate

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateAPIKey returns a 32-character hex key (Radarr/Sonarr-style).
func GenerateAPIKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// GenerateClientID returns a stable-looking hex id for Plex PIN auth.
func GenerateClientID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate client id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

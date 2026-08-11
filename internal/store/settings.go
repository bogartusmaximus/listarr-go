package store

import (
	"encoding/json"
	"time"
)

// Settings is the operator configuration document (SoT after first seed).
type Settings struct {
	APIKey              string                `json:"apiKey"`
	InstanceName        string                `json:"instanceName"`
	SafeMode            bool                  `json:"safeMode"`
	TorboxSearchPerHour int                   `json:"torboxSearchPerHour"`
	TMDBAPIKey          string                `json:"tmdbApiKey"`
	ArrInstances        []ArrInstanceSettings `json:"arrInstances"`
	UpdatedAt           time.Time             `json:"updatedAt"`
}

type settingsJSON struct {
	APIKey              string                `json:"apiKey"`
	InstanceName        string                `json:"instanceName"`
	SafeMode            *bool                 `json:"safeMode"`
	ApplyEnabled        *bool                 `json:"applyEnabled"` // legacy
	TorboxSearchPerHour int                   `json:"torboxSearchPerHour"`
	TMDBAPIKey          string                `json:"tmdbApiKey"`
	ArrInstances        []ArrInstanceSettings `json:"arrInstances"`
	UpdatedAt           time.Time             `json:"updatedAt"`
}

// UnmarshalJSON migrates legacy applyEnabled into safeMode (safeMode = !applyEnabled).
// When neither field is present, Safe Mode defaults to on (read-only).
func (s *Settings) UnmarshalJSON(data []byte) error {
	var raw settingsJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.APIKey = raw.APIKey
	s.InstanceName = raw.InstanceName
	s.TorboxSearchPerHour = raw.TorboxSearchPerHour
	s.TMDBAPIKey = raw.TMDBAPIKey
	s.ArrInstances = raw.ArrInstances
	s.UpdatedAt = raw.UpdatedAt
	switch {
	case raw.SafeMode != nil:
		s.SafeMode = *raw.SafeMode
	case raw.ApplyEnabled != nil:
		s.SafeMode = !*raw.ApplyEnabled
	default:
		s.SafeMode = true
	}
	if s.ArrInstances == nil {
		s.ArrInstances = []ArrInstanceSettings{}
	}
	return nil
}

// ApplyEnabled is the inverse of Safe Mode (writes allowed when Safe Mode is off).
func (s Settings) ApplyEnabled() bool {
	return !s.SafeMode
}

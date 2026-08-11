package redact

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	apiKeyQuery = regexp.MustCompile(`(?i)([?&](?:apikey|api_key|access_token)=)[^&]*`)
	bearerRE    = regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)\S+`)
)

// URLString removes common secret query parameters from a URL string.
func URLString(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return apiKeyQuery.ReplaceAllString(raw, `${1}REDACTED`)
	}
	q := u.Query()
	for _, key := range []string{"apikey", "api_key", "access_token"} {
		if q.Has(key) {
			q.Set(key, "REDACTED")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// HeaderLine redacts Authorization bearer tokens in a single header line.
func HeaderLine(line string) string {
	return bearerRE.ReplaceAllString(line, `${1}REDACTED`)
}

// APIKey replaces a known key substring in free-form text.
func APIKey(text, key string) string {
	if key == "" || text == "" {
		return text
	}
	return strings.ReplaceAll(text, key, "REDACTED")
}

package redact_test

import (
	"strings"
	"testing"

	"github.com/bogartusmaximus/listarr-go/internal/redact"
)

func TestURLStringRedactsAPIKey(t *testing.T) {
	in := "http://127.0.0.1:7878/api/v3/movie?apikey=super-secret&page=1"
	out := redact.URLString(in)
	if strings.Contains(out, "super-secret") {
		t.Fatalf("not redacted: %q", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("missing marker: %q", out)
	}
}

func TestAPIKey(t *testing.T) {
	out := redact.APIKey("key=abc123 used", "abc123")
	if strings.Contains(out, "abc123") {
		t.Fatalf("got %q", out)
	}
}

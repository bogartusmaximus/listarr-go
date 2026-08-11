package httpx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bogartusmaximus/listarr-go/internal/redact"
)

// Client is a small HTTP helper with timeouts and secret-safe errors.
type Client struct {
	HTTP    *http.Client
	APIKey  string
	Header  string // e.g. X-Api-Key; empty means query apikey=
	Cookie  string // optional Cookie header (e.g. oauth2_proxy session)
	Timeout time.Duration
}

// New returns a client with a bounded timeout.
func New(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		HTTP: &http.Client{
			Timeout: timeout,
			// Do not follow redirects: oauth2-proxy login pages often 302→200 HTML.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		Timeout: timeout,
	}
}

// DoJSON performs a request and reads the body. Errors redact secrets.
func (c *Client) DoJSON(ctx context.Context, method, rawURL string, body io.Reader, header http.Header) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, nil, err
	}
	for k, vals := range header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	if c.APIKey != "" && c.Header != "" {
		req.Header.Set(c.Header, c.APIKey)
	}
	if c.Cookie != "" {
		req.Header.Set("Cookie", c.Cookie)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http %s %s: %w", method, redact.URLString(rawURL), err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return resp, nil, fmt.Errorf("read %s: %w", redact.URLString(rawURL), err)
	}
	return resp, data, nil
}

// CheckStatus returns an error for non-2xx responses without leaking secrets.
func CheckStatus(resp *http.Response, rawURL string, body []byte) error {
	return CheckStatusRedact(resp, rawURL, body, "")
}

// CheckStatusRedact is CheckStatus and also redacts secret substrings from the body snippet.
func CheckStatusRedact(resp *http.Response, rawURL string, body []byte, secret string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	snippet := string(body)
	if len(snippet) > 200 {
		snippet = snippet[:200]
	}
	snippet = redact.APIKey(snippet, secret)
	return fmt.Errorf("http %d %s: %s", resp.StatusCode, redact.URLString(rawURL), snippet)
}

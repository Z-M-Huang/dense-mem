// Package mcpclient provides HTTP adapters that implement the 7 core service
// interfaces by forwarding calls to the dense-mem HTTP API.
//
// A Client is bound to a single (baseURL, apiKey, profileID) triple — matching
// the "single-profile MCP instance" design decision. Every outgoing request
// carries Authorization: Bearer <key> and X-Profile-ID automatically.
package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client is an HTTP client bound to a single dense-mem base URL, API key, and
// profile ID. Adapter methods send Authorization and X-Profile-ID on every request.
//
// Security invariant: one Client instance maps to exactly one profile (the
// "single-profile MCP instance" plan key decision). The profileID stored at
// construction time is the identity of this process/instance; it must match
// the profileID passed to every service-method call.
type Client struct {
	baseURL    string
	apiKey     string
	profileID  string
	httpClient *http.Client
}

// NewClient constructs a Client bound to baseURL, apiKey and profileID.
// baseURL must not have a trailing slash (e.g. "http://localhost:8080").
func NewClient(baseURL, apiKey, profileID string) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		profileID:  profileID,
		httpClient: &http.Client{},
	}
}

// httpResult is a fully buffered HTTP response, safe to inspect after the
// underlying connection has been closed.
type httpResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// do executes req, reads the full response body, and returns an httpResult.
// Callers check StatusCode and decode Body independently.
func (c *Client) do(req *http.Request) (*httpResult, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: read body: %w", err)
	}

	return &httpResult{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       body,
	}, nil
}

// newRequest builds a context-bound, authenticated HTTP request.
// The profileID parameter is taken from the service-method caller so that each
// HTTP call is scoped to the correct profile (profile-isolation invariant: never
// substitute or cache a foreign profileID).
func (c *Client) newRequest(ctx context.Context, method, path, profileID string, body any) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("mcpclient: marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("mcpclient: new request: %w", err)
	}

	// Security invariant: Authorization authenticates the client and
	// X-Profile-ID scopes the operation.
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("X-Profile-ID", profileID)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// APIError is a non-2xx HTTP response from the dense-mem API.
// Callers can inspect StatusCode for specific error-handling logic.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("mcpclient: API %d: %s", e.StatusCode, e.Body)
}

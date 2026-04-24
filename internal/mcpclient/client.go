// Package mcpclient provides HTTP adapters that implement the 7 core service
// interfaces by forwarding calls to the dense-mem HTTP API.
//
// A Client is bound to a single (baseURL, apiKey) pair. Every outgoing request
// carries Authorization: Bearer <key>; the HTTP API derives profile scope from
// the profile-bound key.
package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client is an HTTP client bound to a single dense-mem base URL and API key.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient constructs a Client bound to baseURL and apiKey.
// baseURL must not have a trailing slash (e.g. "http://localhost:8080").
func NewClient(baseURL, apiKey string, legacyProfileID ...string) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
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
// The profileID parameter is retained for service-interface compatibility; HTTP
// scope is derived from the authenticated API key.
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

	// Security invariant: Authorization authenticates the client and the server
	// derives profile scope from the profile-bound API key.
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
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

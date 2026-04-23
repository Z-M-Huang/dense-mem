package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	httpDto "github.com/dense-mem/dense-mem/internal/http/dto"
)

// ListTools fetches the authenticated caller's visible tool catalog from the
// dense-mem HTTP API. The returned entries are the source of truth for MCP
// discovery metadata such as JSON schemas.
func (c *Client) ListTools(ctx context.Context, profileID string) (*httpDto.ToolCatalogResponse, error) {
	httpReq, err := c.newRequest(ctx, http.MethodGet, "/api/v1/tools", profileID, nil)
	if err != nil {
		return nil, err
	}

	res, err := c.do(httpReq)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: res.StatusCode, Body: string(res.Body)}
	}

	var resp httpDto.ToolCatalogResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, fmt.Errorf("mcpclient: decode tool catalog response: %w", err)
	}
	return &resp, nil
}

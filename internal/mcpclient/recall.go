package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/dense-mem/dense-mem/internal/service/recallservice"
)

// recallAdapter implements recallservice.RecallService via the dense-mem HTTP API.
type recallAdapter struct {
	c *Client
}

var _ recallservice.RecallService = (*recallAdapter)(nil)

// NewRecall returns a RecallService backed by the dense-mem HTTP API.
// Calls GET /api/v1/recall?query=<text>&limit=<n>.
func NewRecall(c *Client) recallservice.RecallService {
	return &recallAdapter{c: c}
}

// recallHTTPResponse is the wire format for GET /api/v1/recall.
// Each element in Data is a fully hydrated RecallHit containing a domain.Fragment.
type recallHTTPResponse struct {
	Data []recallservice.RecallHit `json:"data"`
}

func (a *recallAdapter) Recall(ctx context.Context, profileID string, req recallservice.RecallRequest) ([]recallservice.RecallHit, error) {
	q := url.Values{}
	q.Set("query", req.Query)
	if req.Limit > 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}

	httpReq, err := a.c.newRequest(ctx, http.MethodGet, "/api/v1/recall?"+q.Encode(), profileID, nil)
	if err != nil {
		return nil, err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: res.StatusCode, Body: string(res.Body)}
	}

	var resp recallHTTPResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, fmt.Errorf("mcpclient: decode recall response: %w", err)
	}

	return resp.Data, nil
}

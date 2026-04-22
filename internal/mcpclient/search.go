package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dense-mem/dense-mem/internal/tools/graphquery"
	"github.com/dense-mem/dense-mem/internal/tools/keywordsearch"
	"github.com/dense-mem/dense-mem/internal/tools/semanticsearch"
)

// --- KeywordSearch adapter ------------------------------------------------

type keywordSearchAdapter struct {
	c *Client
}

var _ keywordsearch.KeywordSearchService = (*keywordSearchAdapter)(nil)

// NewKeywordSearch returns a KeywordSearchService backed by the dense-mem HTTP API.
// Calls POST /api/v1/tools/keyword-search.
func NewKeywordSearch(c *Client) keywordsearch.KeywordSearchService {
	return &keywordSearchAdapter{c: c}
}

// keywordSearchHTTPRequest is the wire format for POST /api/v1/tools/keyword-search.
// The HTTP handler maps req.Keywords → svc.Query, so we invert that mapping here.
type keywordSearchHTTPRequest struct {
	Keywords        string `json:"keywords"`
	Limit           int    `json:"limit"`
	ValidAt         string `json:"valid_at,omitempty"`
	KnownAt         string `json:"known_at,omitempty"`
	IncludeEvidence bool   `json:"include_evidence,omitempty"`
}

func (a *keywordSearchAdapter) Search(ctx context.Context, profileID string, req *keywordsearch.KeywordSearchRequest) (*keywordsearch.KeywordSearchResult, error) {
	httpBody := keywordSearchHTTPRequest{
		Keywords:        req.Query,
		Limit:           req.Limit,
		IncludeEvidence: req.IncludeEvidence,
	}
	if req.ValidAt != nil {
		httpBody.ValidAt = req.ValidAt.UTC().Format(time.RFC3339)
	}
	if req.KnownAt != nil {
		httpBody.KnownAt = req.KnownAt.UTC().Format(time.RFC3339)
	}

	httpReq, err := a.c.newRequest(ctx, http.MethodPost, "/api/v1/tools/keyword-search", profileID, httpBody)
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

	var result keywordsearch.KeywordSearchResult
	if err := json.Unmarshal(res.Body, &result); err != nil {
		return nil, fmt.Errorf("mcpclient: decode keyword search result: %w", err)
	}

	return &result, nil
}

// --- SemanticSearch adapter -----------------------------------------------

type semanticSearchAdapter struct {
	c *Client
}

var _ semanticsearch.SemanticSearchService = (*semanticSearchAdapter)(nil)

// NewSemanticSearch returns a SemanticSearchService backed by the dense-mem HTTP API.
// Calls POST /api/v1/tools/semantic-search.
// The handler binds semanticsearch.SemanticSearchRequest directly, so we post
// the service request struct without field remapping.
func NewSemanticSearch(c *Client) semanticsearch.SemanticSearchService {
	return &semanticSearchAdapter{c: c}
}

func (a *semanticSearchAdapter) Search(ctx context.Context, profileID string, req *semanticsearch.SemanticSearchRequest) (*semanticsearch.SemanticSearchResult, error) {
	httpReq, err := a.c.newRequest(ctx, http.MethodPost, "/api/v1/tools/semantic-search", profileID, req)
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

	var result semanticsearch.SemanticSearchResult
	if err := json.Unmarshal(res.Body, &result); err != nil {
		return nil, fmt.Errorf("mcpclient: decode semantic search result: %w", err)
	}

	return &result, nil
}

// --- GraphQuery adapter ---------------------------------------------------

type graphQueryAdapter struct {
	c *Client
}

var _ graphquery.GraphQueryService = (*graphQueryAdapter)(nil)

// NewGraphQuery returns a GraphQueryService backed by the dense-mem HTTP API.
// Calls POST /api/v1/tools/graph-query.
func NewGraphQuery(c *Client) graphquery.GraphQueryService {
	return &graphQueryAdapter{c: c}
}

// graphQueryHTTPRequest is the wire format for POST /api/v1/tools/graph-query.
type graphQueryHTTPRequest struct {
	Query      string         `json:"query"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// graphQueryHTTPResponse is the response from POST /api/v1/tools/graph-query.
// The handler wraps the result in Data+Meta, which we flatten back into
// graphquery.GraphQueryResult.
type graphQueryHTTPResponse struct {
	Data struct {
		Columns []string         `json:"columns"`
		Rows    []map[string]any `json:"rows"`
	} `json:"data"`
	Meta struct {
		RowCount      int  `json:"row_count"`
		RowCapApplied bool `json:"row_cap_applied"`
	} `json:"meta"`
}

func (a *graphQueryAdapter) Execute(ctx context.Context, profileID, query string, params map[string]any) (*graphquery.GraphQueryResult, error) {
	httpBody := graphQueryHTTPRequest{
		Query:      query,
		Parameters: params,
	}

	httpReq, err := a.c.newRequest(ctx, http.MethodPost, "/api/v1/tools/graph-query", profileID, httpBody)
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

	var resp graphQueryHTTPResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, fmt.Errorf("mcpclient: decode graph query response: %w", err)
	}

	return &graphquery.GraphQueryResult{
		Columns:       resp.Data.Columns,
		Rows:          resp.Data.Rows,
		RowCount:      resp.Meta.RowCount,
		RowCapApplied: resp.Meta.RowCapApplied,
	}, nil
}

package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/dense-mem/dense-mem/internal/domain"
	httpDto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
)

// --- CreateFragment adapter -----------------------------------------------

type fragmentCreateAdapter struct {
	c *Client
}

var _ fragmentservice.CreateFragmentService = (*fragmentCreateAdapter)(nil)

// NewFragmentCreate returns a CreateFragmentService backed by the dense-mem HTTP API.
// Calls POST /api/v1/fragments. Returns Duplicate=true when the server responds
// with a 200 OK and the X-Idempotent-Replay header.
func NewFragmentCreate(c *Client) fragmentservice.CreateFragmentService {
	return &fragmentCreateAdapter{c: c}
}

func (a *fragmentCreateAdapter) Create(ctx context.Context, profileID string, req *httpDto.CreateFragmentRequest) (*fragmentservice.CreateResult, error) {
	httpReq, err := a.c.newRequest(ctx, http.MethodPost, "/api/v1/fragments", profileID, req)
	if err != nil {
		return nil, err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: res.StatusCode, Body: string(res.Body)}
	}

	var resp httpDto.FragmentResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, fmt.Errorf("mcpclient: decode fragment response: %w", err)
	}

	duplicate := res.Header.Get("X-Idempotent-Replay") == "true"
	fragment := fragmentFromResponse(&resp, profileID)
	duplicateOf := ""
	if duplicate {
		duplicateOf = fragment.FragmentID
	}
	return &fragmentservice.CreateResult{
		Fragment:    fragment,
		Duplicate:   duplicate,
		DuplicateOf: duplicateOf,
	}, nil
}

// --- GetFragment adapter --------------------------------------------------

type fragmentGetAdapter struct {
	c *Client
}

var _ fragmentservice.GetFragmentService = (*fragmentGetAdapter)(nil)

// NewFragmentGet returns a GetFragmentService backed by the dense-mem HTTP API.
// Calls GET /api/v1/fragments/:id. Returns fragmentservice.ErrFragmentNotFound on 404.
func NewFragmentGet(c *Client) fragmentservice.GetFragmentService {
	return &fragmentGetAdapter{c: c}
}

func (a *fragmentGetAdapter) GetByID(ctx context.Context, profileID, fragmentID string) (*domain.Fragment, error) {
	path := "/api/v1/fragments/" + url.PathEscape(fragmentID)
	httpReq, err := a.c.newRequest(ctx, http.MethodGet, path, profileID, nil)
	if err != nil {
		return nil, err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return nil, err
	}

	if res.StatusCode == http.StatusNotFound {
		// 404 is returned for both missing fragments and cross-profile reads
		// (by design — existence is not leaked across profiles).
		return nil, fragmentservice.ErrFragmentNotFound
	}
	if res.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: res.StatusCode, Body: string(res.Body)}
	}

	var resp httpDto.FragmentResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, fmt.Errorf("mcpclient: decode fragment response: %w", err)
	}

	return fragmentFromResponse(&resp, profileID), nil
}

// --- ListFragments adapter ------------------------------------------------

type fragmentListAdapter struct {
	c *Client
}

var _ fragmentservice.ListFragmentsService = (*fragmentListAdapter)(nil)

// NewFragmentList returns a ListFragmentsService backed by the dense-mem HTTP API.
// Calls GET /api/v1/fragments with optional limit, cursor, and source_type query params.
func NewFragmentList(c *Client) fragmentservice.ListFragmentsService {
	return &fragmentListAdapter{c: c}
}

func (a *fragmentListAdapter) List(ctx context.Context, profileID string, opts fragmentservice.ListOptions) ([]domain.Fragment, string, error) {
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}
	if opts.SourceType != "" {
		q.Set("source_type", opts.SourceType)
	}

	path := "/api/v1/fragments"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	httpReq, err := a.c.newRequest(ctx, http.MethodGet, path, profileID, nil)
	if err != nil {
		return nil, "", err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return nil, "", err
	}

	if res.StatusCode != http.StatusOK {
		return nil, "", &APIError{StatusCode: res.StatusCode, Body: string(res.Body)}
	}

	var resp httpDto.ListFragmentsResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, "", fmt.Errorf("mcpclient: decode list fragments response: %w", err)
	}

	frags := make([]domain.Fragment, 0, len(resp.Items))
	for i := range resp.Items {
		frags = append(frags, *fragmentFromResponse(&resp.Items[i], profileID))
	}

	return frags, resp.NextCursor, nil
}

// --- helpers --------------------------------------------------------------

// fragmentFromResponse converts a FragmentResponse DTO to a domain.Fragment.
// profileID is injected from the caller because FragmentResponse does not carry
// it — the server enforces the scope at the database layer.
func fragmentFromResponse(r *httpDto.FragmentResponse, profileID string) *domain.Fragment {
	return &domain.Fragment{
		// r.ID and r.FragmentID hold the same value; r.ID maps to json:"id"
		// which aligns with domain.Fragment's json:"id" tag.
		FragmentID:          r.ID,
		ProfileID:           profileID,
		Content:             r.Content,
		SourceType:          domain.SourceType(r.SourceType),
		Source:              r.Source,
		Authority:           domain.Authority(r.Authority),
		Labels:              r.Labels,
		Metadata:            r.Metadata,
		ContentHash:         r.ContentHash,
		IdempotencyKey:      r.IdempotencyKey,
		EmbeddingModel:      r.EmbeddingModel,
		EmbeddingDimensions: r.EmbeddingDimensions,
		SourceQuality:       r.SourceQuality,
		Classification:      r.Classification,
		Status:              domain.FragmentStatus(r.Status),
		RecordedTo:          r.RecordedTo,
		CreatedAt:           r.CreatedAt,
		UpdatedAt:           r.UpdatedAt,
	}
}

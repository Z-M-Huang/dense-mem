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
	"github.com/dense-mem/dense-mem/internal/service/factservice"
)

// --- PromoteClaim adapter -------------------------------------------------

type claimPromoteAdapter struct {
	c *Client
}

var _ factservice.PromoteClaimService = (*claimPromoteAdapter)(nil)

// NewClaimPromote returns a PromoteClaimService backed by the dense-mem HTTP API.
// Calls POST /api/v1/claims/:id/promote (no request body). Returns the newly
// created Fact on success (201 Created).
//
// Security invariant: profile_id is always taken from the profileID parameter;
// the server enforces scope via X-Profile-ID header.
func NewClaimPromote(c *Client) factservice.PromoteClaimService {
	return &claimPromoteAdapter{c: c}
}

func (a *claimPromoteAdapter) Promote(ctx context.Context, profileID, claimID string) (*domain.Fact, error) {
	path := "/api/v1/claims/" + url.PathEscape(claimID) + "/promote"
	httpReq, err := a.c.newRequest(ctx, http.MethodPost, path, profileID, nil)
	if err != nil {
		return nil, err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return nil, err
	}

	// Promote returns 201 Created on success; accept 200 OK as well for
	// defensive compatibility.
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: res.StatusCode, Body: string(res.Body)}
	}

	var resp httpDto.FactResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, fmt.Errorf("mcpclient: decode fact response: %w", err)
	}

	return factFromResponse(&resp), nil
}

// --- GetFact adapter ------------------------------------------------------

type factGetAdapter struct {
	c *Client
}

var _ factservice.GetFactService = (*factGetAdapter)(nil)

// NewFactGet returns a GetFactService backed by the dense-mem HTTP API.
// Calls GET /api/v1/facts/:id. Returns factservice.ErrFactNotFound on 404.
// 404 is returned for both missing facts and cross-profile reads — existence is
// not leaked across profiles by design.
func NewFactGet(c *Client) factservice.GetFactService {
	return &factGetAdapter{c: c}
}

func (a *factGetAdapter) Get(ctx context.Context, profileID, factID string) (*domain.Fact, error) {
	path := "/api/v1/facts/" + url.PathEscape(factID)
	httpReq, err := a.c.newRequest(ctx, http.MethodGet, path, profileID, nil)
	if err != nil {
		return nil, err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return nil, err
	}

	if res.StatusCode == http.StatusNotFound {
		// 404 is returned for both missing and cross-profile facts (existence
		// is not leaked across profiles).
		return nil, factservice.ErrFactNotFound
	}
	if res.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: res.StatusCode, Body: string(res.Body)}
	}

	var resp httpDto.FactResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, fmt.Errorf("mcpclient: decode fact response: %w", err)
	}

	return factFromResponse(&resp), nil
}

// --- ListFacts adapter ----------------------------------------------------

type factListAdapter struct {
	c *Client
}

var _ factservice.ListFactsService = (*factListAdapter)(nil)

// NewFactList returns a ListFactsService backed by the dense-mem HTTP API.
// Calls GET /api/v1/facts with optional subject, predicate, status, limit, and
// cursor query params. Uses keyset cursor pagination matching the server's
// base64-encoded (recorded_at, fact_id) token scheme.
func NewFactList(c *Client) factservice.ListFactsService {
	return &factListAdapter{c: c}
}

func (a *factListAdapter) List(ctx context.Context, profileID string, filters factservice.FactListFilters, limit int, cursor string) ([]*domain.Fact, string, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if filters.Subject != "" {
		q.Set("subject", filters.Subject)
	}
	if filters.Predicate != "" {
		q.Set("predicate", filters.Predicate)
	}
	if filters.Status != "" {
		q.Set("status", string(filters.Status))
	}

	path := "/api/v1/facts"
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

	var resp httpDto.ListFactsResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, "", fmt.Errorf("mcpclient: decode list facts response: %w", err)
	}

	facts := make([]*domain.Fact, 0, len(resp.Items))
	for i := range resp.Items {
		facts = append(facts, factFromResponse(&resp.Items[i]))
	}

	return facts, resp.NextCursor, nil
}

// --- helpers --------------------------------------------------------------

// factFromResponse converts a FactResponse DTO to a domain.Fact.
// ProfileID is preserved from the server response since FactResponse carries it.
func factFromResponse(r *httpDto.FactResponse) *domain.Fact {
	return &domain.Fact{
		FactID:                       r.FactID,
		ProfileID:                    r.ProfileID,
		Subject:                      r.Subject,
		Predicate:                    r.Predicate,
		Object:                       r.Object,
		Status:                       domain.FactStatus(r.Status),
		TruthScore:                   r.TruthScore,
		ValidFrom:                    r.ValidFrom,
		ValidTo:                      r.ValidTo,
		RecordedAt:                   r.RecordedAt,
		RetractedAt:                  r.RetractedAt,
		LastConfirmedAt:              r.LastConfirmedAt,
		PromotedFromClaimID:          r.PromotedFromClaimID,
		Classification:               r.Classification,
		ClassificationLatticeVersion: r.ClassificationLatticeVersion,
		SourceQuality:                r.SourceQuality,
		Labels:                       r.Labels,
		Metadata:                     r.Metadata,
	}
}

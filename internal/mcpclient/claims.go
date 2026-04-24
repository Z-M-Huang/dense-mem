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
	"github.com/dense-mem/dense-mem/internal/service/claimservice"
)

// --- CreateClaim adapter --------------------------------------------------

type claimCreateAdapter struct {
	c *Client
}

var _ claimservice.CreateClaimService = (*claimCreateAdapter)(nil)

// NewClaimCreate returns a CreateClaimService backed by the dense-mem HTTP API.
// Calls POST /api/v1/claims. Returns Duplicate=true when the server responds
// with the X-Idempotent-Replay header.
//
// Security invariant: profile_id is not accepted from tool input; the server
// derives scope from the profile-bound API key.
func NewClaimCreate(c *Client) claimservice.CreateClaimService {
	return &claimCreateAdapter{c: c}
}

func (a *claimCreateAdapter) Create(ctx context.Context, profileID string, claim *domain.Claim) (*claimservice.CreateResult, error) {
	// Map domain.Claim → CreateClaimRequest DTO.
	// profile_id is not included in the request body — the server derives it from
	// the profile-bound API key.
	body := httpDto.CreateClaimRequest{
		SupportedBy:    claim.SupportedBy,
		Subject:        claim.Subject,
		Predicate:      claim.Predicate,
		Object:         claim.Object,
		Modality:       string(claim.Modality),
		Polarity:       string(claim.Polarity),
		Speaker:        claim.Speaker,
		ExtractConf:    claim.ExtractConf,
		ResolutionConf: claim.ResolutionConf,
		IdempotencyKey: claim.IdempotencyKey,
		ValidFrom:      claim.ValidFrom,
		ValidTo:        claim.ValidTo,
	}

	httpReq, err := a.c.newRequest(ctx, http.MethodPost, "/api/v1/claims", profileID, body)
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

	var resp httpDto.ClaimResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, fmt.Errorf("mcpclient: decode claim response: %w", err)
	}

	duplicate := res.Header.Get("X-Idempotent-Replay") == "true"
	return &claimservice.CreateResult{
		Claim:     claimFromResponse(&resp),
		Duplicate: duplicate,
	}, nil
}

// --- GetClaim adapter -----------------------------------------------------

type claimGetAdapter struct {
	c *Client
}

var _ claimservice.GetClaimService = (*claimGetAdapter)(nil)

// NewClaimGet returns a GetClaimService backed by the dense-mem HTTP API.
// Calls GET /api/v1/claims/:id. Returns claimservice.ErrClaimNotFound on 404.
// 404 is returned for both missing claims and cross-profile reads — existence is
// not leaked across profiles by design.
func NewClaimGet(c *Client) claimservice.GetClaimService {
	return &claimGetAdapter{c: c}
}

func (a *claimGetAdapter) Get(ctx context.Context, profileID, claimID string) (*domain.Claim, error) {
	path := "/api/v1/claims/" + url.PathEscape(claimID)
	httpReq, err := a.c.newRequest(ctx, http.MethodGet, path, profileID, nil)
	if err != nil {
		return nil, err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return nil, err
	}

	if res.StatusCode == http.StatusNotFound {
		// 404 is returned for both missing and cross-profile claims (existence
		// is not leaked across profiles).
		return nil, claimservice.ErrClaimNotFound
	}
	if res.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: res.StatusCode, Body: string(res.Body)}
	}

	var resp httpDto.ClaimResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, fmt.Errorf("mcpclient: decode claim response: %w", err)
	}

	return claimFromResponse(&resp), nil
}

// --- ListClaims adapter ---------------------------------------------------

type claimListAdapter struct {
	c *Client
}

var _ claimservice.ListClaimsService = (*claimListAdapter)(nil)

// NewClaimList returns a ListClaimsService backed by the dense-mem HTTP API.
// Calls GET /api/v1/claims with limit and cursor query params.
// The cursor is an opaque base-10 integer encoding the current offset — this
// matches the handler's offset-based pagination scheme.
func NewClaimList(c *Client) claimservice.ListClaimsService {
	return &claimListAdapter{c: c}
}

func (a *claimListAdapter) List(ctx context.Context, profileID string, limit, offset int) ([]*domain.Claim, int, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		// The handler decodes cursor as an offset integer.
		q.Set("cursor", strconv.Itoa(offset))
	}

	path := "/api/v1/claims"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	httpReq, err := a.c.newRequest(ctx, http.MethodGet, path, profileID, nil)
	if err != nil {
		return nil, 0, err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return nil, 0, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, 0, &APIError{StatusCode: res.StatusCode, Body: string(res.Body)}
	}

	var resp httpDto.ListClaimsResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, 0, fmt.Errorf("mcpclient: decode list claims response: %w", err)
	}

	claims := make([]*domain.Claim, 0, len(resp.Items))
	for i := range resp.Items {
		claims = append(claims, claimFromResponse(&resp.Items[i]))
	}

	// Approximate total from pagination signal:
	// HasMore=true  → at least offset+len+1 records exist (lower bound).
	// HasMore=false → this is the last page; total = offset + len(items).
	// The total is used by callers solely to determine whether a next page
	// exists (total > offset+len), so ±1 precision is sufficient.
	total := offset + len(claims)
	if resp.HasMore {
		total++
	}

	return claims, total, nil
}

// --- VerifyClaim adapter --------------------------------------------------

type claimVerifyAdapter struct {
	c *Client
}

var _ claimservice.VerifyClaimService = (*claimVerifyAdapter)(nil)

// NewClaimVerify returns a VerifyClaimService backed by the dense-mem HTTP API.
// Calls POST /api/v1/claims/:id/verify (no request body). Returns a
// *domain.Claim populated with the verification outcome fields (ClaimID,
// EntailmentVerdict, Status, LastVerifierResponse, VerifierModel, VerifiedAt).
// The remaining Claim fields are not returned by the verify endpoint.
func NewClaimVerify(c *Client) claimservice.VerifyClaimService {
	return &claimVerifyAdapter{c: c}
}

func (a *claimVerifyAdapter) Verify(ctx context.Context, profileID, claimID string) (*domain.Claim, error) {
	path := "/api/v1/claims/" + url.PathEscape(claimID) + "/verify"
	httpReq, err := a.c.newRequest(ctx, http.MethodPost, path, profileID, nil)
	if err != nil {
		return nil, err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return nil, err
	}

	if res.StatusCode == http.StatusNotFound {
		return nil, claimservice.ErrClaimNotFound
	}
	if res.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: res.StatusCode, Body: string(res.Body)}
	}

	var resp httpDto.VerifyClaimResponse
	if err := json.Unmarshal(res.Body, &resp); err != nil {
		return nil, fmt.Errorf("mcpclient: decode verify claim response: %w", err)
	}

	// The verify endpoint returns only the verification outcome fields.
	// Populate a domain.Claim with what the server provides; other fields
	// remain zero-valued.
	return &domain.Claim{
		ClaimID:              resp.ClaimID,
		ProfileID:            profileID,
		EntailmentVerdict:    domain.EntailmentVerdict(resp.EntailmentVerdict),
		Status:               domain.ClaimStatus(resp.Status),
		LastVerifierResponse: resp.LastVerifierResponse,
		VerifierModel:        resp.VerifierModel,
		VerifiedAt:           resp.VerifiedAt,
	}, nil
}

// --- helpers --------------------------------------------------------------

// claimFromResponse converts a ClaimResponse DTO to a domain.Claim.
// ProfileID is preserved from the server response since ClaimResponse carries it.
func claimFromResponse(r *httpDto.ClaimResponse) *domain.Claim {
	return &domain.Claim{
		ClaimID:           r.ClaimID,
		ProfileID:         r.ProfileID,
		Subject:           r.Subject,
		Predicate:         r.Predicate,
		Object:            r.Object,
		Modality:          domain.ClaimModality(r.Modality),
		Polarity:          domain.ClaimPolarity(r.Polarity),
		Speaker:           r.Speaker,
		SpanStart:         r.SpanStart,
		SpanEnd:           r.SpanEnd,
		ValidFrom:         r.ValidFrom,
		ValidTo:           r.ValidTo,
		RecordedAt:        r.RecordedAt,
		RecordedTo:        r.RecordedTo,
		ExtractConf:       r.ExtractConf,
		ResolutionConf:    r.ResolutionConf,
		SourceQuality:     r.SourceQuality,
		EntailmentVerdict: domain.EntailmentVerdict(r.EntailmentVerdict),
		Status:            domain.ClaimStatus(r.Status),
		ExtractionModel:   r.ExtractionModel,
		ExtractionVersion: r.ExtractionVersion,
		VerifierModel:     r.VerifierModel,
		PipelineRunID:     r.PipelineRunID,
		ContentHash:       r.ContentHash,
		IdempotencyKey:    r.IdempotencyKey,
		Classification:    r.Classification,
		SupportedBy:       r.SupportedBy,
		Evidence:          evidenceFromResponse(r.Evidence),
	}
}

func evidenceFromResponse(items []httpDto.Evidence) []domain.Evidence {
	if len(items) == 0 {
		return nil
	}
	out := make([]domain.Evidence, 0, len(items))
	for _, item := range items {
		out = append(out, domain.Evidence{
			FragmentID:        item.FragmentID,
			Speaker:           item.Speaker,
			SpanStart:         item.SpanStart,
			SpanEnd:           item.SpanEnd,
			ExtractConf:       item.ExtractConf,
			ExtractionModel:   item.ExtractionModel,
			ExtractionVersion: item.ExtractionVersion,
			PipelineRunID:     item.PipelineRunID,
			Authority:         domain.Authority(item.Authority),
		})
	}
	return out
}

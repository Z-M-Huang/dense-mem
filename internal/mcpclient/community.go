package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/service/communityservice"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
)

// --- RetractFragment adapter ----------------------------------------------

type fragmentRetractAdapter struct {
	c *Client
}

var _ fragmentservice.RetractFragmentService = (*fragmentRetractAdapter)(nil)

// NewFragmentRetract returns a RetractFragmentService backed by the dense-mem HTTP API.
// Calls POST /api/v1/fragments/:id/retract (no request body).
// Returns fragmentservice.ErrFragmentNotFound on 404.
// 404 is returned for both missing fragments and cross-profile retracts —
// existence is not leaked across profiles by design.
func NewFragmentRetract(c *Client) fragmentservice.RetractFragmentService {
	return &fragmentRetractAdapter{c: c}
}

func (a *fragmentRetractAdapter) Retract(ctx context.Context, profileID, fragmentID string) error {
	path := "/api/v1/fragments/" + url.PathEscape(fragmentID) + "/retract"
	httpReq, err := a.c.newRequest(ctx, http.MethodPost, path, profileID, nil)
	if err != nil {
		return err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return err
	}

	if res.StatusCode == http.StatusNotFound {
		// 404 is returned for both missing and cross-profile fragments
		// (existence is not leaked across profiles).
		return fragmentservice.ErrFragmentNotFound
	}
	if res.StatusCode != http.StatusOK {
		return &APIError{StatusCode: res.StatusCode, Body: string(res.Body)}
	}

	return nil
}

// --- DetectCommunity adapter ----------------------------------------------

type communityDetectAdapter struct {
	c *Client
}

var _ communityservice.DetectCommunityService = (*communityDetectAdapter)(nil)

// NewCommunityDetect returns a DetectCommunityService backed by the dense-mem HTTP API.
// Calls POST /api/v1/admin/profiles/:profileId/community/detect (no request body).
// Returns communityservice.ErrCommunityUnavailable on 503 and
// communityservice.ErrCommunityGraphTooLarge on 422.
//
// Security invariant: profileID is embedded in the URL path for this admin
// endpoint — the server reads it from the path parameter, not X-Profile-ID.
// X-Profile-ID is also sent (via newRequest) for defence-in-depth.
func NewCommunityDetect(c *Client) communityservice.DetectCommunityService {
	return &communityDetectAdapter{c: c}
}

func (a *communityDetectAdapter) Detect(ctx context.Context, profileID string) error {
	path := "/api/v1/admin/profiles/" + url.PathEscape(profileID) + "/community/detect"
	httpReq, err := a.c.newRequest(ctx, http.MethodPost, path, profileID, nil)
	if err != nil {
		return err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return err
	}

	switch res.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusServiceUnavailable:
		return communityservice.ErrCommunityUnavailable
	case http.StatusUnprocessableEntity:
		return communityservice.ErrCommunityGraphTooLarge
	default:
		return fmt.Errorf("mcpclient: community detect: %w", &APIError{StatusCode: res.StatusCode, Body: string(res.Body)})
	}
}

// --- Community summary adapters ------------------------------------------

type communityGetAdapter struct {
	c *Client
}

type communityListAdapter struct {
	c *Client
}

var _ communityservice.GetCommunitySummaryService = (*communityGetAdapter)(nil)
var _ communityservice.ListCommunitiesService = (*communityListAdapter)(nil)

// NewCommunityGet returns a GetCommunitySummaryService backed by the dense-mem HTTP API.
func NewCommunityGet(c *Client) communityservice.GetCommunitySummaryService {
	return &communityGetAdapter{c: c}
}

// NewCommunityList returns a ListCommunitiesService backed by the dense-mem HTTP API.
func NewCommunityList(c *Client) communityservice.ListCommunitiesService {
	return &communityListAdapter{c: c}
}

func (a *communityGetAdapter) Get(ctx context.Context, profileID, communityID string) (*domain.Community, error) {
	path := "/api/v1/communities/" + url.PathEscape(communityID)
	httpReq, err := a.c.newRequest(ctx, http.MethodGet, path, profileID, nil)
	if err != nil {
		return nil, err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return nil, err
	}
	switch res.StatusCode {
	case http.StatusOK:
		var body dto.CommunityResponse
		if err := json.Unmarshal(res.Body, &body); err != nil {
			return nil, fmt.Errorf("mcpclient: decode community: %w", err)
		}
		return communityFromDTO(body), nil
	case http.StatusNotFound:
		return nil, communityservice.ErrCommunityNotFound
	default:
		return nil, fmt.Errorf("mcpclient: community get: %w", &APIError{StatusCode: res.StatusCode, Body: string(res.Body)})
	}
}

func (a *communityListAdapter) List(ctx context.Context, profileID string, limit int) ([]*domain.Community, error) {
	path := "/api/v1/communities"
	if limit > 0 {
		path += "?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit))
	}
	httpReq, err := a.c.newRequest(ctx, http.MethodGet, path, profileID, nil)
	if err != nil {
		return nil, err
	}

	res, err := a.c.do(httpReq)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mcpclient: community list: %w", &APIError{StatusCode: res.StatusCode, Body: string(res.Body)})
	}

	var body dto.ListCommunitiesResponse
	if err := json.Unmarshal(res.Body, &body); err != nil {
		return nil, fmt.Errorf("mcpclient: decode community list: %w", err)
	}

	communities := make([]*domain.Community, 0, len(body.Items))
	for _, item := range body.Items {
		communities = append(communities, communityFromDTO(item))
	}
	return communities, nil
}

func communityFromDTO(item dto.CommunityResponse) *domain.Community {
	return &domain.Community{
		CommunityID:      item.CommunityID,
		ProfileID:        item.ProfileID,
		Level:            item.Level,
		Summary:          item.Summary,
		SummaryVersion:   item.SummaryVersion,
		MemberCount:      item.MemberCount,
		TopEntities:      item.TopEntities,
		TopPredicates:    item.TopPredicates,
		LastSummarizedAt: item.LastSummarizedAt,
	}
}

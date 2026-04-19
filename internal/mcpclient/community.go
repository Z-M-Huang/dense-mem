package mcpclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

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

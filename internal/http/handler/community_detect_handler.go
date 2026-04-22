package handler

import (
	"errors"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/http/response"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/communityservice"
)

// CommunityDetectHandler serves POST /api/v1/admin/profiles/:profileId/community/detect.
//
// This endpoint is admin-only. The target profileId is read from the URL path
// parameter, not from the standard profile-resolution middleware (which is not
// applied to admin routes). The handler calls DetectCommunityService.Detect to
// trigger community detection on the profile's knowledge graph and writes
// community membership back to the Neo4j nodes.
//
// Profile isolation invariant: the profileId from the URL is forwarded verbatim
// to the service, which scopes the GDS graph projection to that profile only.
type CommunityDetectHandler struct {
	detectSvc communityservice.DetectCommunityService
	listSvc   communityservice.ListCommunitiesService
}

// CommunityDetectHandlerInterface is the companion interface for CommunityDetectHandler.
type CommunityDetectHandlerInterface interface {
	Handle(c echo.Context) error
}

// Ensure CommunityDetectHandler implements CommunityDetectHandlerInterface.
var _ CommunityDetectHandlerInterface = (*CommunityDetectHandler)(nil)

// NewCommunityDetectHandler constructs a CommunityDetectHandler.
func NewCommunityDetectHandler(
	detectSvc communityservice.DetectCommunityService,
	listSvc communityservice.ListCommunitiesService,
) *CommunityDetectHandler {
	return &CommunityDetectHandler{
		detectSvc: detectSvc,
		listSvc:   listSvc,
	}
}

// Handle triggers community detection for the profile identified by :profileId.
// Returns 200 {"detected": true} on success.
// Returns 422 when the graph is too large for detection.
// Returns 503 when the GDS plugin is not available.
func (h *CommunityDetectHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	// Verify admin principal — AdminOnly middleware already enforces this;
	// the check here is defense-in-depth so the handler is safe if wired
	// without the middleware in future.
	principal := middleware.GetPrincipal(ctx)
	if principal == nil {
		return httperr.New(httperr.AUTH_MISSING, "authentication required")
	}
	if principal.Role != "admin" {
		return httperr.New(httperr.FORBIDDEN, "admin access required")
	}

	// Extract and validate profileId from the URL path parameter.
	rawID := c.Param("profileId")
	if rawID == "" {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile_id is required")
	}
	profileUUID, err := uuid.Parse(rawID)
	if err != nil {
		return httperr.New(httperr.INVALID_UUID, "profile_id must be a valid UUID")
	}

	// Bind the optional tuning parameters for validation. The current service
	// interface accepts only profileID; gamma/tolerance/max_levels are
	// declared in the DTO for future service evolution and OpenAPI docs.
	var req dto.CommunityDetectRequest
	if bindErr := c.Bind(&req); bindErr != nil {
		return httperr.New(httperr.VALIDATION_ERROR, "malformed JSON body")
	}

	if err := h.detectSvc.Detect(ctx, profileUUID.String()); err != nil {
		if errors.Is(err, communityservice.ErrCommunityUnavailable) {
			return httperr.New(httperr.SERVICE_UNAVAILABLE, "community detection service unavailable")
		}
		if errors.Is(err, communityservice.ErrCommunityGraphTooLarge) {
			return httperr.New(httperr.ErrCommunityGraphTooLarge, "knowledge graph too large for community detection")
		}
		return httperr.New(httperr.INTERNAL_ERROR, "community detection failed")
	}

	communities, err := h.listSvc.List(ctx, profileUUID.String(), 0)
	if err != nil {
		return httperr.New(httperr.INTERNAL_ERROR, "community summary retrieval failed")
	}

	return response.SuccessOK(c, response.ToCommunityDetectResponse(communities))
}

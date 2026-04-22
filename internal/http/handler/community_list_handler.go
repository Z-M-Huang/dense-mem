package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/http/response"
	"github.com/dense-mem/dense-mem/internal/http/validation"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/communityservice"
)

const defaultCommunityListLimit = 20

// CommunityListHandler serves GET /api/v1/communities.
type CommunityListHandler struct {
	svc communityservice.ListCommunitiesService
}

// CommunityListHandlerInterface is the companion interface for CommunityListHandler.
type CommunityListHandlerInterface interface {
	Handle(c echo.Context) error
}

var _ CommunityListHandlerInterface = (*CommunityListHandler)(nil)

// NewCommunityListHandler constructs a CommunityListHandler.
func NewCommunityListHandler(svc communityservice.ListCommunitiesService) *CommunityListHandler {
	return &CommunityListHandler{svc: svc}
}

// Handle lists persisted community summaries in the resolved profile scope.
func (h *CommunityListHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	profileID, ok := middleware.GetResolvedProfileID(ctx)
	if !ok {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	var req dto.ListCommunitiesRequest
	if err := c.Bind(&req); err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, "invalid query parameters")
	}
	if err := validation.ValidateStruct(&req); err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, err.Error())
	}

	limit := req.Limit
	if limit == 0 {
		limit = defaultCommunityListLimit
	}

	communities, err := h.svc.List(ctx, profileID.String(), limit)
	if err != nil {
		return httperr.New(httperr.INTERNAL_ERROR, "failed to list communities")
	}

	return c.JSON(http.StatusOK, response.ToListCommunitiesResponse(communities))
}

package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/http/response"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/communityservice"
)

// CommunityReadHandler serves GET /api/v1/communities/:id.
type CommunityReadHandler struct {
	svc communityservice.GetCommunitySummaryService
}

// CommunityReadHandlerInterface is the companion interface for CommunityReadHandler.
type CommunityReadHandlerInterface interface {
	Handle(c echo.Context) error
}

var _ CommunityReadHandlerInterface = (*CommunityReadHandler)(nil)

// NewCommunityReadHandler constructs a CommunityReadHandler.
func NewCommunityReadHandler(svc communityservice.GetCommunitySummaryService) *CommunityReadHandler {
	return &CommunityReadHandler{svc: svc}
}

// Handle reads a persisted community summary within the resolved profile scope.
func (h *CommunityReadHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	profileID, ok := middleware.GetResolvedProfileID(ctx)
	if !ok {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	communityID := c.Param("id")
	if communityID == "" {
		return httperr.New(httperr.VALIDATION_ERROR, "community id is required")
	}

	community, err := h.svc.Get(ctx, profileID.String(), communityID)
	if err != nil {
		if errors.Is(err, communityservice.ErrCommunityNotFound) {
			return httperr.New(httperr.NOT_FOUND, "community not found")
		}
		return httperr.New(httperr.INTERNAL_ERROR, "failed to read community")
	}

	return c.JSON(http.StatusOK, response.ToCommunityResponse(community))
}

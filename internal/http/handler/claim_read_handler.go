package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/http/response"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/claimservice"
)

// ClaimReadHandler serves GET /api/v1/claims/:id.
type ClaimReadHandler struct {
	svc claimservice.GetClaimService
}

// ClaimReadHandlerInterface is the companion interface for ClaimReadHandler.
type ClaimReadHandlerInterface interface {
	Handle(c echo.Context) error
}

// Ensure ClaimReadHandler implements ClaimReadHandlerInterface.
var _ ClaimReadHandlerInterface = (*ClaimReadHandler)(nil)

// NewClaimReadHandler constructs a ClaimReadHandler.
func NewClaimReadHandler(svc claimservice.GetClaimService) *ClaimReadHandler {
	return &ClaimReadHandler{svc: svc}
}

// Handle reads a claim by its ID, scoped to the caller's profile.
// Missing claims and cross-profile reads both return 404.
func (h *ClaimReadHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	profileID, ok := middleware.GetResolvedProfileID(ctx)
	if !ok {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	claimID := c.Param("id")
	if claimID == "" {
		return httperr.New(httperr.VALIDATION_ERROR, "claim id is required")
	}

	claim, err := h.svc.Get(ctx, profileID.String(), claimID)
	if err != nil {
		if errors.Is(err, claimservice.ErrClaimNotFound) {
			return httperr.New(httperr.ErrClaimNotFound, "claim not found")
		}
		return httperr.New(httperr.INTERNAL_ERROR, "failed to read claim")
	}

	return c.JSON(http.StatusOK, response.ToClaimResponse(claim, ""))
}

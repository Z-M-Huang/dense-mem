package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/http/response"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
)

// FragmentReadHandler serves GET /api/v1/fragments/:id.
type FragmentReadHandler struct {
	svc fragmentservice.GetFragmentService
}

// FragmentReadHandlerInterface is the companion interface for FragmentReadHandler.
type FragmentReadHandlerInterface interface {
	Handle(c echo.Context) error
}

var _ FragmentReadHandlerInterface = (*FragmentReadHandler)(nil)

// NewFragmentReadHandler constructs a FragmentReadHandler.
func NewFragmentReadHandler(svc fragmentservice.GetFragmentService) *FragmentReadHandler {
	return &FragmentReadHandler{svc: svc}
}

// Handle reads a fragment by its ID, scoped to the caller's profile.
// Missing fragments and cross-profile reads both return 404 (AC-27).
// The response excludes the embedding vector (AC-28).
func (h *FragmentReadHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	profileID, ok := middleware.GetResolvedProfileID(ctx)
	if !ok {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	fragmentID := c.Param("id")
	if fragmentID == "" {
		return httperr.New(httperr.VALIDATION_ERROR, "fragment id is required")
	}

	frag, err := h.svc.GetByID(ctx, profileID.String(), fragmentID)
	if err != nil {
		if errors.Is(err, fragmentservice.ErrFragmentNotFound) {
			return httperr.New(httperr.NOT_FOUND, "fragment not found")
		}
		return httperr.New(httperr.INTERNAL_ERROR, "failed to read fragment")
	}

	return c.JSON(http.StatusOK, response.ToFragmentResponse(frag))
}

package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	dto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/http/response"
	"github.com/dense-mem/dense-mem/internal/http/validation"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
)

// FragmentListHandler serves GET /api/v1/fragments.
type FragmentListHandler struct {
	svc fragmentservice.ListFragmentsService
}

// FragmentListHandlerInterface is the companion interface for FragmentListHandler.
type FragmentListHandlerInterface interface {
	Handle(c echo.Context) error
}

var _ FragmentListHandlerInterface = (*FragmentListHandler)(nil)

// NewFragmentListHandler constructs a FragmentListHandler.
func NewFragmentListHandler(svc fragmentservice.ListFragmentsService) *FragmentListHandler {
	return &FragmentListHandler{svc: svc}
}

// Handle lists fragments in the caller's profile with keyset pagination.
func (h *FragmentListHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	profileID, ok := middleware.GetResolvedProfileID(ctx)
	if !ok {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	var req dto.ListFragmentsRequest
	if err := c.Bind(&req); err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, "invalid query parameters")
	}
	if err := validation.ValidateStruct(&req); err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, err.Error())
	}

	fragments, nextCursor, err := h.svc.List(ctx, profileID.String(), fragmentservice.ListOptions{
		Limit:      req.Limit,
		Cursor:     req.Cursor,
		SourceType: req.SourceType,
	})
	if err != nil {
		if errors.Is(err, fragmentservice.ErrInvalidCursor) {
			return httperr.New(httperr.VALIDATION_ERROR, "invalid cursor")
		}
		return httperr.New(httperr.INTERNAL_ERROR, "failed to list fragments")
	}

	items := make([]dto.FragmentResponse, 0, len(fragments))
	for i := range fragments {
		items = append(items, *response.ToFragmentResponse(&fragments[i]))
	}

	return c.JSON(http.StatusOK, dto.ListFragmentsResponse{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	})
}

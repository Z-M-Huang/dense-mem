package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/domain"
	dto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/http/response"
	"github.com/dense-mem/dense-mem/internal/http/validation"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/factservice"
)

const (
	defaultFactListHandlerLimit = 20
	maxFactListHandlerLimit     = 100
)

// FactListHandler serves GET /api/v1/facts.
type FactListHandler struct {
	svc factservice.ListFactsService
}

// FactListHandlerInterface is the companion interface for FactListHandler.
type FactListHandlerInterface interface {
	Handle(c echo.Context) error
}

var _ FactListHandlerInterface = (*FactListHandler)(nil)

// NewFactListHandler constructs a FactListHandler.
func NewFactListHandler(svc factservice.ListFactsService) *FactListHandler {
	return &FactListHandler{svc: svc}
}

// Handle lists facts in the caller's profile with keyset pagination and optional filters.
func (h *FactListHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	profileID, ok := middleware.GetResolvedProfileID(ctx)
	if !ok {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	var req dto.ListFactsRequest
	if err := c.Bind(&req); err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, "invalid query parameters")
	}
	if err := validation.ValidateStruct(&req); err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, err.Error())
	}

	limit := req.Limit
	if limit == 0 {
		limit = defaultFactListHandlerLimit
	}

	filters := factservice.FactListFilters{
		Subject:   req.Subject,
		Predicate: req.Predicate,
	}
	if req.Status != "" {
		filters.Status = domain.FactStatus(req.Status)
	}

	facts, nextCursor, err := h.svc.List(ctx, profileID.String(), filters, limit, req.Cursor)
	if err != nil {
		return httperr.New(httperr.INTERNAL_ERROR, "failed to list facts")
	}

	return c.JSON(http.StatusOK, response.ToListFactsResponse(facts, nextCursor))
}

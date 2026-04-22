package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/http/response"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/factservice"
)

// FactReadHandler serves GET /api/v1/facts/:id.
type FactReadHandler struct {
	svc factservice.GetFactService
}

// FactReadHandlerInterface is the companion interface for FactReadHandler.
type FactReadHandlerInterface interface {
	Handle(c echo.Context) error
}

var _ FactReadHandlerInterface = (*FactReadHandler)(nil)

// NewFactReadHandler constructs a FactReadHandler.
func NewFactReadHandler(svc factservice.GetFactService) *FactReadHandler {
	return &FactReadHandler{svc: svc}
}

// Handle reads a fact by its ID, scoped to the caller's profile.
// Missing facts and cross-profile reads both return 404 (profile isolation invariant).
func (h *FactReadHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	profileID, ok := middleware.GetResolvedProfileID(ctx)
	if !ok {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	factID := c.Param("id")
	if factID == "" {
		return httperr.New(httperr.VALIDATION_ERROR, "fact id is required")
	}

	fact, err := h.svc.Get(ctx, profileID.String(), factID)
	if err != nil {
		if errors.Is(err, factservice.ErrFactNotFound) {
			return httperr.New(httperr.ErrFactNotFound, "fact not found")
		}
		return httperr.New(httperr.INTERNAL_ERROR, "failed to read fact")
	}

	var validAt *time.Time
	if raw := c.QueryParam("valid_at"); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			return httperr.New(httperr.VALIDATION_ERROR, "valid_at must be RFC3339")
		}
		validAt = &parsed
	}
	var knownAt *time.Time
	if raw := c.QueryParam("known_at"); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			return httperr.New(httperr.VALIDATION_ERROR, "known_at must be RFC3339")
		}
		knownAt = &parsed
	}
	if !factMatchesTemporalWindow(fact, validAt, knownAt) {
		return httperr.New(httperr.ErrFactNotFound, "fact not found")
	}

	if c.QueryParam("include_evidence") != "true" {
		factCopy := *fact
		factCopy.Evidence = nil
		fact = &factCopy
	}

	return c.JSON(http.StatusOK, response.ToFactResponse(fact))
}

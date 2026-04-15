package handler

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/tools/admingraph"
)

// AdminGraphRequest represents the request body for admin graph query.
type AdminGraphRequest struct {
	Query     string         `json:"query" validate:"required"`
	ProfileID string         `json:"profile_id" validate:"required"` // UUID
	Params    map[string]any `json:"params,omitempty"`
}

// AdminGraphResponse represents the response for admin graph query.
type AdminGraphResponse struct {
	Data AdminGraphData `json:"data"`
}

// AdminGraphData represents the data portion of the response.
type AdminGraphData struct {
	Columns  []string         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	RowCount int              `json:"row_count"`
}

// AdminGraphServiceInterface defines the interface for admin graph service.
type AdminGraphServiceInterface interface {
	ExecuteWithAudit(ctx context.Context, profileID string, query string, params map[string]any, actorKeyID *string, actorRole, clientIP, correlationID string) (*admingraph.AdminGraphResult, error)
}

// AdminGraphHandler handles HTTP requests for admin graph query operations.
type AdminGraphHandler struct {
	svc AdminGraphServiceInterface
}

// AdminGraphHandlerInterface is the companion interface for AdminGraphHandler.
type AdminGraphHandlerInterface interface {
	Handle(c echo.Context) error
}

// Ensure AdminGraphHandler implements AdminGraphHandlerInterface.
var _ AdminGraphHandlerInterface = (*AdminGraphHandler)(nil)

// NewAdminGraphHandler creates a new admin graph handler.
func NewAdminGraphHandler(svc AdminGraphServiceInterface) *AdminGraphHandler {
	return &AdminGraphHandler{svc: svc}
}

// Handle handles POST /api/v1/admin/graph/query.
// It validates the request, executes the query with audit logging, and returns results.
func (h *AdminGraphHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	// Get principal from context (set by AuthMiddleware)
	principal := middleware.GetPrincipal(ctx)
	if principal == nil {
		return httperr.New(httperr.AUTH_MISSING, "authentication required")
	}

	// Verify admin role (AdminOnly middleware should already enforce this, but defensive)
	if principal.Role != "admin" {
		return httperr.New(httperr.FORBIDDEN, "admin access required")
	}

	// Bind request body
	var req AdminGraphRequest
	if err := c.Bind(&req); err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, "malformed JSON body")
	}

	// Validate required fields
	if req.Query == "" {
		return httperr.New(httperr.VALIDATION_ERROR, "query is required")
	}

	if req.ProfileID == "" {
		return httperr.New(httperr.VALIDATION_ERROR, "profile_id is required")
	}

	// Validate profile_id is a valid UUID
	profileUUID, err := uuid.Parse(req.ProfileID)
	if err != nil {
		return httperr.New(httperr.INVALID_UUID, "profile_id must be a valid UUID")
	}

	// Get audit metadata
	actorKeyID := principal.KeyID.String()
	clientIP := c.RealIP()
	correlationID := middleware.GetCorrelationID(ctx)

	// Execute query with audit
	result, err := h.svc.ExecuteWithAudit(
		ctx,
		profileUUID.String(),
		req.Query,
		req.Params,
		&actorKeyID,
		principal.Role,
		clientIP,
		correlationID,
	)

	if err != nil {
		return handleAdminGraphError(err)
	}

	// Build response
	response := AdminGraphResponse{
		Data: AdminGraphData{
			Columns:  result.Columns,
			Rows:     result.Rows,
			RowCount: result.RowCount,
		},
	}

	return c.JSON(http.StatusOK, response)
}

// handleAdminGraphError converts service errors to HTTP errors.
func handleAdminGraphError(err error) *httperr.APIError {
	if err == nil {
		return nil
	}

	// Check for specific error types
	if admingraph.IsTimeoutError(err) {
		return httperr.New(httperr.SERVICE_UNAVAILABLE, "query exceeded timeout")
	}

	if admingraph.IsForbiddenParamError(err) {
		return httperr.New(httperr.VALIDATION_ERROR, err.Error())
	}

	if admingraph.IsSyntaxError(err) {
		return httperr.New(httperr.VALIDATION_ERROR, err.Error())
	}

	// Check for validation errors
	if admingraph.IsValidationError(err) {
		return httperr.New(httperr.VALIDATION_ERROR, err.Error())
	}

	// Default to internal error (sanitized)
	return httperr.New(httperr.INTERNAL_ERROR, "query execution failed")
}
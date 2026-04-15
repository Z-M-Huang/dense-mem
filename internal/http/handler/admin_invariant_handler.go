package handler

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service"
)

// InvariantScanResponse represents the response for invariant scan.
type InvariantScanResponse struct {
	Data InvariantScanData `json:"data"`
}

// InvariantScanData represents the data portion of the response.
type InvariantScanData struct {
	Violations int                       `json:"violations"`
	Status     string                    `json:"status"`
	Findings   []service.InvariantFinding `json:"findings,omitempty"`
}

// InvariantScanServiceInterface defines the interface for invariant scan service.
type InvariantScanServiceInterface interface {
	ScanWithAudit(ctx context.Context, actorKeyID *string, actorRole, clientIP, correlationID string) (*service.InvariantScanResult, error)
}

// InvariantScanHandler handles HTTP requests for invariant scan operations.
type InvariantScanHandler struct {
	svc InvariantScanServiceInterface
}

// InvariantScanHandlerInterface is the companion interface for InvariantScanHandler.
type InvariantScanHandlerInterface interface {
	Handle(c echo.Context) error
}

// Ensure InvariantScanHandler implements InvariantScanHandlerInterface.
var _ InvariantScanHandlerInterface = (*InvariantScanHandler)(nil)

// NewInvariantScanHandler creates a new invariant scan handler.
func NewInvariantScanHandler(svc InvariantScanServiceInterface) *InvariantScanHandler {
	return &InvariantScanHandler{svc: svc}
}

// Handle handles POST /api/v1/admin/invariant-scan.
// It executes the invariant scan, logs the result to audit, and returns findings.
// This endpoint is admin-only and requires the AdminOnly middleware.
func (h *InvariantScanHandler) Handle(c echo.Context) error {
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

	// Get audit metadata
	actorKeyID := principal.KeyID.String()
	clientIP := c.RealIP()
	correlationID := middleware.GetCorrelationID(ctx)

	// Execute scan with audit logging
	result, err := h.svc.ScanWithAudit(ctx, &actorKeyID, principal.Role, clientIP, correlationID)

	if err != nil {
		return httperr.New(httperr.INTERNAL_ERROR, "invariant scan failed")
	}

	// Build response
	response := InvariantScanResponse{
		Data: InvariantScanData{
			Violations: result.Violations,
			Status:     result.Status,
			Findings:   result.Findings,
		},
	}

	return c.JSON(http.StatusOK, response)
}
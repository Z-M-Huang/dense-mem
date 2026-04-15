package middleware

import (
	"context"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// CorrelationIDHeader is the HTTP header for correlation IDs.
const CorrelationIDHeader = "X-Correlation-ID"

// CorrelationIDContextKey is the context key for correlation IDs.
type CorrelationIDContextKey struct{}

// CorrelationIDMiddleware reads or generates a correlation ID for each request.
// It reads X-Correlation-ID from the request header if present,
// otherwise generates a new UUID. The correlation ID is stored in the
// request context and echoed back in the response header.
func CorrelationIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Try to read existing correlation ID from header
			correlationID := c.Request().Header.Get(CorrelationIDHeader)

			// If not present, generate a new UUID
			if correlationID == "" {
				correlationID = uuid.New().String()
			}

			// Store in context
			ctx := context.WithValue(c.Request().Context(), CorrelationIDContextKey{}, correlationID)
			c.SetRequest(c.Request().WithContext(ctx))

			// Echo back in response header
			c.Response().Header().Set(CorrelationIDHeader, correlationID)

			return next(c)
		}
	}
}

// GetCorrelationID retrieves the correlation ID from the context.
// Returns empty string if not found.
func GetCorrelationID(ctx context.Context) string {
	if id, ok := ctx.Value(CorrelationIDContextKey{}).(string); ok {
		return id
	}
	return ""
}

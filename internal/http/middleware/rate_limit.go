package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/config"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service"
)

// RateLimitMiddleware creates a rate limiting middleware using the fixed-window algorithm.
// It reads the Principal from context to determine the profile and role for tier selection.
// Admin roles use AdminRateLimitPerMinute, standard profiles use RateLimitPerMinute.
func RateLimitMiddleware(svc service.RateLimitServiceInterface, cfg config.ConfigProvider, auditSvc service.AuditService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get the principal from context
			principal := GetPrincipal(c.Request().Context())
			if principal == nil {
				// No principal means auth middleware didn't run - let it through
				// so auth errors take precedence
				return next(c)
			}

			// Get profile ID - admin keys have no profile
			var profileID string
			if principal.ProfileID != nil {
				profileID = principal.ProfileID.String()
			} else {
				// Admin keys without profile use special identifier
				profileID = fmt.Sprintf("admin:%s", principal.KeyID.String())
			}

			// Get route path for stable bucket
			routePath := c.Path()

			// Select limit tier based on role
			var limit int
			if principal.Role == "admin" {
				limit = cfg.GetAdminRateLimitPerMinute()
			} else {
				limit = cfg.GetRateLimitPerMinute()
			}

			// Perform rate limit check
			ctx := c.Request().Context()
			allowed, remaining, resetAt, err := svc.Check(ctx, profileID, routePath, limit)
			if err != nil {
				// On error, let the request through (fail open)
				// But log the error
				c.Logger().Errorf("rate limit check failed: %v", err)
				return next(c)
			}

			// Set rate limit headers on all responses
			c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			c.Response().Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			c.Response().Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

			if !allowed {
				// Calculate retry-after seconds
				retryAfter := int(time.Until(resetAt).Seconds())
				if retryAfter < 0 {
					retryAfter = 0
				}
				c.Response().Header().Set("Retry-After", strconv.Itoa(retryAfter))

				// Log and audit rate limit hit
				logRateLimit(c, auditSvc, profileID, routePath, limit, remaining, resetAt)

				return httperr.New(httperr.RATE_LIMITED, "rate limit exceeded")
			}

			return next(c)
		}
	}
}

// logRateLimit logs and audits a rate limit event.
func logRateLimit(c echo.Context, auditSvc service.AuditService, profileID, routePath string, limit, remaining int, resetAt time.Time) {
	if auditSvc == nil {
		return
	}

	clientIP := c.RealIP()
	correlationID := GetCorrelationID(c.Request().Context())

	metadata := map[string]interface{}{
		"route_path": routePath,
		"limit":      limit,
		"remaining":  remaining,
		"reset_at":    resetAt.Unix(),
	}

	// Use a background context with timeout for logging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var profileIDPtr *string
	if profileID != "" {
		profileIDPtr = &profileID
	}

	_ = auditSvc.RateLimited(ctx, profileIDPtr, "request", metadata, clientIP, correlationID)
}
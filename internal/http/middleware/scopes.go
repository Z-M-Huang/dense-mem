package middleware

import (
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/httperr"
)

// RequireScopes creates a middleware that enforces scope requirements.
//
// Authorization rules:
// 1. If principal has admin role, allow access (admin bypass).
// 2. If principal has all required scopes, allow access.
// 3. Otherwise, deny with 403 FORBIDDEN.
//
// This middleware must run after AuthMiddleware.
func RequireScopes(required ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			principal := GetPrincipal(ctx)

			// If no principal, deny (RequireAuth should have been used first)
			if principal == nil {
				return httperr.New(httperr.FORBIDDEN, "authentication required")
			}

			// Admin principals bypass scope checks
			if principal.Role == "admin" {
				return next(c)
			}

			// Check that principal has all required scopes
			if hasAllScopes(principal.Scopes, required) {
				return next(c)
			}

			// Missing required scope(s)
			return httperr.New(httperr.FORBIDDEN, "insufficient permissions")
		}
	}
}

// AdminOnly creates a middleware that restricts access to admin principals only.
//
// Authorization rules:
// 1. If principal has admin role, allow access.
// 2. Otherwise, deny with 403 FORBIDDEN.
//
// This middleware must run after AuthMiddleware.
func AdminOnly() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			principal := GetPrincipal(ctx)

			// If no principal, deny (RequireAuth should have been used first)
			if principal == nil {
				return httperr.New(httperr.FORBIDDEN, "authentication required")
			}

			// Only admin principals are allowed
			if principal.Role == "admin" {
				return next(c)
			}

			// Standard keys are forbidden
			return httperr.New(httperr.FORBIDDEN, "admin access required")
		}
	}
}

// hasAllScopes checks if the provided scopes contain all required scopes.
func hasAllScopes(scopes []string, required []string) bool {
	scopeSet := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		scopeSet[s] = true
	}

	for _, r := range required {
		if !scopeSet[r] {
			return false
		}
	}

	return true
}
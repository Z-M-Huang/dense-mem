package middleware

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/httperr"
)

// ProfileResolutionServiceInterface defines the minimal interface the middleware
// needs to resolve a profile. Handler and middleware share the GetByID method
// name on the concrete ProfileService implementation, so the middleware
// re-declares only that single method here to avoid importing the handler
// package and creating a cycle.
type ProfileResolutionServiceInterface interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Profile, error)
}

// ResolvedProfileKey is the typed context key for storing the resolved profile ID.
// Using a typed context key prevents accidental overwrites from other packages.
type ResolvedProfileKey struct{}

// ProfileIDHeader is the HTTP header for profile ID (used by tool routes).
const ProfileIDHeader = "X-Profile-ID"

// ProfileResolutionMiddleware creates a middleware that resolves and validates
// profile IDs from either path parameters or headers.
//
// For profile-scoped routes (/api/v1/profiles/:profileId/*): reads :profileId param
// For tool routes (/api/v1/tools/*): reads X-Profile-ID header
//
// The middleware:
// - Validates that a profile ID is provided (returns 400 PROFILE_ID_REQUIRED if missing)
// - Validates the UUID format (returns 400 INVALID_UUID if malformed)
// - Resolves the profile through the service (returns 404 NOT_FOUND if not found or deleted)
// - Stores the resolved profile ID in context for downstream use
//
// This middleware does NOT perform authorization - it only resolves and loads.
func ProfileResolutionMiddleware(svc ProfileResolutionServiceInterface) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			principal := GetPrincipal(ctx)

			var profileIDStr string
			var isToolRoute bool

			// Determine route type and extract profile ID accordingly
			path := c.Request().URL.Path

			if strings.HasPrefix(path, "/api/v1/tools") {
				// Tool route: read from X-Profile-ID header
				isToolRoute = true
				profileIDStr = c.Request().Header.Get(ProfileIDHeader)
			} else if strings.HasPrefix(path, "/api/v1/profiles/") {
				// Profile route: read from :profileId path param
				isToolRoute = false
				profileIDStr = c.Param("profileId")
			} else {
				// Not a route that needs profile resolution, pass through
				return next(c)
			}

			// Validate profile ID is present
			if profileIDStr == "" {
				return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
			}

			// Parse and validate UUID
			profileID, err := uuid.Parse(profileIDStr)
			if err != nil {
				return httperr.New(httperr.INVALID_UUID, "invalid profile ID format")
			}

			// Resolve profile through service
			// The service returns NOT_FOUND for non-existent or soft-deleted profiles
			profile, err := svc.GetByID(ctx, profileID)
			if err != nil {
				// Check if it's a NOT_FOUND error
				if apiErr, ok := err.(*httperr.APIError); ok && apiErr.Code == httperr.NOT_FOUND {
					return httperr.New(httperr.NOT_FOUND, "profile not found")
				}
				// Other errors are internal
				return httperr.New(httperr.INTERNAL_ERROR, "failed to resolve profile")
			}

			if profile == nil {
				return httperr.New(httperr.NOT_FOUND, "profile not found")
			}

			// Store resolved profile ID in context
			// Use a dedicated typed key to prevent collisions
			resolvedCtx := context.WithValue(ctx, ResolvedProfileKey{}, profileID)
			c.SetRequest(c.Request().WithContext(resolvedCtx))

			// Continue to next handler
			_ = principal // Principal is available for authorization decisions in downstream middleware
			_ = isToolRoute // Route type available for future authorization logic

			return next(c)
		}
	}
}

// GetResolvedProfileID retrieves the resolved profile ID from the context.
// Returns the profile ID and true if found, or uuid.Nil and false if not found.
func GetResolvedProfileID(ctx context.Context) (uuid.UUID, bool) {
	if id, ok := ctx.Value(ResolvedProfileKey{}).(uuid.UUID); ok {
		return id, true
	}
	return uuid.Nil, false
}

// MustGetResolvedProfileID retrieves the resolved profile ID from the context.
// Panics if not found. Use only when profile resolution is guaranteed.
func MustGetResolvedProfileID(ctx context.Context) uuid.UUID {
	id, ok := GetResolvedProfileID(ctx)
	if !ok {
		panic("profile resolution middleware: profile ID not found in context")
	}
	return id
}

// SetResolvedProfileIDForTest sets a resolved profile ID in context for testing purposes.
// This is intended for use in unit tests to bypass profile resolution middleware.
func SetResolvedProfileIDForTest(ctx context.Context, profileID uuid.UUID) context.Context {
	return context.WithValue(ctx, ResolvedProfileKey{}, profileID)
}
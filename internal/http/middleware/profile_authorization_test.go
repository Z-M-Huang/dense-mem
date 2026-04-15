package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

// mockProfileAuthorizationService is a mock implementation of ProfileAuthorizationService
type mockProfileAuthorizationService struct {
	crossProfileDeniedCalled bool
	crossProfileDeniedParams struct {
		actorProfileID  string
		targetProfileID string
		operation       string
		metadata        map[string]interface{}
		clientIP        string
		correlationID   string
	}
	adminBypassCalled bool
	adminBypassParams struct {
		operation     string
		reason        string
		metadata      map[string]interface{}
		actorKeyID    *string
		actorRole     string
		clientIP      string
		correlationID string
	}
}

func (m *mockProfileAuthorizationService) CrossProfileDenied(ctx context.Context, actorProfileID, targetProfileID string, operation string, metadata map[string]interface{}, clientIP, correlationID string) error {
	m.crossProfileDeniedCalled = true
	m.crossProfileDeniedParams.actorProfileID = actorProfileID
	m.crossProfileDeniedParams.targetProfileID = targetProfileID
	m.crossProfileDeniedParams.operation = operation
	m.crossProfileDeniedParams.metadata = metadata
	m.crossProfileDeniedParams.clientIP = clientIP
	m.crossProfileDeniedParams.correlationID = correlationID
	return nil
}

func (m *mockProfileAuthorizationService) AdminBypass(ctx context.Context, operation string, reason string, metadata map[string]interface{}, actorKeyID *string, actorRole, clientIP, correlationID string) error {
	m.adminBypassCalled = true
	m.adminBypassParams.operation = operation
	m.adminBypassParams.reason = reason
	m.adminBypassParams.metadata = metadata
	m.adminBypassParams.actorKeyID = actorKeyID
	m.adminBypassParams.actorRole = actorRole
	m.adminBypassParams.clientIP = clientIP
	m.adminBypassParams.correlationID = correlationID
	return nil
}

// TestProfileAuthorization_AdminPassThrough tests that admin principals bypass profile authorization.
func TestProfileAuthorization_AdminPassThrough(t *testing.T) {
	e := newTestEcho()
	mockAuthzSvc := &mockProfileAuthorizationService{}
	targetProfileID := uuid.New()

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Set admin principal
			ctx := c.Request().Context()
			ctx = context.WithValue(ctx, principalContextKey{}, &Principal{
				KeyID:     uuid.New(),
				ProfileID: nil, // Admin has no profile
				Role:      "admin",
				Scopes:    []string{"admin"},
			})
			// Set target profile in context
			ctx = context.WithValue(ctx, ResolvedProfileKey{}, targetProfileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(AuthorizeProfile(mockAuthzSvc))

	handlerCalled := false
	e.GET("/test", func(c echo.Context) error {
		handlerCalled = true
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.True(t, handlerCalled, "handler should be called for admin")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, mockAuthzSvc.crossProfileDeniedCalled, "CrossProfileDenied should not be called for admin")
	assert.True(t, mockAuthzSvc.adminBypassCalled, "AdminBypass should be audited when an admin takes a cross-profile path")
	assert.Equal(t, "profile_access", mockAuthzSvc.adminBypassParams.operation)
	assert.Equal(t, "admin", mockAuthzSvc.adminBypassParams.actorRole)
	assert.Equal(t, targetProfileID.String(), mockAuthzSvc.adminBypassParams.metadata["target_profile_id"])
}

// TestProfileAuthorization_SameProfile_Allowed tests that standard principals can access their own profile.
func TestProfileAuthorization_SameProfile_Allowed(t *testing.T) {
	e := newTestEcho()
	mockAuthzSvc := &mockProfileAuthorizationService{}
	profileID := uuid.New()

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Set standard principal with matching profile
			ctx := c.Request().Context()
			ctx = context.WithValue(ctx, principalContextKey{}, &Principal{
				KeyID:     uuid.New(),
				ProfileID: &profileID,
				Role:      "standard",
				Scopes:    []string{"read", "write"},
			})
			// Set same target profile in context
			ctx = context.WithValue(ctx, ResolvedProfileKey{}, profileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(AuthorizeProfile(mockAuthzSvc))

	handlerCalled := false
	e.GET("/test", func(c echo.Context) error {
		handlerCalled = true
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.True(t, handlerCalled, "handler should be called for same profile")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, mockAuthzSvc.crossProfileDeniedCalled, "CrossProfileDenied should not be called for same profile")
}

// TestProfileAuthorization_DifferentProfile_Forbidden tests that cross-profile access is denied.
func TestProfileAuthorization_DifferentProfile_Forbidden(t *testing.T) {
	e := newTestEcho()
	mockAuthzSvc := &mockProfileAuthorizationService{}
	actorProfileID := uuid.New()
	targetProfileID := uuid.New()

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Set standard principal with different profile
			ctx := c.Request().Context()
			ctx = context.WithValue(ctx, principalContextKey{}, &Principal{
				KeyID:     uuid.New(),
				ProfileID: &actorProfileID,
				Role:      "standard",
				Scopes:    []string{"read", "write"},
			})
			// Set different target profile in context
			ctx = context.WithValue(ctx, ResolvedProfileKey{}, targetProfileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(AuthorizeProfile(mockAuthzSvc))

	handlerCalled := false
	e.GET("/test", func(c echo.Context) error {
		handlerCalled = true
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.False(t, handlerCalled, "handler should not be called for cross-profile access")
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "FORBIDDEN")
	assert.True(t, mockAuthzSvc.crossProfileDeniedCalled, "CrossProfileDenied should be called")
	assert.Equal(t, actorProfileID.String(), mockAuthzSvc.crossProfileDeniedParams.actorProfileID)
	assert.Equal(t, targetProfileID.String(), mockAuthzSvc.crossProfileDeniedParams.targetProfileID)
}

// TestProfileAuthorization_NoTargetProfile_PassThrough tests that requests without a target profile pass through.
func TestProfileAuthorization_NoTargetProfile_PassThrough(t *testing.T) {
	e := newTestEcho()
	mockAuthzSvc := &mockProfileAuthorizationService{}
	profileID := uuid.New()

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Set standard principal
			ctx := c.Request().Context()
			ctx = context.WithValue(ctx, principalContextKey{}, &Principal{
				KeyID:     uuid.New(),
				ProfileID: &profileID,
				Role:      "standard",
				Scopes:    []string{"read", "write"},
			})
			// No target profile set in context
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(AuthorizeProfile(mockAuthzSvc))

	handlerCalled := false
	e.GET("/test", func(c echo.Context) error {
		handlerCalled = true
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.True(t, handlerCalled, "handler should be called when no target profile")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, mockAuthzSvc.crossProfileDeniedCalled, "CrossProfileDenied should not be called")
}

// TestProfileAuthorization_NilPrincipal_Forbidden verifies the middleware fails
// closed when no principal is set (e.g., auth middleware misordered or bypassed).
func TestProfileAuthorization_NilPrincipal_Forbidden(t *testing.T) {
	e := newTestEcho()
	mockAuthzSvc := &mockProfileAuthorizationService{}
	targetProfileID := uuid.New()

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Intentionally do NOT set a principal; simulate missing auth.
			ctx := c.Request().Context()
			ctx = context.WithValue(ctx, ResolvedProfileKey{}, targetProfileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(AuthorizeProfile(mockAuthzSvc))

	handlerCalled := false
	e.GET("/test", func(c echo.Context) error {
		handlerCalled = true
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.False(t, handlerCalled, "handler should not be called when principal is nil")
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "FORBIDDEN")
}

// TestRequireScopes_AdminBypasses tests that admin principals bypass scope checks.
func TestRequireScopes_AdminBypasses(t *testing.T) {
	e := newTestEcho()

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = context.WithValue(ctx, principalContextKey{}, &Principal{
				KeyID:     uuid.New(),
				ProfileID: nil,
				Role:      "admin",
				Scopes:    []string{"admin"},
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(RequireScopes("read", "write", "delete"))

	handlerCalled := false
	e.GET("/test", func(c echo.Context) error {
		handlerCalled = true
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.True(t, handlerCalled, "handler should be called for admin")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestRequireScopes_MatchingScopes_Allowed tests that principals with all required scopes are allowed.
func TestRequireScopes_MatchingScopes_Allowed(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = context.WithValue(ctx, principalContextKey{}, &Principal{
				KeyID:     uuid.New(),
				ProfileID: &profileID,
				Role:      "standard",
				Scopes:    []string{"read", "write", "delete"},
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(RequireScopes("read", "write"))

	handlerCalled := false
	e.GET("/test", func(c echo.Context) error {
		handlerCalled = true
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.True(t, handlerCalled, "handler should be called when all scopes present")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestRequireScopes_MissingScope_Forbidden tests that missing scopes result in 403.
func TestRequireScopes_MissingScope_Forbidden(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = context.WithValue(ctx, principalContextKey{}, &Principal{
				KeyID:     uuid.New(),
				ProfileID: &profileID,
				Role:      "standard",
				Scopes:    []string{"read"}, // missing "write" and "delete"
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(RequireScopes("read", "write", "delete"))

	handlerCalled := false
	e.GET("/test", func(c echo.Context) error {
		handlerCalled = true
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.False(t, handlerCalled, "handler should not be called when scopes missing")
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "FORBIDDEN")
}

// TestAdminOnly_AdminAllowed tests that admin principals are allowed.
func TestAdminOnly_AdminAllowed(t *testing.T) {
	e := newTestEcho()

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = context.WithValue(ctx, principalContextKey{}, &Principal{
				KeyID:     uuid.New(),
				ProfileID: nil,
				Role:      "admin",
				Scopes:    []string{"admin"},
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(AdminOnly())

	handlerCalled := false
	e.GET("/test", func(c echo.Context) error {
		handlerCalled = true
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.True(t, handlerCalled, "handler should be called for admin")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestAdminOnly_StandardKey_Forbidden tests that standard keys are blocked.
func TestAdminOnly_StandardKey_Forbidden(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = context.WithValue(ctx, principalContextKey{}, &Principal{
				KeyID:     uuid.New(),
				ProfileID: &profileID,
				Role:      "standard",
				Scopes:    []string{"read", "write"},
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(AdminOnly())

	handlerCalled := false
	e.GET("/test", func(c echo.Context) error {
		handlerCalled = true
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.False(t, handlerCalled, "handler should not be called for standard key")
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "FORBIDDEN")
}
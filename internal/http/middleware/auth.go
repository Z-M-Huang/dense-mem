package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/crypto"
	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/repository"
	"github.com/dense-mem/dense-mem/internal/service"
)

// Principal represents the authenticated principal stored in context.
type Principal struct {
	KeyID     uuid.UUID
	ProfileID *uuid.UUID
	Role      string
	Scopes    []string
	KeyPrefix string
}

// PrincipalInterface is the companion interface for Principal.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type PrincipalInterface interface {
	GetKeyID() uuid.UUID
	GetProfileID() *uuid.UUID
	GetRole() string
	GetScopes() []string
	GetKeyPrefix() string
}

// Ensure Principal implements PrincipalInterface
var _ PrincipalInterface = (*Principal)(nil)

// Getters for PrincipalInterface
func (p *Principal) GetKeyID() uuid.UUID      { return p.KeyID }
func (p *Principal) GetProfileID() *uuid.UUID { return p.ProfileID }
func (p *Principal) GetRole() string          { return p.Role }
func (p *Principal) GetScopes() []string      { return p.Scopes }
func (p *Principal) GetKeyPrefix() string     { return p.KeyPrefix }

// principalContextKey is the unexported context key type for storing principals.
// Using an unexported type prevents downstream code from constructing fake principals.
type principalContextKey struct{}

// AuthMiddleware creates an authentication middleware that validates API keys.
// It requires the Authorization header in the format "Bearer <rawKey>".
func AuthMiddleware(repo repository.APIKeyRepository, auditSvc service.AuditService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Extract Authorization header
			authHeader := c.Request().Header.Get("Authorization")

			// Missing header
			if authHeader == "" {
				logAuthFailure(c, auditSvc, nil, "AUTH_MISSING", "missing authorization header")
				return httperr.New(httperr.AUTH_MISSING, "missing authorization header")
			}

			// Parse Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				logAuthFailure(c, auditSvc, nil, "AUTH_INVALID", "malformed authorization header")
				return httperr.New(httperr.AUTH_INVALID, "malformed authorization header")
			}

			rawKey := parts[1]
			if rawKey == "" {
				logAuthFailure(c, auditSvc, nil, "AUTH_INVALID", "empty bearer token")
				return httperr.New(httperr.AUTH_INVALID, "empty bearer token")
			}

			// Extract prefix (first 12 characters)
			if len(rawKey) < 12 {
				logAuthFailure(c, auditSvc, nil, "AUTH_INVALID", "invalid key format")
				return httperr.New(httperr.AUTH_INVALID, "invalid key format")
			}
			prefix := rawKey[:12]

			// Look up active key by prefix
			ctx := c.Request().Context()
			key, err := repo.GetActiveByPrefix(ctx, prefix)
			if err != nil {
				return httperr.New(httperr.INTERNAL_ERROR, "failed to lookup key")
			}

			// No matching key found
			if key == nil {
				logAuthFailure(c, auditSvc, nil, "AUTH_INVALID", "invalid api key")
				return httperr.New(httperr.AUTH_INVALID, "invalid api key")
			}

			// Check if key is revoked (shouldn't happen with GetActiveByPrefix, but defensive)
			if key.RevokedAt != nil {
				profileID := key.ProfileID.String()
				logAuthFailure(c, auditSvc, &profileID, "AUTH_REVOKED", "api key has been revoked")
				return httperr.New(httperr.AUTH_REVOKED, "api key has been revoked")
			}

			// Check if key is expired (shouldn't happen with GetActiveByPrefix, but defensive)
			if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now().UTC()) {
				profileID := key.ProfileID.String()
				logAuthFailure(c, auditSvc, &profileID, "AUTH_EXPIRED", "api key has expired")
				return httperr.New(httperr.AUTH_EXPIRED, "api key has expired")
			}

			// Verify the raw key against the stored Argon2id hash
			if !crypto.VerifyKey(rawKey, key.KeyHash) {
				profileID := key.ProfileID.String()
				logAuthFailure(c, auditSvc, &profileID, "AUTH_INVALID", "invalid api key")
				return httperr.New(httperr.AUTH_INVALID, "invalid api key")
			}

			// All runtime keys must be profile-bound. Legacy profile-less keys are
			// rejected so the server only accepts the multi-tenant bearer model.
			if key.ProfileID == uuid.Nil {
				logAuthFailure(c, auditSvc, nil, "AUTH_INVALID", "api key is not profile bound")
				return httperr.New(httperr.AUTH_INVALID, "invalid api key")
			}

			principal := &Principal{
				KeyID:     key.ID,
				ProfileID: &key.ProfileID,
				Role:      "standard",
				Scopes:    key.Scopes,
				KeyPrefix: prefix,
			}

			// Store principal in context
			ctx = context.WithValue(ctx, principalContextKey{}, principal)

			// Remove the Authorization header to prevent downstream access to raw key
			req := c.Request().Clone(ctx)
			req.Header.Del("Authorization")
			c.SetRequest(req)

			// Touch last used asynchronously (fire and forget)
			go func(keyID uuid.UUID) {
				// Use a background context with timeout for the async operation
				touchCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = repo.TouchLastUsed(touchCtx, keyID)
			}(key.ID)

			return next(c)
		}
	}
}

// logAuthFailure logs an authentication failure event to the audit service.
func logAuthFailure(c echo.Context, auditSvc service.AuditService, profileID *string, reason, message string) {
	if auditSvc == nil {
		return
	}

	clientIP := c.RealIP()
	correlationID := GetCorrelationID(c.Request().Context())

	metadata := map[string]interface{}{
		"reason":  reason,
		"message": message,
	}

	// Use a background context with timeout for logging
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = auditSvc.AuthFailure(ctx, profileID, "api_key", "", metadata, clientIP, correlationID)
}

// GetPrincipal retrieves the authenticated principal from the context.
// Returns nil if no principal is found.
func GetPrincipal(ctx context.Context) *Principal {
	if p, ok := ctx.Value(principalContextKey{}).(*Principal); ok {
		return p
	}
	return nil
}

// GetPrincipalInterface retrieves the authenticated principal as the interface type.
//
// Test helpers for injecting Principals live under the build tag `testhelpers`
// in auth_testhelpers.go; production code has no way to construct a Principal
// context.
// This is useful for consumers that want to depend on the interface rather than concrete type.
func GetPrincipalInterface(ctx context.Context) PrincipalInterface {
	return GetPrincipal(ctx)
}

// RequirePrincipal is a helper that returns the principal or an error if not authenticated.
// This is useful for handlers that require authentication.
func RequirePrincipal(ctx context.Context) (*Principal, error) {
	p := GetPrincipal(ctx)
	if p == nil {
		return nil, httperr.New(httperr.AUTH_MISSING, "authentication required")
	}
	return p, nil
}

// RequireAuth is middleware that ensures a principal is present in the context.
// It should be used after AuthMiddleware to enforce authentication on specific routes.
func RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if GetPrincipal(c.Request().Context()) == nil {
				return httperr.New(httperr.AUTH_MISSING, "authentication required")
			}
			return next(c)
		}
	}
}

// Ensure the imports are used
var _ = domain.APIKey{}
var _ = http.StatusOK

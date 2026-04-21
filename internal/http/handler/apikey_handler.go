package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/domain"
	dto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/http/response"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service"
)

// APIKeyServiceInterface defines the interface for API key service operations.
// This allows mocking in tests and decouples the handler from concrete implementations.
type APIKeyServiceInterface interface {
	CreateStandardKey(ctx context.Context, profileID uuid.UUID, req service.CreateAPIKeyRequest, actorKeyID *string, actorRole, clientIP, correlationID string) (*domain.APIKey, string, error)
	ListByProfile(ctx context.Context, profileID uuid.UUID, limit, offset int) ([]*domain.APIKey, error)
	CountByProfile(ctx context.Context, profileID uuid.UUID) (int64, error)
	GetByIDForProfile(ctx context.Context, profileID, id uuid.UUID) (*domain.APIKey, error)
	RevokeForProfile(ctx context.Context, profileID, id uuid.UUID, actorKeyID *string, actorRole, clientIP, correlationID string) error
}

// APIKeyHandler handles HTTP requests for API key operations.
type APIKeyHandler struct {
	svc APIKeyServiceInterface
}

// APIKeyHandlerInterface is the companion interface for APIKeyHandler.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type APIKeyHandlerInterface interface {
	Create(c echo.Context) error
	List(c echo.Context) error
	Get(c echo.Context) error
	Delete(c echo.Context) error
}

// Ensure APIKeyHandler implements APIKeyHandlerInterface
var _ APIKeyHandlerInterface = (*APIKeyHandler)(nil)

// NewAPIKeyHandler creates a new API key handler with the given service.
func NewAPIKeyHandler(svc APIKeyServiceInterface) *APIKeyHandler {
	return &APIKeyHandler{svc: svc}
}

// CreateAPIKeyResponse is the response for creating an API key.
// It includes the plaintext key exactly once.
type CreateAPIKeyResponse struct {
	APIKey string             `json:"api_key"`
	Key    dto.APIKeyResponse `json:"key"`
}

// Create handles POST /api/v1/profiles/:profileId/api-keys.
// Requires 'write' scope. Admin bypasses scope check.
// Returns 201 with the created key and plaintext api_key.
func (h *APIKeyHandler) Create(c echo.Context) error {
	ctx := c.Request().Context()
	principal := middleware.GetPrincipal(ctx)

	// Parse profile ID from path
	profileIDStr := c.Param("profileId")
	if profileIDStr == "" {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	// Validate UUID format
	profileID, err := uuid.Parse(profileIDStr)
	if err != nil {
		return httperr.New(httperr.INVALID_UUID, "invalid profile ID format")
	}

	// Get validated request body
	body, ok := middleware.GetValidatedBody[dto.CreateAPIKeyRequest](ctx, middleware.CreateAPIKeyBodyKey)
	if !ok {
		return httperr.New(httperr.VALIDATION_ERROR, "request body not found")
	}

	// Build service request
	req := service.CreateAPIKeyRequest{
		Label:     body.Label,
		Scopes:    body.Scopes,
		RateLimit: body.RateLimit,
	}
	if body.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err == nil {
			req.ExpiresAt = &t
		}
	}

	// Get actor metadata from principal
	var actorKeyID *string
	actorRole := "standard"
	if principal != nil {
		keyIDStr := principal.KeyID.String()
		actorKeyID = &keyIDStr
		actorRole = principal.Role
	}

	// Create API key
	key, rawKey, err := h.svc.CreateStandardKey(ctx, profileID, req, actorKeyID, actorRole, c.RealIP(), middleware.GetCorrelationID(ctx))
	if err != nil {
		return err
	}

	// Return 201 with key data and plaintext
	return response.SuccessCreated(c, CreateAPIKeyResponse{
		APIKey: rawKey,
		Key:    toAPIKeyResponse(key),
	})
}

// List handles GET /api/v1/profiles/:profileId/api-keys.
// Requires 'read' scope. Admin bypasses scope check.
// Returns 200 with paginated list of API keys (never includes key_hash or plaintext).
func (h *APIKeyHandler) List(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse profile ID from path
	profileIDStr := c.Param("profileId")
	if profileIDStr == "" {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	// Validate UUID format
	profileID, err := uuid.Parse(profileIDStr)
	if err != nil {
		return httperr.New(httperr.INVALID_UUID, "invalid profile ID format")
	}

	// Parse pagination params
	limit, offset := parsePaginationParams(c)

	// Get API keys
	keys, err := h.svc.ListByProfile(ctx, profileID, limit, offset)
	if err != nil {
		return err
	}

	// Count total keys for the profile (real total, not just this page).
	total, err := h.svc.CountByProfile(ctx, profileID)
	if err != nil {
		return err
	}

	// Convert to response format (never includes key_hash or plaintext)
	data := make([]dto.APIKeyListItem, len(keys))
	for i, k := range keys {
		data[i] = toAPIKeyListItem(k)
	}

	// Return 200 with pagination envelope
	return c.JSON(http.StatusOK, PaginationEnvelope{
		Data: data,
		Pagination: Pagination{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	})
}

// Get handles GET /api/v1/profiles/:profileId/api-keys/:keyId.
// Requires 'read' scope. Admin bypasses scope check.
// Returns 200 with the API key data (never includes key_hash or plaintext).
// Scoped to the profileId in the path — returns NOT_FOUND for cross-profile ids.
func (h *APIKeyHandler) Get(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse profile ID from path
	profileIDStr := c.Param("profileId")
	if profileIDStr == "" {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	// Validate profile UUID format
	profileID, err := uuid.Parse(profileIDStr)
	if err != nil {
		return httperr.New(httperr.INVALID_UUID, "invalid profile ID format")
	}

	// Parse key ID from path
	keyIDStr := c.Param("keyId")
	if keyIDStr == "" {
		return httperr.New(httperr.NOT_FOUND, "key ID is required")
	}

	// Validate key UUID format
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		return httperr.New(httperr.INVALID_UUID, "invalid key ID format")
	}

	// Get API key scoped to profile — returns NOT_FOUND on cross-profile id.
	key, err := h.svc.GetByIDForProfile(ctx, profileID, keyID)
	if err != nil {
		return err
	}

	// Return 200 with key data (never includes key_hash or plaintext)
	return response.SuccessOK(c, toAPIKeyResponse(key))
}

// Delete handles DELETE /api/v1/profiles/:profileId/api-keys/:keyId.
// Requires 'write' scope. Admin bypasses scope check.
// Returns 200 with { "status": "revoked" }.
// Scoped to the profileId in the path — returns NOT_FOUND for cross-profile ids.
func (h *APIKeyHandler) Delete(c echo.Context) error {
	ctx := c.Request().Context()
	principal := middleware.GetPrincipal(ctx)

	// Parse profile ID from path
	profileIDStr := c.Param("profileId")
	if profileIDStr == "" {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	// Validate profile UUID format
	profileID, err := uuid.Parse(profileIDStr)
	if err != nil {
		return httperr.New(httperr.INVALID_UUID, "invalid profile ID format")
	}

	// Parse key ID from path
	keyIDStr := c.Param("keyId")
	if keyIDStr == "" {
		return httperr.New(httperr.NOT_FOUND, "key ID is required")
	}

	// Validate key UUID format
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		return httperr.New(httperr.INVALID_UUID, "invalid key ID format")
	}

	// Get actor metadata from principal
	var actorKeyID *string
	actorRole := "standard"
	if principal != nil {
		keyIDStr := principal.KeyID.String()
		actorKeyID = &keyIDStr
		actorRole = principal.Role
	}

	// Revoke the key scoped to profile — NOT_FOUND on cross-profile id.
	err = h.svc.RevokeForProfile(ctx, profileID, keyID, actorKeyID, actorRole, c.RealIP(), middleware.GetCorrelationID(ctx))
	if err != nil {
		return err
	}

	// Return 200 with status revoked
	return response.SuccessOK(c, map[string]string{"status": "revoked"})
}

// toAPIKeyResponse converts a domain.APIKey to dto.APIKeyResponse.
// Never includes key_hash or plaintext.
func toAPIKeyResponse(k *domain.APIKey) dto.APIKeyResponse {
	var lastUsedAt *string
	if k.LastUsedAt != nil {
		formatted := k.LastUsedAt.Format("2006-01-02T15:04:05Z")
		lastUsedAt = &formatted
	}

	var expiresAt *string
	if k.ExpiresAt != nil {
		formatted := k.ExpiresAt.Format("2006-01-02T15:04:05Z")
		expiresAt = &formatted
	}

	var revokedAt *string
	if k.RevokedAt != nil {
		formatted := k.RevokedAt.Format("2006-01-02T15:04:05Z")
		revokedAt = &formatted
	}

	return dto.APIKeyResponse{
		ID:         k.ID,
		ProfileID:  k.ProfileID,
		Label:      k.Label,
		Scopes:     k.Scopes,
		RateLimit:  k.RateLimit,
		LastUsedAt: lastUsedAt,
		ExpiresAt:  expiresAt,
		CreatedAt:  k.CreatedAt.Format("2006-01-02T15:04:05Z"),
		RevokedAt:  revokedAt,
	}
}

// toAPIKeyListItem converts a domain.APIKey to dto.APIKeyListItem.
// Never includes key_hash or plaintext.
func toAPIKeyListItem(k *domain.APIKey) dto.APIKeyListItem {
	var lastUsedAt *string
	if k.LastUsedAt != nil {
		formatted := k.LastUsedAt.Format("2006-01-02T15:04:05Z")
		lastUsedAt = &formatted
	}

	var expiresAt *string
	if k.ExpiresAt != nil {
		formatted := k.ExpiresAt.Format("2006-01-02T15:04:05Z")
		expiresAt = &formatted
	}

	var revokedAt *string
	if k.RevokedAt != nil {
		formatted := k.RevokedAt.Format("2006-01-02T15:04:05Z")
		revokedAt = &formatted
	}

	return dto.APIKeyListItem{
		ID:         k.ID,
		Label:      k.Label,
		Scopes:     k.Scopes,
		RateLimit:  k.RateLimit,
		LastUsedAt: lastUsedAt,
		ExpiresAt:  expiresAt,
		CreatedAt:  k.CreatedAt.Format("2006-01-02T15:04:05Z"),
		RevokedAt:  revokedAt,
	}
}

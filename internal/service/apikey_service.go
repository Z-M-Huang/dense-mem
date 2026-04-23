package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/dense-mem/dense-mem/internal/crypto"
	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/repository"
)

// CreateAPIKeyRequest represents a request to create a new API key.
// This is imported from dto package but redeclared here for convenience.
type CreateAPIKeyRequest struct {
	Label     string     `json:"label" validate:"required,min=1,max=100,notblank"`
	Scopes    []string   `json:"scopes"`
	RateLimit int        `json:"rate_limit"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// KeySessionInvalidator is an interface for invalidating key sessions.
// Unit 22 will implement this with Redis cleanup.
type KeySessionInvalidator interface {
	InvalidateKeySessions(ctx context.Context, profileID, keyID string) error
}

// ProfileStatePurger is an interface for purging profile state.
// Unit 22 will implement this with Redis cleanup.
type ProfileStatePurger interface {
	PurgeProfileState(ctx context.Context, profileID string) error
}

// APIKeyService is the companion interface for API key business logic.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type APIKeyService interface {
	CreateStandardKey(ctx context.Context, profileID uuid.UUID, req CreateAPIKeyRequest, actorKeyID *string, actorRole, clientIP, correlationID string) (*domain.APIKey, string, error)
	ListByProfile(ctx context.Context, profileID uuid.UUID, limit, offset int) ([]*domain.APIKey, error)
	CountByProfile(ctx context.Context, profileID uuid.UUID) (int64, error)
	// GetByIDForProfile returns the key only when it belongs to profileID; NOT_FOUND otherwise (no existence oracle).
	GetByIDForProfile(ctx context.Context, profileID, id uuid.UUID) (*domain.APIKey, error)
	// RevokeForProfile revokes the key only when it belongs to profileID; NOT_FOUND otherwise.
	RevokeForProfile(ctx context.Context, profileID, id uuid.UUID, actorKeyID *string, actorRole, clientIP, correlationID string) error
}

// APIKeyServiceImpl implements the APIKeyService interface.
type APIKeyServiceImpl struct {
	repo               repository.APIKeyRepository
	profileService     ProfileService
	auditService       AuditService
	sessionInvalidator KeySessionInvalidator
	statePurger        ProfileStatePurger
	logger             *slog.Logger
}

// Ensure APIKeyServiceImpl implements APIKeyService
var _ APIKeyService = (*APIKeyServiceImpl)(nil)

// NewAPIKeyService creates a new API key service instance.
func NewAPIKeyService(
	repo repository.APIKeyRepository,
	profileService ProfileService,
	auditService AuditService,
	sessionInvalidator KeySessionInvalidator,
	statePurger ProfileStatePurger,
) *APIKeyServiceImpl {
	return &APIKeyServiceImpl{
		repo:               repo,
		profileService:     profileService,
		auditService:       auditService,
		sessionInvalidator: sessionInvalidator,
		statePurger:        statePurger,
		logger:             slog.Default(),
	}
}

// NewAPIKeyServiceWithLogger creates a new API key service instance with a custom logger.
func NewAPIKeyServiceWithLogger(
	repo repository.APIKeyRepository,
	profileService ProfileService,
	auditService AuditService,
	sessionInvalidator KeySessionInvalidator,
	statePurger ProfileStatePurger,
	logger *slog.Logger,
) *APIKeyServiceImpl {
	return &APIKeyServiceImpl{
		repo:               repo,
		profileService:     profileService,
		auditService:       auditService,
		sessionInvalidator: sessionInvalidator,
		statePurger:        statePurger,
		logger:             logger,
	}
}

// logAuditError logs an audit service error with structured logging.
func (s *APIKeyServiceImpl) logAuditError(err error, operation, keyID, correlationID string) {
	if s.logger == nil {
		return
	}
	s.logger.Error("audit_log_write_failed",
		slog.String("error", err.Error()),
		slog.String("operation", operation),
		slog.String("key_id", keyID),
		slog.String("correlation_id", correlationID),
	)
}

// CreateStandardKey creates a new standard API key for a profile.
// Returns the created key and the plaintext raw key (returned exactly once).
func (s *APIKeyServiceImpl) CreateStandardKey(ctx context.Context, profileID uuid.UUID, req CreateAPIKeyRequest, actorKeyID *string, actorRole, clientIP, correlationID string) (*domain.APIKey, string, error) {
	// Verify profile exists
	_, err := s.profileService.Get(ctx, profileID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to verify profile: %w", err)
	}

	// Generate raw key
	rawKey, err := crypto.GenerateRawKey()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate raw key: %w", err)
	}

	// Hash the key
	keyHash, err := crypto.HashKey(rawKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to hash key: %w", err)
	}

	// Create the key record
	key := &domain.APIKey{
		ProfileID: profileID,
		Label:     req.Label,
		KeyHash:   keyHash,
		KeyPrefix: crypto.GetKeyPrefix(rawKey),
		Scopes:    req.Scopes,
		RateLimit: req.RateLimit,
		ExpiresAt: req.ExpiresAt,
	}

	if err := s.repo.CreateStandardKey(ctx, key); err != nil {
		return nil, "", fmt.Errorf("failed to create api key: %w", err)
	}

	// Audit the creation (without the raw key or hash)
	afterPayload := map[string]interface{}{
		"id":         key.ID.String(),
		"profile_id": key.ProfileID.String(),
		"label":      key.Label,
		"scopes":     key.Scopes,
		"rate_limit": key.RateLimit,
		"role":       "standard",
	}
	if key.ExpiresAt != nil {
		afterPayload["expires_at"] = key.ExpiresAt.Format(time.RFC3339)
	}

	profileIDStr := profileID.String()
	if err := s.auditService.APIKeyCreated(ctx, &profileIDStr, key.ID.String(), afterPayload, actorKeyID, actorRole, clientIP, correlationID); err != nil {
		// Log the audit failure but don't fail the operation
		s.logAuditError(err, "CREATE", key.ID.String(), correlationID)
	}

	// Return the key (without hash) and the raw key (shown exactly once)
	key.KeyHash = ""
	return key, rawKey, nil
}

// ListByProfile retrieves API keys for a profile.
// Never returns the key_hash field.
func (s *APIKeyServiceImpl) ListByProfile(ctx context.Context, profileID uuid.UUID, limit, offset int) ([]*domain.APIKey, error) {
	return s.repo.ListByProfile(ctx, profileID, limit, offset)
}

// CountByProfile returns the total number of API keys for a profile (used for pagination totals).
func (s *APIKeyServiceImpl) CountByProfile(ctx context.Context, profileID uuid.UUID) (int64, error) {
	return s.repo.CountByProfile(ctx, profileID)
}

// GetByIDForProfile retrieves an API key by ID scoped to the caller's profile.
// Returns NOT_FOUND when the id/profile combination does not match (prevents existence oracle).
func (s *APIKeyServiceImpl) GetByIDForProfile(ctx context.Context, profileID, id uuid.UUID) (*domain.APIKey, error) {
	key, err := s.repo.GetByIDForProfile(ctx, profileID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get api key for profile: %w", err)
	}
	if key == nil {
		return nil, httperr.New(httperr.NOT_FOUND, fmt.Sprintf("api key with id '%s' not found", id.String()))
	}
	return key, nil
}

// RevokeForProfile revokes an API key scoped to the caller's profile.
// Returns NOT_FOUND when the id/profile combination does not match.
// Invalidates active sessions and writes audit.
func (s *APIKeyServiceImpl) RevokeForProfile(ctx context.Context, profileID, id uuid.UUID, actorKeyID *string, actorRole, clientIP, correlationID string) error {
	// Fetch under profile scope for audit before-payload and to verify ownership.
	key, err := s.repo.GetByIDForProfile(ctx, profileID, id)
	if err != nil {
		return fmt.Errorf("failed to get api key for profile: %w", err)
	}
	if key == nil {
		return httperr.New(httperr.NOT_FOUND, fmt.Sprintf("api key with id '%s' not found", id.String()))
	}
	if key.RevokedAt != nil {
		return httperr.New(httperr.CONFLICT, "api key is already revoked")
	}

	beforePayload := map[string]interface{}{
		"id":         key.ID.String(),
		"profile_id": key.ProfileID.String(),
		"label":      key.Label,
		"scopes":     key.Scopes,
		"rate_limit": key.RateLimit,
		"revoked_at": nil,
	}

	rows, err := s.repo.RevokeForProfile(ctx, profileID, id)
	if err != nil {
		return fmt.Errorf("failed to revoke api key for profile: %w", err)
	}
	if rows == 0 {
		// Race: key was revoked or reassigned between check and update.
		return httperr.New(httperr.NOT_FOUND, fmt.Sprintf("api key with id '%s' not found", id.String()))
	}

	if s.sessionInvalidator != nil {
		if err := s.sessionInvalidator.InvalidateKeySessions(ctx, profileID.String(), id.String()); err != nil {
			s.logger.Warn("session_invalidation_failed", slog.String("error", err.Error()), slog.String("key_id", id.String()))
		}
	}

	profileIDStr := profileID.String()
	if err := s.auditService.APIKeyRevoked(ctx, &profileIDStr, key.ID.String(), beforePayload, actorKeyID, actorRole, clientIP, correlationID); err != nil {
		s.logAuditError(err, "REVOKE", key.ID.String(), correlationID)
	}

	return nil
}

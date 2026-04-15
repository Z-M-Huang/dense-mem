package dto

import (
	"github.com/google/uuid"
)

// CreateAPIKeyRequest represents a request to create a new API key.
// Validation rules:
//   - Label: 1-100 characters, not blank
//   - Scopes: optional array of permission scopes
//   - RateLimit: optional rate limit per minute
//   - ExpiresAt: optional expiration time
type CreateAPIKeyRequest struct {
	Label     string     `json:"label" validate:"required,min=1,max=100,notblank"`
	Scopes    []string   `json:"scopes"`
	RateLimit int        `json:"rate_limit"`
	ExpiresAt *string    `json:"expires_at"`
}

// APIKeyResponse represents an API key in API responses.
type APIKeyResponse struct {
	ID         uuid.UUID `json:"id"`
	ProfileID  uuid.UUID `json:"profile_id"`
	Label      string    `json:"label"`
	Scopes     []string  `json:"scopes"`
	RateLimit  int       `json:"rate_limit"`
	LastUsedAt *string   `json:"last_used_at"`
	ExpiresAt  *string   `json:"expires_at"`
	CreatedAt  string    `json:"created_at"`
	RevokedAt  *string   `json:"revoked_at"`
}

// APIKeyListItem represents a single API key in a list response.
type APIKeyListItem struct {
	ID         uuid.UUID `json:"id"`
	Label      string    `json:"label"`
	Scopes     []string  `json:"scopes"`
	RateLimit  int       `json:"rate_limit"`
	LastUsedAt *string   `json:"last_used_at"`
	ExpiresAt  *string   `json:"expires_at"`
	CreatedAt  string    `json:"created_at"`
	RevokedAt  *string   `json:"revoked_at"`
}

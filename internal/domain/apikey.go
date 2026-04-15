package domain

import (
	"time"

	"github.com/google/uuid"
)

// APIKeyModel is the companion interface for APIKey.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type APIKeyModel interface {
	GetID() uuid.UUID
	GetProfileID() uuid.UUID
	GetLabel() string
	GetKeyHash() string
	GetKeyPrefix() string
	GetScopes() []string
	GetRateLimit() int
	GetLastUsedAt() *time.Time
	GetExpiresAt() *time.Time
	GetCreatedAt() time.Time
	GetRevokedAt() *time.Time
}

// APIKey represents an API key for authentication.
type APIKey struct {
	ID         uuid.UUID
	ProfileID  uuid.UUID
	Label      string
	KeyHash    string
	KeyPrefix  string
	Scopes     []string
	RateLimit  int
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

// Ensure APIKey implements APIKeyModel
var _ APIKeyModel = (*APIKey)(nil)

// Getters for APIKeyModel interface
func (k *APIKey) GetID() uuid.UUID         { return k.ID }
func (k *APIKey) GetProfileID() uuid.UUID  { return k.ProfileID }
func (k *APIKey) GetLabel() string         { return k.Label }
func (k *APIKey) GetKeyHash() string       { return k.KeyHash }
func (k *APIKey) GetKeyPrefix() string     { return k.KeyPrefix }
func (k *APIKey) GetScopes() []string      { return k.Scopes }
func (k *APIKey) GetRateLimit() int        { return k.RateLimit }
func (k *APIKey) GetLastUsedAt() *time.Time { return k.LastUsedAt }
func (k *APIKey) GetExpiresAt() *time.Time  { return k.ExpiresAt }
func (k *APIKey) GetCreatedAt() time.Time   { return k.CreatedAt }
func (k *APIKey) GetRevokedAt() *time.Time  { return k.RevokedAt }

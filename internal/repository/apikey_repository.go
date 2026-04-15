package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/storage/postgres"
)

// APIKeyRepository is the companion interface for API key data access.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type APIKeyRepository interface {
	CreateStandardKey(ctx context.Context, key *domain.APIKey) error
	CreateAdminKey(ctx context.Context, key *domain.APIKey) error
	ListByProfile(ctx context.Context, profileID uuid.UUID, limit, offset int) ([]*domain.APIKey, error)
	CountByProfile(ctx context.Context, profileID uuid.UUID) (int64, error)
	// GetByID returns an API key by id without a profile scope — use ONLY for admin paths.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.APIKey, error)
	// GetByIDForProfile returns an API key only when it belongs to profileID. Returns nil on mismatch.
	GetByIDForProfile(ctx context.Context, profileID, id uuid.UUID) (*domain.APIKey, error)
	GetActiveByPrefix(ctx context.Context, prefix string) (*domain.APIKey, error)
	// Revoke marks a key revoked without profile scope — use ONLY for admin paths.
	Revoke(ctx context.Context, id uuid.UUID) error
	// RevokeForProfile marks a key revoked only when it belongs to profileID. Returns number of rows affected.
	RevokeForProfile(ctx context.Context, profileID, id uuid.UUID) (int64, error)
	TouchLastUsed(ctx context.Context, id uuid.UUID) error
	AdminKeyExists(ctx context.Context) (bool, error)
}

// APIKeyRepositoryImpl implements the APIKeyRepository interface.
// Every query runs inside an RLS-aware transaction so Postgres FORCE RLS
// policies (app.current_profile_id / app.role) enforce tenant isolation
// even if a caller ever reaches the repository without the service layer.
type APIKeyRepositoryImpl struct {
	db  *gorm.DB
	rls postgres.RLSHelper
}

// Ensure APIKeyRepositoryImpl implements APIKeyRepository
var _ APIKeyRepository = (*APIKeyRepositoryImpl)(nil)

// NewAPIKeyRepository creates a new API key repository instance.
// rls is required; nil causes a panic at first use. Callers should pass
// postgres.NewRLS() for production and an RLSHelper mock for unit tests.
func NewAPIKeyRepository(db *gorm.DB, rls postgres.RLSHelper) *APIKeyRepositoryImpl {
	return &APIKeyRepositoryImpl{db: db, rls: rls}
}

// CreateStandardKey creates a new standard API key associated with a profile.
func (r *APIKeyRepositoryImpl) CreateStandardKey(ctx context.Context, key *domain.APIKey) error {
	if key.ID == uuid.Nil {
		key.ID = uuid.New()
	}

	now := time.Now().UTC()
	key.CreatedAt = now

	// Standard keys must have a profile_id
	// Use the KeyPrefix field from the domain object (derived from raw key)
	keyPrefix := key.KeyPrefix
	if keyPrefix == "" {
		// Fallback: derive from hash (incorrect but legacy support)
		keyPrefix = GetKeyPrefixFromHash(key.KeyHash)
	}

	// INSERT must satisfy api_keys_self_access (profile_id = app.current_profile_id);
	// set the session to the owning profile so the RLS WITH CHECK passes.
	// Scopes must be wrapped in pq.Array — the pgx driver (via gorm.io/driver/postgres)
	// does not encode a naked []string as Postgres text[]; it writes NULL and the
	// authorization layer later sees an empty scope set.
	err := r.rls.WithProfileTx(ctx, r.db, key.ProfileID.String(), "standard", func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO api_keys (id, profile_id, key_hash, key_prefix, label, role, scopes, rate_limit, expires_at, revoked_at, last_used_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'standard', $6, $7, $8, NULL, NULL, $9, $9)
		`, key.ID, key.ProfileID, key.KeyHash, keyPrefix, key.Label, pq.Array(key.Scopes), key.RateLimit, key.ExpiresAt, now).Error
	})

	if err != nil {
		return fmt.Errorf("failed to create standard api key: %w", err)
	}

	return nil
}

// CreateAdminKey creates a new admin API key (no profile association).
func (r *APIKeyRepositoryImpl) CreateAdminKey(ctx context.Context, key *domain.APIKey) error {
	if key.ID == uuid.Nil {
		key.ID = uuid.New()
	}

	now := time.Now().UTC()
	key.CreatedAt = now

	// Admin keys must NOT have a profile_id
	// Use the KeyPrefix field from the domain object (derived from raw key)
	keyPrefix := key.KeyPrefix
	if keyPrefix == "" {
		// Fallback: derive from hash (incorrect but legacy support)
		keyPrefix = GetKeyPrefixFromHash(key.KeyHash)
	}

	// Admin key creation has profile_id=NULL; we run under admin RLS context
	// because this is bootstrap/operator territory. No RLS policy explicitly
	// allows INSERT with NULL profile_id — admin bootstrap therefore requires
	// a DB role with BYPASSRLS or a policy extension. The wiring here makes
	// the intent explicit; any enforcement gap is a schema concern, not code.
	err := r.rls.WithAdminTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO api_keys (id, profile_id, key_hash, key_prefix, label, role, scopes, rate_limit, expires_at, revoked_at, last_used_at, created_at, updated_at)
			VALUES ($1, NULL, $2, $3, $4, 'admin', $5, $6, $7, NULL, NULL, $8, $8)
		`, key.ID, key.KeyHash, keyPrefix, key.Label, pq.Array(key.Scopes), key.RateLimit, key.ExpiresAt, now).Error
	})

	if err != nil {
		return fmt.Errorf("failed to create admin api key: %w", err)
	}

	return nil
}

// GetKeyPrefixFromHash extracts a prefix placeholder from the key hash.
// In practice, the key_prefix should be passed separately, but this helper
// extracts the first 12 chars of the hash as a fallback.
func GetKeyPrefixFromHash(hash string) string {
	if len(hash) < 12 {
		return hash
	}
	return hash[:12]
}

// ListByProfile retrieves API keys for a profile with pagination.
// Excludes the key_hash field from results.
//
// Uses *sql.Rows + pq.Array() — see GetActiveByPrefix for the rationale.
func (r *APIKeyRepositoryImpl) ListByProfile(ctx context.Context, profileID uuid.UUID, limit, offset int) ([]*domain.APIKey, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	keys := make([]*domain.APIKey, 0)
	err := r.rls.WithProfileTx(ctx, r.db, profileID.String(), "standard", func(tx *gorm.DB) error {
		rows, rerr := tx.Raw(`
			SELECT id, profile_id, label, scopes, rate_limit, last_used_at, expires_at, created_at, revoked_at
			FROM api_keys
			WHERE profile_id = $1
			ORDER BY created_at DESC, id ASC
			LIMIT $2 OFFSET $3
		`, profileID, limit, offset).Rows()
		if rerr != nil {
			return rerr
		}
		defer rows.Close()

		for rows.Next() {
			var k domain.APIKey
			if serr := rows.Scan(
				&k.ID,
				&k.ProfileID,
				&k.Label,
				pq.Array(&k.Scopes),
				&k.RateLimit,
				&k.LastUsedAt,
				&k.ExpiresAt,
				&k.CreatedAt,
				&k.RevokedAt,
			); serr != nil {
				return serr
			}
			keys = append(keys, &k)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list api keys: %w", err)
	}
	return keys, nil
}

// GetByID retrieves an API key by ID.
// Excludes the key_hash field from results.
//
// Uses *sql.Rows + pq.Array() — see GetActiveByPrefix for the rationale.
func (r *APIKeyRepositoryImpl) GetByID(ctx context.Context, id uuid.UUID) (*domain.APIKey, error) {
	var key domain.APIKey
	var profileID *uuid.UUID
	found := false

	err := r.rls.WithAdminTx(ctx, r.db, func(tx *gorm.DB) error {
		rows, rerr := tx.Raw(`
			SELECT id, profile_id, label, scopes, rate_limit, last_used_at, expires_at, created_at, revoked_at
			FROM api_keys
			WHERE id = $1
		`, id).Rows()
		if rerr != nil {
			return rerr
		}
		defer rows.Close()

		if rows.Next() {
			found = true
			return rows.Scan(
				&key.ID,
				&profileID,
				&key.Label,
				pq.Array(&key.Scopes),
				&key.RateLimit,
				&key.LastUsedAt,
				&key.ExpiresAt,
				&key.CreatedAt,
				&key.RevokedAt,
			)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get api key: %w", err)
	}
	if !found {
		return nil, nil
	}
	if profileID != nil {
		key.ProfileID = *profileID
	}
	return &key, nil
}

// GetActiveByPrefix retrieves an active (non-revoked, non-expired) API key by its prefix.
// This is used during authentication to look up the key hash for verification.
// Includes the key_hash field for verification purposes.
//
// Uses *sql.Rows + pq.Array() rather than GORM .Scan() because the pgx driver
// (via gorm.io/driver/postgres) does not route text[] values through lib/pq's
// StringArray scanner when GORM copies columns by reflection; scopes come back
// empty and authorization fails closed.
func (r *APIKeyRepositoryImpl) GetActiveByPrefix(ctx context.Context, prefix string) (*domain.APIKey, error) {
	var key domain.APIKey
	var profileID *uuid.UUID
	found := false

	err := r.rls.WithAdminTx(ctx, r.db, func(tx *gorm.DB) error {
		rows, rerr := tx.Raw(`
			SELECT id, profile_id, key_hash, label, scopes, rate_limit, last_used_at, expires_at, created_at, revoked_at
			FROM api_keys
			WHERE key_prefix = $1
				AND revoked_at IS NULL
				AND (expires_at IS NULL OR expires_at > NOW())
		`, prefix).Rows()
		if rerr != nil {
			return rerr
		}
		defer rows.Close()

		if rows.Next() {
			found = true
			return rows.Scan(
				&key.ID,
				&profileID,
				&key.KeyHash,
				&key.Label,
				pq.Array(&key.Scopes),
				&key.RateLimit,
				&key.LastUsedAt,
				&key.ExpiresAt,
				&key.CreatedAt,
				&key.RevokedAt,
			)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get api key by prefix: %w", err)
	}
	if !found {
		return nil, nil
	}
	if profileID != nil {
		key.ProfileID = *profileID
	}
	return &key, nil
}

// Revoke marks an API key as revoked WITHOUT profile scope — reserved for admin paths.
// Standard-role callers MUST use RevokeForProfile to prevent cross-profile revocation.
func (r *APIKeyRepositoryImpl) Revoke(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()

	// Unscoped admin-only revoke; runs in admin RLS context. Standard callers
	// must use RevokeForProfile instead (enforced at the service layer).
	err := r.rls.WithAdminTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Exec(`
			UPDATE api_keys
			SET revoked_at = $1, updated_at = $1
			WHERE id = $2 AND revoked_at IS NULL
		`, now, id).Error
	})

	if err != nil {
		return fmt.Errorf("failed to revoke api key: %w", err)
	}

	return nil
}

// RevokeForProfile marks an API key as revoked only when it belongs to profileID.
// Returns the number of rows affected (0 means the id/profile combination did not match).
func (r *APIKeyRepositoryImpl) RevokeForProfile(ctx context.Context, profileID, id uuid.UUID) (int64, error) {
	now := time.Now().UTC()

	// Profile-scoped revoke; UPDATE must satisfy api_keys_self_access.
	var rowsAffected int64
	err := r.rls.WithProfileTx(ctx, r.db, profileID.String(), "standard", func(tx *gorm.DB) error {
		res := tx.Exec(`
			UPDATE api_keys
			SET revoked_at = $1, updated_at = $1
			WHERE id = $2 AND profile_id = $3 AND revoked_at IS NULL
		`, now, id, profileID)
		if res.Error != nil {
			return res.Error
		}
		rowsAffected = res.RowsAffected
		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("failed to revoke api key for profile: %w", err)
	}

	return rowsAffected, nil
}

// GetByIDForProfile retrieves an API key by ID only when it belongs to profileID.
// Returns nil when the id/profile combination does not match (prevents existence oracle).
// Excludes the key_hash field from results.
//
// Uses *sql.Rows + pq.Array() — see GetActiveByPrefix for the rationale.
func (r *APIKeyRepositoryImpl) GetByIDForProfile(ctx context.Context, profileID, id uuid.UUID) (*domain.APIKey, error) {
	var key domain.APIKey
	var rowProfileID *uuid.UUID
	found := false

	err := r.rls.WithProfileTx(ctx, r.db, profileID.String(), "standard", func(tx *gorm.DB) error {
		rows, rerr := tx.Raw(`
			SELECT id, profile_id, label, scopes, rate_limit, last_used_at, expires_at, created_at, revoked_at
			FROM api_keys
			WHERE id = $1 AND profile_id = $2
		`, id, profileID).Rows()
		if rerr != nil {
			return rerr
		}
		defer rows.Close()

		if rows.Next() {
			found = true
			return rows.Scan(
				&key.ID,
				&rowProfileID,
				&key.Label,
				pq.Array(&key.Scopes),
				&key.RateLimit,
				&key.LastUsedAt,
				&key.ExpiresAt,
				&key.CreatedAt,
				&key.RevokedAt,
			)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get api key for profile: %w", err)
	}
	if !found {
		return nil, nil
	}
	if rowProfileID != nil {
		key.ProfileID = *rowProfileID
	}
	return &key, nil
}

// CountByProfile returns the total number of API keys for a profile.
// Used to populate pagination totals without a second full-result scan.
func (r *APIKeyRepositoryImpl) CountByProfile(ctx context.Context, profileID uuid.UUID) (int64, error) {
	var count int64
	err := r.rls.WithProfileTx(ctx, r.db, profileID.String(), "standard", func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT COUNT(*) FROM api_keys WHERE profile_id = $1
		`, profileID).Scan(&count).Error
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count api keys for profile: %w", err)
	}
	return count, nil
}

// TouchLastUsed updates the last_used_at timestamp for an API key.
// This should be called asynchronously to avoid blocking the request.
func (r *APIKeyRepositoryImpl) TouchLastUsed(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()

	// Auth-path update: callers don't have profile context (admin keys have
	// profile_id=NULL and standard keys were just authenticated). Admin RLS
	// context keeps auth hot-path writes consistent across both key roles.
	err := r.rls.WithAdminTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Exec(`
			UPDATE api_keys
			SET last_used_at = $1
			WHERE id = $2
		`, now, id).Error
	})

	if err != nil {
		return fmt.Errorf("failed to touch last used: %w", err)
	}

	return nil
}

// AdminKeyExists checks if any admin key exists in the database.
// This is used to ensure idempotent bootstrap admin key creation.
func (r *APIKeyRepositoryImpl) AdminKeyExists(ctx context.Context) (bool, error) {
	var count int64

	// Bootstrap check: counts admin keys across the table; requires admin RLS
	// context to see rows with profile_id=NULL.
	err := r.rls.WithAdminTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT COUNT(*)
			FROM api_keys
			WHERE role = 'admin'
		`).Scan(&count).Error
	})

	if err != nil {
		return false, fmt.Errorf("failed to check admin key existence: %w", err)
	}

	return count > 0, nil
}

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/storage/postgres"
)

// emptyMetadata is an empty JSON object for postgres jsonb columns
var emptyMetadata = map[string]any{}

// ProfileRepository is the companion interface for profile data access.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type ProfileRepository interface {
	Create(ctx context.Context, profile *domain.Profile) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Profile, error)
	List(ctx context.Context, limit, offset int) ([]*domain.Profile, error)
	Count(ctx context.Context) (int64, error)
	Update(ctx context.Context, profile *domain.Profile) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	CountActiveKeys(ctx context.Context, profileID uuid.UUID) (int64, error)
	NameExists(ctx context.Context, name string) (bool, error)
}

// ProfileRepositoryImpl implements the ProfileRepository interface.
// Every query runs inside an RLS-aware transaction so Postgres FORCE RLS
// policies (app.current_profile_id / app.role) enforce tenant isolation
// even if a caller ever reaches the repository without the service layer.
type ProfileRepositoryImpl struct {
	db  *gorm.DB
	rls postgres.RLSHelper
}

// Ensure ProfileRepositoryImpl implements ProfileRepository
var _ ProfileRepository = (*ProfileRepositoryImpl)(nil)

// NewProfileRepository creates a new profile repository instance.
// rls is required; nil causes a panic at first use. Callers should pass
// postgres.NewRLS() for production and an RLSHelper mock for unit tests.
func NewProfileRepository(db *gorm.DB, rls postgres.RLSHelper) *ProfileRepositoryImpl {
	return &ProfileRepositoryImpl{db: db, rls: rls}
}

// Create creates a new profile with server-side UUID generation.
// Enforces unique lower(name) among non-deleted rows and sets status='active'.
func (r *ProfileRepositoryImpl) Create(ctx context.Context, profile *domain.Profile) error {
	// Generate UUID server-side if not provided
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}

	// Set timestamps
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now

	// Ensure metadata and config are not nil for postgres jsonb NOT NULL columns
	metadata := profile.Metadata
	if metadata == nil {
		metadata = emptyMetadata
	}
	config := profile.Config
	if config == nil {
		config = emptyMetadata
	}

	// INSERT must satisfy profiles_self_access (id = app.current_profile_id);
	// seed the session with the new profile's id so the RLS policy passes.
	err := r.rls.WithProfileTx(ctx, r.db, profile.ID.String(), "standard", func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO profiles (id, name, description, metadata, config, status, created_at, updated_at, deleted_at)
			VALUES ($1, $2, $3, $4, $5, 'active', $6, $7, NULL)
		`, profile.ID, profile.Name, profile.Description, metadata, config, profile.CreatedAt, profile.UpdatedAt).Error
	})

	if err != nil {
		return fmt.Errorf("failed to create profile: %w", err)
	}

	return nil
}

// GetByID retrieves a profile by ID, excluding soft-deleted profiles.
// Uses admin RLS context because callers include middleware paths that
// resolve profiles without yet knowing whether the requester is authorized.
// Authorization is enforced at the HTTP middleware layer, not here.
func (r *ProfileRepositoryImpl) GetByID(ctx context.Context, id uuid.UUID) (*domain.Profile, error) {
	var profile domain.Profile

	err := r.rls.WithAdminTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT id, name, description, metadata, config, created_at, updated_at, deleted_at
			FROM profiles
			WHERE id = $1 AND deleted_at IS NULL
		`, id).Scan(&profile).Error
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}

	// Check if profile was found
	if profile.ID == uuid.Nil {
		return nil, nil
	}

	return &profile, nil
}

// List retrieves profiles with pagination, excluding soft-deleted rows.
// Default limit=20, max limit=100, sorted by created_at DESC, id ASC.
func (r *ProfileRepositoryImpl) List(ctx context.Context, limit, offset int) ([]*domain.Profile, error) {
	// Apply defaults and limits
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var profiles []*domain.Profile

	// List is an admin/cross-profile read; admin RLS context lets the
	// profiles_admin_access policy return every non-deleted row.
	err := r.rls.WithAdminTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT id, name, description, metadata, config, created_at, updated_at, deleted_at
			FROM profiles
			WHERE deleted_at IS NULL
			ORDER BY created_at DESC, id ASC
			LIMIT $1 OFFSET $2
		`, limit, offset).Scan(&profiles).Error
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list profiles: %w", err)
	}

	return profiles, nil
}

// Count returns the total number of non-deleted profiles.
func (r *ProfileRepositoryImpl) Count(ctx context.Context) (int64, error) {
	var count int64

	err := r.rls.WithAdminTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT COUNT(*)
			FROM profiles
			WHERE deleted_at IS NULL
		`).Scan(&count).Error
	})

	if err != nil {
		return 0, fmt.Errorf("failed to count profiles: %w", err)
	}

	return count, nil
}

// Update updates an existing profile, excluding soft-deleted rows.
func (r *ProfileRepositoryImpl) Update(ctx context.Context, profile *domain.Profile) error {
	profile.UpdatedAt = time.Now().UTC()

	// Ensure metadata and config are not nil for postgres jsonb NOT NULL columns
	metadata := profile.Metadata
	if metadata == nil {
		metadata = emptyMetadata
	}
	config := profile.Config
	if config == nil {
		config = emptyMetadata
	}

	// UPDATE must satisfy profiles_self_access (id = app.current_profile_id).
	err := r.rls.WithProfileTx(ctx, r.db, profile.ID.String(), "standard", func(tx *gorm.DB) error {
		return tx.Exec(`
			UPDATE profiles
			SET name = $1, description = $2, metadata = $3, config = $4, updated_at = $5
			WHERE id = $6 AND deleted_at IS NULL
		`, profile.Name, profile.Description, metadata, config, profile.UpdatedAt, profile.ID).Error
	})

	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}

	return nil
}

// SoftDelete marks a profile as deleted by setting status='deleted' and deleted_at=now().
// Does NOT check for active keys - that is the service layer's responsibility.
func (r *ProfileRepositoryImpl) SoftDelete(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()

	// Soft-delete is an UPDATE; must satisfy profiles_self_access (id = app.current_profile_id).
	err := r.rls.WithProfileTx(ctx, r.db, id.String(), "standard", func(tx *gorm.DB) error {
		return tx.Exec(`
			UPDATE profiles
			SET status = 'deleted', deleted_at = $1
			WHERE id = $2 AND deleted_at IS NULL
		`, now, id).Error
	})

	if err != nil {
		return fmt.Errorf("failed to soft delete profile: %w", err)
	}

	return nil
}

// CountActiveKeys counts the number of non-revoked, non-expired active keys for a profile.
func (r *ProfileRepositoryImpl) CountActiveKeys(ctx context.Context, profileID uuid.UUID) (int64, error) {
	var count int64

	// SELECT is scoped to one profile; api_keys_self_access matches on profile_id.
	err := r.rls.WithProfileTx(ctx, r.db, profileID.String(), "standard", func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT COUNT(*)
			FROM api_keys
			WHERE profile_id = $1
				AND revoked_at IS NULL
				AND (expires_at IS NULL OR expires_at > NOW())
		`, profileID).Scan(&count).Error
	})

	if err != nil {
		return 0, fmt.Errorf("failed to count active keys: %w", err)
	}

	return count, nil
}

// NameExists checks if a profile name exists (case-insensitive) among non-deleted rows.
func (r *ProfileRepositoryImpl) NameExists(ctx context.Context, name string) (bool, error) {
	var count int64

	// NameExists must see all profiles (collision detection is cross-tenant);
	// admin RLS context enables the profiles_admin_access SELECT policy.
	err := r.rls.WithAdminTx(ctx, r.db, func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT COUNT(*)
			FROM profiles
			WHERE lower(name) = lower($1) AND deleted_at IS NULL
		`, name).Scan(&count).Error
	})

	if err != nil {
		return false, fmt.Errorf("failed to check name existence: %w", err)
	}

	return count > 0, nil
}

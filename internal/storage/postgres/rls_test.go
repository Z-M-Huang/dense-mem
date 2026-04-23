//go:build integration
// +build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestRLSPoliciesProfileIsolation verifies profile A cannot read profile B's rows
func TestRLSPoliciesProfileIsolation(t *testing.T) {
	ctx := context.Background()

	// Create test database connection
	db, cleanup := setupTestDB(t)
	defer cleanup()

	rls := NewRLS()

	// Create two profiles
	profileA := createTestProfile(t, db, "Profile A")
	profileB := createTestProfile(t, db, "Profile B")

	// Create an API key for profile A
	apiKeyA := createTestAPIKey(t, db, profileA)

	// Create an API key for profile B
	apiKeyB := createTestAPIKey(t, db, profileB)

	// Verify profile A can only see its own API keys
	var count int64
	err := rls.WithProfileTx(ctx, db, profileA.String(), func(tx *gorm.DB) error {
		return tx.Model(&APIKey{}).Count(&count).Error
	})
	if err != nil {
		t.Fatalf("failed to query as profile A: %v", err)
	}
	if count != 1 {
		t.Errorf("profile A should see exactly 1 API key, got %d", count)
	}

	// Verify profile A cannot see profile B's API key
	var foundKey APIKey
	err = rls.WithProfileTx(ctx, db, profileA.String(), func(tx *gorm.DB) error {
		return tx.First(&foundKey, "id = ?", apiKeyB).Error
	})
	if err == nil {
		t.Error("profile A should not be able to read profile B's API key")
	}

	// Verify profile B can only see its own API keys
	err = rls.WithProfileTx(ctx, db, profileB.String(), func(tx *gorm.DB) error {
		return tx.Model(&APIKey{}).Count(&count).Error
	})
	if err != nil {
		t.Fatalf("failed to query as profile B: %v", err)
	}
	if count != 1 {
		t.Errorf("profile B should see exactly 1 API key, got %d", count)
	}

	// Verify profile B cannot see profile A's API key
	err = rls.WithProfileTx(ctx, db, profileB.String(), func(tx *gorm.DB) error {
		return tx.First(&foundKey, "id = ?", apiKeyA).Error
	})
	if err == nil {
		t.Error("profile B should not be able to read profile A's API key")
	}
}

// TestRLSPoliciesSystemSeesAll verifies internal/system transactions can read
// all rows across profiles.
func TestRLSPoliciesSystemSeesAll(t *testing.T) {
	ctx := context.Background()

	// Create test database connection
	db, cleanup := setupTestDB(t)
	defer cleanup()

	rls := NewRLS()

	// Create two profiles
	profileA := createTestProfile(t, db, "Profile A")
	profileB := createTestProfile(t, db, "Profile B")

	// Create API keys for both profiles
	createTestAPIKey(t, db, profileA)
	createTestAPIKey(t, db, profileB)

	// Verify system transactions can see all API keys.
	var count int64
	err := rls.WithSystemTx(ctx, db, func(tx *gorm.DB) error {
		return tx.Model(&APIKey{}).Count(&count).Error
	})
	if err != nil {
		t.Fatalf("failed to query as system transaction: %v", err)
	}
	if count != 2 {
		t.Errorf("system transaction should see exactly 2 API keys, got %d", count)
	}

	// Verify system transactions can see all profiles.
	err = rls.WithSystemTx(ctx, db, func(tx *gorm.DB) error {
		return tx.Model(&Profile{}).Count(&count).Error
	})
	if err != nil {
		t.Fatalf("failed to query profiles as system transaction: %v", err)
	}
	if count < 2 {
		t.Errorf("system transaction should see at least 2 profiles, got %d", count)
	}
}

// TestRLSPoliciesAuditLogAppendable verifies profile and system transactions
// can insert audit_log entries.
func TestRLSPoliciesAuditLogAppendable(t *testing.T) {
	ctx := context.Background()

	// Create test database connection
	db, cleanup := setupTestDB(t)
	defer cleanup()

	rls := NewRLS()

	// Create a profile
	profile := createTestProfile(t, db, "Test Profile")

	// Verify standard role can insert audit log
	auditID := uuid.New()
	err := rls.WithProfileTx(ctx, db, profile.String(), func(tx *gorm.DB) error {
		return tx.Create(&AuditLog{
			ID:         auditID,
			ProfileID:  &profile,
			Operation:  "test_operation",
			EntityType: "test_entity",
			EntityID:   "test-123",
			ActorRole:  "standard",
			Timestamp:  time.Now(),
		}).Error
	})
	if err != nil {
		t.Fatalf("standard role should be able to insert audit log: %v", err)
	}

	// Verify system transactions can insert audit log.
	auditID2 := uuid.New()
	err = rls.WithSystemTx(ctx, db, func(tx *gorm.DB) error {
		return tx.Create(&AuditLog{
			ID:         auditID2,
			ProfileID:  &profile,
			Operation:  "system_test_operation",
			EntityType: "test_entity",
			EntityID:   "test-456",
			ActorRole:  "system",
			Timestamp:  time.Now(),
		}).Error
	})
	if err != nil {
		t.Fatalf("system transaction should be able to insert audit log: %v", err)
	}

	// Verify system transactions can read both entries.
	var count int64
	err = rls.WithSystemTx(ctx, db, func(tx *gorm.DB) error {
		return tx.Model(&AuditLog{}).Count(&count).Error
	})
	if err != nil {
		t.Fatalf("failed to count audit logs: %v", err)
	}
	if count < 2 {
		t.Errorf("system transaction should see at least 2 audit log entries, got %d", count)
	}
}

// TestWithProfileTx verifies SET LOCAL variables are scoped to the transaction
func TestWithProfileTx(t *testing.T) {
	ctx := context.Background()

	// Create test database connection
	db, cleanup := setupTestDB(t)
	defer cleanup()

	rls := NewRLS()

	profileID := uuid.New().String()

	// Execute transaction with profile context
	err := rls.WithProfileTx(ctx, db, profileID, func(tx *gorm.DB) error {
		// Verify variables are set inside the transaction
		var currentProfileID string
		var currentTxMode string

		if err := tx.Raw("SELECT current_setting('app.current_profile_id', true)").Scan(&currentProfileID).Error; err != nil {
			return err
		}
		if err := tx.Raw("SELECT current_setting('app.tx_mode', true)").Scan(&currentTxMode).Error; err != nil {
			return err
		}

		if currentProfileID != profileID {
			t.Errorf("expected profile_id %s, got %s", profileID, currentProfileID)
		}
		if currentTxMode != "profile" {
			t.Errorf("expected tx_mode profile, got %s", currentTxMode)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}

	// Verify variables are NOT set outside the transaction (SET LOCAL scoped correctly)
	var currentProfileID string
	var currentTxMode string

	err = db.Raw("SELECT current_setting('app.current_profile_id', true)").Scan(&currentProfileID).Error
	if err != nil {
		t.Fatalf("failed to check profile_id outside transaction: %v", err)
	}
	if currentProfileID != "" {
		t.Errorf("profile_id should be empty outside transaction (SET LOCAL worked), got %s", currentProfileID)
	}

	err = db.Raw("SELECT current_setting('app.tx_mode', true)").Scan(&currentTxMode).Error
	if err != nil {
		t.Fatalf("failed to check tx_mode outside transaction: %v", err)
	}
	if currentTxMode != "" {
		t.Errorf("tx_mode should be empty outside transaction (SET LOCAL worked), got %s", currentTxMode)
	}
}

// TestWithSystemTx verifies system session variables are set correctly.
func TestWithSystemTx(t *testing.T) {
	ctx := context.Background()

	// Create test database connection
	db, cleanup := setupTestDB(t)
	defer cleanup()

	rls := NewRLS()

	// Execute transaction with system context.
	err := rls.WithSystemTx(ctx, db, func(tx *gorm.DB) error {
		// Verify variables are set inside the transaction
		var currentProfileID string
		var currentTxMode string

		if err := tx.Raw("SELECT current_setting('app.current_profile_id', true)").Scan(&currentProfileID).Error; err != nil {
			return err
		}
		if err := tx.Raw("SELECT current_setting('app.tx_mode', true)").Scan(&currentTxMode).Error; err != nil {
			return err
		}

		if currentProfileID != "" {
			t.Errorf("expected empty profile_id for system transaction, got %s", currentProfileID)
		}
		if currentTxMode != "system" {
			t.Errorf("expected tx_mode system, got %s", currentTxMode)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}

	// Verify variables are NOT set outside the transaction
	var currentTxMode string
	err = db.Raw("SELECT current_setting('app.tx_mode', true)").Scan(&currentTxMode).Error
	if err != nil {
		t.Fatalf("failed to check tx_mode outside transaction: %v", err)
	}
	if currentTxMode != "" {
		t.Errorf("tx_mode should be empty outside transaction (SET LOCAL worked), got %s", currentTxMode)
	}
}

// setupTestDB creates a test database connection, runs all migrations,
// and returns a cleanup function that truncates fixture tables under system context.
// Skips the test when DATABASE_URL is not set.
func setupTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	dsn := GetTestDSN()
	if dsn == "" {
		t.Skip("set DATABASE_URL to run RLS integration tests")
		return nil, func() {}
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	ctx := context.Background()
	migrator, err := NewMigrator(db)
	if err != nil {
		t.Fatalf("failed to create migrator: %v", err)
	}
	if err := migrator.RunUp(ctx); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Clean slate before test: truncate fixture tables under system context.
	rls := NewRLS()
	if err := rls.WithSystemTx(ctx, db, func(tx *gorm.DB) error {
		return tx.Exec("TRUNCATE profiles, api_keys, audit_log CASCADE").Error
	}); err != nil {
		t.Fatalf("failed to truncate fixture tables before test: %v", err)
	}

	cleanup := func() {
		if err := rls.WithSystemTx(ctx, db, func(tx *gorm.DB) error {
			return tx.Exec("TRUNCATE profiles, api_keys, audit_log CASCADE").Error
		}); err != nil {
			t.Logf("warning: cleanup truncate failed: %v", err)
		}
		if sqlDB, err := db.DB(); err == nil && sqlDB != nil {
			_ = sqlDB.Close()
		}
	}

	return db, cleanup
}

// The following gorm fixture structs are intentionally test-local — domain models
// live in internal/domain and are not gorm-annotated. These fixtures exist only
// to exercise RLS policies; production writes go through the repository layer.

// Profile represents the profiles table
type Profile struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key"`
	Name        string     `gorm:"type:varchar(100);not null"`
	Description string     `gorm:"type:text;not null;default:''"`
	Metadata    string     `gorm:"type:jsonb;not null;default:'{}'"`
	Config      string     `gorm:"type:jsonb;not null;default:'{}'"`
	Status      string     `gorm:"type:varchar(20);not null;default:'active'"`
	CreatedAt   time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt   time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	DeletedAt   *time.Time `gorm:"type:timestamptz"`
}

func (Profile) TableName() string {
	return "profiles"
}

// APIKey represents the api_keys table
type APIKey struct {
	ID         uuid.UUID  `gorm:"type:uuid;primary_key"`
	ProfileID  uuid.UUID  `gorm:"type:uuid;not null"`
	KeyHash    string     `gorm:"type:text;not null"`
	KeyPrefix  string     `gorm:"type:varchar(12);not null"`
	Label      string     `gorm:"type:varchar(100);not null;default:''"`
	Scopes     string     `gorm:"type:text[];not null;default:ARRAY['read']"`
	RateLimit  int        `gorm:"type:integer;not null;default:0"`
	ExpiresAt  *time.Time `gorm:"type:timestamptz"`
	RevokedAt  *time.Time `gorm:"type:timestamptz"`
	LastUsedAt *time.Time `gorm:"type:timestamptz"`
	CreatedAt  time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt  time.Time  `gorm:"type:timestamptz;not null;default:now()"`
}

func (APIKey) TableName() string {
	return "api_keys"
}

// AuditLog represents the audit_log table
type AuditLog struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key"`
	ProfileID     *uuid.UUID `gorm:"type:uuid"`
	Timestamp     time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	Operation     string     `gorm:"type:varchar(64);not null"`
	EntityType    string     `gorm:"type:varchar(64);not null"`
	EntityID      string     `gorm:"type:text;not null"`
	BeforePayload *string    `gorm:"type:jsonb"`
	AfterPayload  *string    `gorm:"type:jsonb"`
	ActorKeyID    *uuid.UUID `gorm:"type:uuid"`
	ActorRole     string     `gorm:"type:varchar(20)"`
	ClientIP      string     `gorm:"type:inet"`
	CorrelationID *uuid.UUID `gorm:"type:uuid"`
	Metadata      string     `gorm:"type:jsonb;not null;default:'{}'"`
}

func (AuditLog) TableName() string {
	return "audit_log"
}

// createTestProfile inserts a profile using profile-scoped RLS context.
func createTestProfile(t *testing.T, db *gorm.DB, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	profile := Profile{
		ID:          id,
		Name:        name,
		Description: "Test profile",
		Metadata:    "{}",
		Config:      "{}",
		Status:      "active",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := NewRLS().WithProfileTx(context.Background(), db, id.String(), func(tx *gorm.DB) error {
		return tx.Create(&profile).Error
	})
	if err != nil {
		t.Fatalf("failed to create test profile: %v", err)
	}
	return id
}

// createTestAPIKey inserts an api_key using profile-scoped RLS context.
func createTestAPIKey(t *testing.T, db *gorm.DB, profileID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	key := APIKey{
		ID:        id,
		ProfileID: profileID,
		KeyHash:   "test_hash_" + id.String(),
		KeyPrefix: id.String()[:12],
		Label:     "Test Key",
		Scopes:    "{read}",
		RateLimit: 0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := NewRLS().WithProfileTx(context.Background(), db, profileID.String(), func(tx *gorm.DB) error {
		return tx.Create(&key).Error
	})
	if err != nil {
		t.Fatalf("failed to create test API key: %v", err)
	}
	return id
}

//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ConnectorCredentials represents the connector_credentials table
type ConnectorCredentials struct {
	ID              uuid.UUID  `gorm:"type:uuid;primary_key"`
	ProfileID       uuid.UUID  `gorm:"type:uuid;not null"`
	ConnectorType   string     `gorm:"type:varchar(50);not null"`
	CredentialName  string     `gorm:"type:varchar(100);not null"`
	EncryptedSecret []byte     `gorm:"type:bytea;not null"`
	Metadata        string     `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt       time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt       time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	DeletedAt       *time.Time `gorm:"type:timestamptz"`
}

func (ConnectorCredentials) TableName() string {
	return "connector_credentials"
}

// ConnectorSyncState represents the connector_sync_state table
type ConnectorSyncState struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key"`
	ProfileID     uuid.UUID  `gorm:"type:uuid;not null"`
	ConnectorType string     `gorm:"type:varchar(50);not null"`
	SourceID      string     `gorm:"type:varchar(255);not null"`
	LastSyncAt    *time.Time `gorm:"type:timestamptz"`
	Cursor        *string    `gorm:"type:text"`
	Status        string     `gorm:"type:varchar(20);not null"`
	ErrorMessage  *string    `gorm:"type:text"`
	ItemsSynced   int        `gorm:"type:integer;not null;default:0"`
	CreatedAt     time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt     time.Time  `gorm:"type:timestamptz;not null;default:now()"`
}

func (ConnectorSyncState) TableName() string {
	return "connector_sync_state"
}

// TestConnectorIsolationSchemaCredentialsTable verifies the connector_credentials table exists with all columns, unique index, and FK cascade.
func TestConnectorIsolationSchemaCredentialsTable(t *testing.T) {
	ctx := context.Background()

	dsn, cleanup := skipIfNoPostgres(t, ctx)
	defer cleanup()

	cfg := &testConfig{dsn: dsn}

	db, err := Open(ctx, cfg)
	require.NoError(t, err, "Open should succeed")
	defer func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}()

	m, err := NewMigrator(db)
	require.NoError(t, err, "NewMigrator should succeed")

	// Run up migrations
	err = m.RunUp(ctx)
	require.NoError(t, err, "RunUp should succeed")

	sqlDB, err := db.DB()
	require.NoError(t, err, "should get underlying sql.DB")

	// Verify connector_credentials table exists
	var tableExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = 'connector_credentials'
		)
	`).Scan(&tableExists)
	require.NoError(t, err, "should check table existence")
	assert.True(t, tableExists, "connector_credentials table should exist")

	// Verify all columns exist
	expectedColumns := []string{
		"id", "profile_id", "connector_type", "credential_name",
		"encrypted_secret", "metadata", "created_at", "updated_at", "deleted_at",
	}
	for _, col := range expectedColumns {
		var colExists bool
		err = sqlDB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'connector_credentials' AND column_name = $1
			)
		`, col).Scan(&colExists)
		require.NoError(t, err, "should check column existence for %s", col)
		assert.True(t, colExists, "connector_credentials.%s column should exist", col)
	}

	// Verify foreign key constraint on profile_id with ON DELETE CASCADE
	var fkExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.table_constraints 
			WHERE table_schema = 'public' AND table_name = 'connector_credentials' 
			AND constraint_type = 'FOREIGN KEY'
		)
	`).Scan(&fkExists)
	require.NoError(t, err, "should check FK existence")
	assert.True(t, fkExists, "connector_credentials should have a foreign key constraint")

	// Verify ON DELETE CASCADE behavior
	// Create a profile
	var profileID string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO profiles (id, name, status)
		VALUES (gen_random_uuid(), 'test-profile-credentials', 'active')
		RETURNING id
	`).Scan(&profileID)
	require.NoError(t, err, "should create test profile")

	// Create a credential for the profile
	var credID string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO connector_credentials (id, profile_id, connector_type, credential_name, encrypted_secret)
		VALUES (gen_random_uuid(), $1, 'test-connector', 'test-cred', '\x00'::bytea)
		RETURNING id
	`, profileID).Scan(&credID)
	require.NoError(t, err, "should create test credential")

	// Verify credential exists
	var credCount int
	err = sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM connector_credentials WHERE id = $1`, credID).Scan(&credCount)
	require.NoError(t, err, "should count credentials")
	assert.Equal(t, 1, credCount, "credential should exist")

	// Delete the profile - should cascade delete the credential
	_, err = sqlDB.ExecContext(ctx, `DELETE FROM profiles WHERE id = $1`, profileID)
	require.NoError(t, err, "should delete profile")

	// Verify credential is deleted due to CASCADE
	err = sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM connector_credentials WHERE id = $1`, credID).Scan(&credCount)
	require.NoError(t, err, "should count credentials after profile deletion")
	assert.Equal(t, 0, credCount, "credential should be deleted due to ON DELETE CASCADE")

	// Verify partial unique index exists
	var idxExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes 
			WHERE schemaname = 'public' AND indexname = 'idx_connector_credentials_unique_active'
		)
	`).Scan(&idxExists)
	require.NoError(t, err, "should check index existence")
	assert.True(t, idxExists, "idx_connector_credentials_unique_active index should exist")

	// Verify the partial unique index enforces uniqueness
	// Create another profile
	var profileID2 string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO profiles (id, name, status)
		VALUES (gen_random_uuid(), 'test-profile-credentials-2', 'active')
		RETURNING id
	`).Scan(&profileID2)
	require.NoError(t, err, "should create second test profile")

	// Create a credential
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO connector_credentials (id, profile_id, connector_type, credential_name, encrypted_secret)
		VALUES (gen_random_uuid(), $1, 'test-connector', 'test-cred', '\x00'::bytea)
	`, profileID2)
	require.NoError(t, err, "should create first credential")

	// Try to create a duplicate credential (same profile_id, connector_type, credential_name)
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO connector_credentials (id, profile_id, connector_type, credential_name, encrypted_secret)
		VALUES (gen_random_uuid(), $1, 'test-connector', 'test-cred', '\x00'::bytea)
	`, profileID2)
	assert.Error(t, err, "duplicate active credential should fail due to unique index")

	// Create a soft-deleted credential with same values - should succeed
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO connector_credentials (id, profile_id, connector_type, credential_name, encrypted_secret, deleted_at)
		VALUES (gen_random_uuid(), $1, 'test-connector', 'test-cred', '\x00'::bytea, now())
	`, profileID2)
	assert.NoError(t, err, "soft-deleted duplicate credential should be allowed")

	// Cleanup
	_, _ = sqlDB.ExecContext(ctx, `DELETE FROM connector_credentials WHERE profile_id = $1`, profileID2)
	_, _ = sqlDB.ExecContext(ctx, `DELETE FROM profiles WHERE id = $1`, profileID2)
}

// TestConnectorIsolationSchemaSyncStateTable verifies the connector_sync_state table exists with all columns, unique index, and status check constraint.
func TestConnectorIsolationSchemaSyncStateTable(t *testing.T) {
	ctx := context.Background()

	dsn, cleanup := skipIfNoPostgres(t, ctx)
	defer cleanup()

	cfg := &testConfig{dsn: dsn}

	db, err := Open(ctx, cfg)
	require.NoError(t, err, "Open should succeed")
	defer func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}()

	m, err := NewMigrator(db)
	require.NoError(t, err, "NewMigrator should succeed")

	// Run up migrations
	err = m.RunUp(ctx)
	require.NoError(t, err, "RunUp should succeed")

	sqlDB, err := db.DB()
	require.NoError(t, err, "should get underlying sql.DB")

	// Verify connector_sync_state table exists
	var tableExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = 'connector_sync_state'
		)
	`).Scan(&tableExists)
	require.NoError(t, err, "should check table existence")
	assert.True(t, tableExists, "connector_sync_state table should exist")

	// Verify all columns exist
	expectedColumns := []string{
		"id", "profile_id", "connector_type", "source_id",
		"last_sync_at", "cursor", "status", "error_message",
		"items_synced", "created_at", "updated_at",
	}
	for _, col := range expectedColumns {
		var colExists bool
		err = sqlDB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'connector_sync_state' AND column_name = $1
			)
		`, col).Scan(&colExists)
		require.NoError(t, err, "should check column existence for %s", col)
		assert.True(t, colExists, "connector_sync_state.%s column should exist", col)
	}

	// Verify foreign key constraint on profile_id
	var fkExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.table_constraints 
			WHERE table_schema = 'public' AND table_name = 'connector_sync_state' 
			AND constraint_type = 'FOREIGN KEY'
		)
	`).Scan(&fkExists)
	require.NoError(t, err, "should check FK existence")
	assert.True(t, fkExists, "connector_sync_state should have a foreign key constraint")

	// Verify status check constraint
	var checkConstraintExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.check_constraints 
			WHERE constraint_name = 'connector_sync_state_status_check'
		)
	`).Scan(&checkConstraintExists)
	require.NoError(t, err, "should check status constraint existence")
	assert.True(t, checkConstraintExists, "connector_sync_state_status_check constraint should exist")

	// Verify unique index exists
	var idxExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes 
			WHERE schemaname = 'public' AND indexname = 'idx_connector_sync_state_unique'
		)
	`).Scan(&idxExists)
	require.NoError(t, err, "should check index existence")
	assert.True(t, idxExists, "idx_connector_sync_state_unique index should exist")

	// Create a profile for testing
	var profileID string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO profiles (id, name, status)
		VALUES (gen_random_uuid(), 'test-profile-sync', 'active')
		RETURNING id
	`).Scan(&profileID)
	require.NoError(t, err, "should create test profile")

	// Test valid status values
	validStatuses := []string{"idle", "syncing", "error"}
	for _, status := range validStatuses {
		_, err = sqlDB.ExecContext(ctx, `
			INSERT INTO connector_sync_state (id, profile_id, connector_type, source_id, status)
			VALUES (gen_random_uuid(), $1, 'test-connector', $2, $3)
		`, profileID, "source-"+status, status)
		assert.NoError(t, err, "status '%s' should be valid", status)
	}

	// Test invalid status value
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO connector_sync_state (id, profile_id, connector_type, source_id, status)
		VALUES (gen_random_uuid(), $1, 'test-connector', 'invalid-source', 'invalid')
	`, profileID)
	assert.Error(t, err, "invalid status should fail check constraint")

	// Test unique index enforcement
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO connector_sync_state (id, profile_id, connector_type, source_id, status)
		VALUES (gen_random_uuid(), $1, 'test-connector', 'duplicate-source', 'idle')
	`, profileID)
	require.NoError(t, err, "first insert should succeed")

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO connector_sync_state (id, profile_id, connector_type, source_id, status)
		VALUES (gen_random_uuid(), $1, 'test-connector', 'duplicate-source', 'idle')
	`, profileID)
	assert.Error(t, err, "duplicate (profile_id, connector_type, source_id) should fail unique index")

	// Cleanup
	_, _ = sqlDB.ExecContext(ctx, `DELETE FROM connector_sync_state WHERE profile_id = $1`, profileID)
	_, _ = sqlDB.ExecContext(ctx, `DELETE FROM profiles WHERE id = $1`, profileID)
}

// TestConnectorIsolationSchemaRLSEnabled verifies FORCE ROW LEVEL SECURITY is applied to both tables.
func TestConnectorIsolationSchemaRLSEnabled(t *testing.T) {
	ctx := context.Background()

	dsn, cleanup := skipIfNoPostgres(t, ctx)
	defer cleanup()

	cfg := &testConfig{dsn: dsn}

	db, err := Open(ctx, cfg)
	require.NoError(t, err, "Open should succeed")
	defer func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}()

	m, err := NewMigrator(db)
	require.NoError(t, err, "NewMigrator should succeed")

	// Run up migrations
	err = m.RunUp(ctx)
	require.NoError(t, err, "RunUp should succeed")

	sqlDB, err := db.DB()
	require.NoError(t, err, "should get underlying sql.DB")

	// Verify RLS is enabled and forced on connector_credentials
	var rlsEnabled bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT relrowsecurity FROM pg_class 
		WHERE relname = 'connector_credentials'
	`).Scan(&rlsEnabled)
	require.NoError(t, err, "should check RLS enabled")
	assert.True(t, rlsEnabled, "connector_credentials should have RLS enabled")

	// Verify RLS is forced on connector_credentials
	var rlsForced bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT relforcerowsecurity FROM pg_class 
		WHERE relname = 'connector_credentials'
	`).Scan(&rlsForced)
	require.NoError(t, err, "should check RLS forced")
	assert.True(t, rlsForced, "connector_credentials should have RLS forced")

	// Verify RLS is enabled and forced on connector_sync_state
	err = sqlDB.QueryRowContext(ctx, `
		SELECT relrowsecurity FROM pg_class 
		WHERE relname = 'connector_sync_state'
	`).Scan(&rlsEnabled)
	require.NoError(t, err, "should check RLS enabled on sync_state")
	assert.True(t, rlsEnabled, "connector_sync_state should have RLS enabled")

	err = sqlDB.QueryRowContext(ctx, `
		SELECT relforcerowsecurity FROM pg_class 
		WHERE relname = 'connector_sync_state'
	`).Scan(&rlsForced)
	require.NoError(t, err, "should check RLS forced on sync_state")
	assert.True(t, rlsForced, "connector_sync_state should have RLS forced")

	// Verify policies exist for connector_credentials
	var credSelfPolicyExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_policies 
			WHERE tablename = 'connector_credentials' AND policyname = 'connector_credentials_self_access'
		)
	`).Scan(&credSelfPolicyExists)
	require.NoError(t, err, "should check self policy existence")
	assert.True(t, credSelfPolicyExists, "connector_credentials_self_access policy should exist")

	var credAdminPolicyExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_policies 
			WHERE tablename = 'connector_credentials' AND policyname = 'connector_credentials_admin_access'
		)
	`).Scan(&credAdminPolicyExists)
	require.NoError(t, err, "should check admin policy existence")
	assert.True(t, credAdminPolicyExists, "connector_credentials_admin_access policy should exist")

	// Verify policies exist for connector_sync_state
	var syncSelfPolicyExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_policies 
			WHERE tablename = 'connector_sync_state' AND policyname = 'connector_sync_state_self_access'
		)
	`).Scan(&syncSelfPolicyExists)
	require.NoError(t, err, "should check sync self policy existence")
	assert.True(t, syncSelfPolicyExists, "connector_sync_state_self_access policy should exist")

	var syncAdminPolicyExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_policies 
			WHERE tablename = 'connector_sync_state' AND policyname = 'connector_sync_state_admin_access'
		)
	`).Scan(&syncAdminPolicyExists)
	require.NoError(t, err, "should check sync admin policy existence")
	assert.True(t, syncAdminPolicyExists, "connector_sync_state_admin_access policy should exist")
}

// TestConnectorIsolationSchemaCrossProfileBlocked verifies profile A cannot read profile B's credentials.
func TestConnectorIsolationSchemaCrossProfileBlocked(t *testing.T) {
	ctx := context.Background()

	dsn, cleanup := skipIfNoPostgres(t, ctx)
	defer cleanup()

	cfg := &testConfig{dsn: dsn}

	db, err := Open(ctx, cfg)
	require.NoError(t, err, "Open should succeed")
	defer func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}()

	m, err := NewMigrator(db)
	require.NoError(t, err, "NewMigrator should succeed")

	// Run up migrations
	err = m.RunUp(ctx)
	require.NoError(t, err, "RunUp should succeed")

	sqlDB, err := db.DB()
	require.NoError(t, err, "should get underlying sql.DB")

	// Create two profiles
	var profileA, profileB string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO profiles (id, name, status)
		VALUES (gen_random_uuid(), 'profile-a', 'active')
		RETURNING id
	`).Scan(&profileA)
	require.NoError(t, err, "should create profile A")

	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO profiles (id, name, status)
		VALUES (gen_random_uuid(), 'profile-b', 'active')
		RETURNING id
	`).Scan(&profileB)
	require.NoError(t, err, "should create profile B")

	// Create a credential for profile A
	var credA string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO connector_credentials (id, profile_id, connector_type, credential_name, encrypted_secret)
		VALUES (gen_random_uuid(), $1, 'test-connector', 'cred-a', '\x00'::bytea)
		RETURNING id
	`, profileA).Scan(&credA)
	require.NoError(t, err, "should create credential for profile A")

	// Create a credential for profile B
	var credB string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO connector_credentials (id, profile_id, connector_type, credential_name, encrypted_secret)
		VALUES (gen_random_uuid(), $1, 'test-connector', 'cred-b', '\x00'::bytea)
		RETURNING id
	`, profileB).Scan(&credB)
	require.NoError(t, err, "should create credential for profile B")

	// Create sync state for profile A
	var syncA string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO connector_sync_state (id, profile_id, connector_type, source_id, status)
		VALUES (gen_random_uuid(), $1, 'test-connector', 'source-a', 'idle')
		RETURNING id
	`, profileA).Scan(&syncA)
	require.NoError(t, err, "should create sync state for profile A")

	// Create sync state for profile B
	var syncB string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO connector_sync_state (id, profile_id, connector_type, source_id, status)
		VALUES (gen_random_uuid(), $1, 'test-connector', 'source-b', 'idle')
		RETURNING id
	`, profileB).Scan(&syncB)
	require.NoError(t, err, "should create sync state for profile B")

	rls := NewRLS()

	// Verify profile A can only see its own credentials
	var count int64
	err = rls.WithProfileTx(ctx, db, profileA, "standard", func(tx *gorm.DB) error {
		return tx.Model(&ConnectorCredentials{}).Count(&count).Error
	})
	require.NoError(t, err, "profile A should be able to query credentials")
	assert.Equal(t, int64(1), count, "profile A should see exactly 1 credential")

	// Verify profile A cannot see profile B's credential
	var foundCred ConnectorCredentials
	err = rls.WithProfileTx(ctx, db, profileA, "standard", func(tx *gorm.DB) error {
		return tx.First(&foundCred, "id = ?", credB).Error
	})
	assert.Error(t, err, "profile A should not be able to read profile B's credential")

	// Verify profile B can only see its own credentials
	err = rls.WithProfileTx(ctx, db, profileB, "standard", func(tx *gorm.DB) error {
		return tx.Model(&ConnectorCredentials{}).Count(&count).Error
	})
	require.NoError(t, err, "profile B should be able to query credentials")
	assert.Equal(t, int64(1), count, "profile B should see exactly 1 credential")

	// Verify profile B cannot see profile A's credential
	err = rls.WithProfileTx(ctx, db, profileB, "standard", func(tx *gorm.DB) error {
		return tx.First(&foundCred, "id = ?", credA).Error
	})
	assert.Error(t, err, "profile B should not be able to read profile A's credential")

	// Verify profile A can only see its own sync state
	err = rls.WithProfileTx(ctx, db, profileA, "standard", func(tx *gorm.DB) error {
		return tx.Model(&ConnectorSyncState{}).Count(&count).Error
	})
	require.NoError(t, err, "profile A should be able to query sync state")
	assert.Equal(t, int64(1), count, "profile A should see exactly 1 sync state")

	// Verify profile A cannot see profile B's sync state
	var foundSync ConnectorSyncState
	err = rls.WithProfileTx(ctx, db, profileA, "standard", func(tx *gorm.DB) error {
		return tx.First(&foundSync, "id = ?", syncB).Error
	})
	assert.Error(t, err, "profile A should not be able to read profile B's sync state")

	// Verify profile B can only see its own sync state
	err = rls.WithProfileTx(ctx, db, profileB, "standard", func(tx *gorm.DB) error {
		return tx.Model(&ConnectorSyncState{}).Count(&count).Error
	})
	require.NoError(t, err, "profile B should be able to query sync state")
	assert.Equal(t, int64(1), count, "profile B should see exactly 1 sync state")

	// Verify admin can see all credentials
	err = rls.WithAdminTx(ctx, db, func(tx *gorm.DB) error {
		return tx.Model(&ConnectorCredentials{}).Count(&count).Error
	})
	require.NoError(t, err, "admin should be able to query all credentials")
	assert.Equal(t, int64(2), count, "admin should see all 2 credentials")

	// Verify admin can see all sync state
	err = rls.WithAdminTx(ctx, db, func(tx *gorm.DB) error {
		return tx.Model(&ConnectorSyncState{}).Count(&count).Error
	})
	require.NoError(t, err, "admin should be able to query all sync state")
	assert.Equal(t, int64(2), count, "admin should see all 2 sync states")

	// Cleanup
	_, _ = sqlDB.ExecContext(ctx, `DELETE FROM connector_credentials WHERE profile_id IN ($1, $2)`, profileA, profileB)
	_, _ = sqlDB.ExecContext(ctx, `DELETE FROM connector_sync_state WHERE profile_id IN ($1, $2)`, profileA, profileB)
	_, _ = sqlDB.ExecContext(ctx, `DELETE FROM profiles WHERE id IN ($1, $2)`, profileA, profileB)
}

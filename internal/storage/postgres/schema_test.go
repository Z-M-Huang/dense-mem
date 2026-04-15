//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SchemaVerifier defines the interface for verifying database schema.
// This companion interface enables mocking for unit tests.
type SchemaVerifier interface {
	TableExists(ctx context.Context, tableName string) (bool, error)
	ColumnExists(ctx context.Context, tableName, columnName string) (bool, error)
	ConstraintExists(ctx context.Context, tableName, constraintName string) (bool, error)
	IndexExists(ctx context.Context, indexName string) (bool, error)
	GetColumnType(ctx context.Context, tableName, columnName string) (string, error)
}

// TestCoreSchemaProfilesTable verifies the profiles table exists with all columns and constraints.
func TestCoreSchemaProfilesTable(t *testing.T) {
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

	// Verify profiles table exists
	var tableExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = 'profiles'
		)
	`).Scan(&tableExists)
	require.NoError(t, err, "should check table existence")
	assert.True(t, tableExists, "profiles table should exist")

	// Verify all columns exist
	expectedColumns := []string{
		"id", "name", "description", "metadata", "config",
		"status", "created_at", "updated_at", "deleted_at",
	}
	for _, col := range expectedColumns {
		var colExists bool
		err = sqlDB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'profiles' AND column_name = $1
			)
		`, col).Scan(&colExists)
		require.NoError(t, err, "should check column existence for %s", col)
		assert.True(t, colExists, "profiles.%s column should exist", col)
	}

	// Verify status check constraint
	var constraintExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.check_constraints 
			WHERE constraint_name = 'profiles_status_check'
		)
	`).Scan(&constraintExists)
	require.NoError(t, err, "should check constraint existence")
	assert.True(t, constraintExists, "profiles_status_check constraint should exist")
}

// TestCoreSchemaAPIKeysTable verifies the api_keys table, check constraints for admin/standard role split.
func TestCoreSchemaAPIKeysTable(t *testing.T) {
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

	// Verify api_keys table exists
	var tableExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = 'api_keys'
		)
	`).Scan(&tableExists)
	require.NoError(t, err, "should check table existence")
	assert.True(t, tableExists, "api_keys table should exist")

	// Verify all columns exist
	expectedColumns := []string{
		"id", "profile_id", "key_hash", "key_prefix", "label",
		"role", "scopes", "expires_at", "revoked_at", "last_used_at",
		"created_at", "updated_at",
	}
	for _, col := range expectedColumns {
		var colExists bool
		err = sqlDB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'api_keys' AND column_name = $1
			)
		`, col).Scan(&colExists)
		require.NoError(t, err, "should check column existence for %s", col)
		assert.True(t, colExists, "api_keys.%s column should exist", col)
	}

	// Verify role check constraint
	var roleConstraintExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.check_constraints 
			WHERE constraint_name = 'api_keys_role_check'
		)
	`).Scan(&roleConstraintExists)
	require.NoError(t, err, "should check role constraint existence")
	assert.True(t, roleConstraintExists, "api_keys_role_check constraint should exist")

	// Verify the admin/standard profile_id check constraint
	var profileCheckExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.check_constraints 
			WHERE constraint_name = 'chk_api_keys_role_profile'
		)
	`).Scan(&profileCheckExists)
	require.NoError(t, err, "should check profile constraint existence")
	assert.True(t, profileCheckExists, "chk_api_keys_role_profile constraint should exist")

	// Verify foreign key constraint on profile_id
	var fkExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.table_constraints 
			WHERE table_schema = 'public' AND table_name = 'api_keys' 
			AND constraint_type = 'FOREIGN KEY'
		)
	`).Scan(&fkExists)
	require.NoError(t, err, "should check FK existence")
	assert.True(t, fkExists, "api_keys should have a foreign key constraint")

	// Test the check constraint: admin keys must have profile_id IS NULL
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO api_keys (id, key_hash, key_prefix, role, scopes)
		VALUES (gen_random_uuid(), 'hash123', 'prefix', 'admin', ARRAY['read'])
	`)
	assert.NoError(t, err, "admin key with NULL profile_id should be allowed")

	// Test the check constraint: standard keys must have profile_id IS NOT NULL
	// First create a profile
	var profileID string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO profiles (id, name, status)
		VALUES (gen_random_uuid(), 'test-profile', 'active')
		RETURNING id
	`).Scan(&profileID)
	require.NoError(t, err, "should create test profile")

	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO api_keys (id, profile_id, key_hash, key_prefix, role, scopes)
		VALUES (gen_random_uuid(), $1, 'hash456', 'prefix2', 'standard', ARRAY['read'])
	`, profileID)
	assert.NoError(t, err, "standard key with profile_id should be allowed")

	// Test: standard key without profile_id should fail
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO api_keys (id, key_hash, key_prefix, role, scopes)
		VALUES (gen_random_uuid(), 'hash789', 'prefix3', 'standard', ARRAY['read'])
	`)
	assert.Error(t, err, "standard key without profile_id should fail")

	// Test: admin key with profile_id should fail
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO api_keys (id, profile_id, key_hash, key_prefix, role, scopes)
		VALUES (gen_random_uuid(), $1, 'hashabc', 'prefix4', 'admin', ARRAY['read'])
	`, profileID)
	assert.Error(t, err, "admin key with profile_id should fail")
}

// TestCoreSchemaAuditLogTable verifies append semantics via FK cascade rules.
func TestCoreSchemaAuditLogTable(t *testing.T) {
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

	// Verify audit_log table exists
	var tableExists bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = 'audit_log'
		)
	`).Scan(&tableExists)
	require.NoError(t, err, "should check table existence")
	assert.True(t, tableExists, "audit_log table should exist")

	// Verify all columns exist
	expectedColumns := []string{
		"id", "profile_id", "timestamp", "operation", "entity_type",
		"entity_id", "before_payload", "after_payload", "actor_key_id",
		"actor_role", "client_ip", "correlation_id", "metadata",
	}
	for _, col := range expectedColumns {
		var colExists bool
		err = sqlDB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = 'audit_log' AND column_name = $1
			)
		`, col).Scan(&colExists)
		require.NoError(t, err, "should check column existence for %s", col)
		assert.True(t, colExists, "audit_log.%s column should exist", col)
	}

	// Create a profile and an audit log entry to test SET NULL
	var profileID string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO profiles (id, name, status)
		VALUES (gen_random_uuid(), 'audit-test-profile', 'active')
		RETURNING id
	`).Scan(&profileID)
	require.NoError(t, err, "should create test profile")

	var auditLogID string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO audit_log (id, profile_id, operation, entity_type, entity_id)
		VALUES (gen_random_uuid(), $1, 'CREATE', 'test', 'test-123')
		RETURNING id
	`, profileID).Scan(&auditLogID)
	require.NoError(t, err, "should create audit log entry")

	// Verify profile_id is set
	var currentProfileID string
	err = sqlDB.QueryRowContext(ctx, `SELECT profile_id FROM audit_log WHERE id = $1`, auditLogID).Scan(&currentProfileID)
	require.NoError(t, err, "should get audit log entry")
	assert.Equal(t, profileID, currentProfileID, "profile_id should be set")

	// Delete the profile - should set profile_id to NULL (SET NULL behavior)
	_, err = sqlDB.ExecContext(ctx, `DELETE FROM profiles WHERE id = $1`, profileID)
	require.NoError(t, err, "should delete profile without error due to SET NULL")

	// Verify profile_id is now NULL
	var nullProfileID interface{}
	err = sqlDB.QueryRowContext(ctx, `SELECT profile_id FROM audit_log WHERE id = $1`, auditLogID).Scan(&nullProfileID)
	require.NoError(t, err, "should get audit log entry after profile deletion")
	assert.Nil(t, nullProfileID, "profile_id should be NULL after profile deletion (SET NULL)")

	// Verify ON DELETE SET NULL for actor_key_id FK
	// Create an admin key
	var keyID string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO api_keys (id, key_hash, key_prefix, role, scopes)
		VALUES (gen_random_uuid(), 'testhash', 'test', 'admin', ARRAY['read'])
		RETURNING id
	`).Scan(&keyID)
	require.NoError(t, err, "should create test api key")

	// Create another audit log entry with actor_key_id
	var auditLogID2 string
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO audit_log (id, actor_key_id, operation, entity_type, entity_id)
		VALUES (gen_random_uuid(), $1, 'DELETE', 'test', 'test-456')
		RETURNING id
	`, keyID).Scan(&auditLogID2)
	require.NoError(t, err, "should create audit log entry with actor_key_id")

	// Delete the key - should set actor_key_id to NULL (SET NULL behavior)
	_, err = sqlDB.ExecContext(ctx, `DELETE FROM api_keys WHERE id = $1`, keyID)
	require.NoError(t, err, "should delete api key without error due to SET NULL")

	// Verify actor_key_id is now NULL
	var nullKeyID interface{}
	err = sqlDB.QueryRowContext(ctx, `SELECT actor_key_id FROM audit_log WHERE id = $1`, auditLogID2).Scan(&nullKeyID)
	require.NoError(t, err, "should get audit log entry after key deletion")
	assert.Nil(t, nullKeyID, "actor_key_id should be NULL after key deletion (SET NULL)")
}

// TestCoreSchemaIndexes verifies all four indexes exist.
func TestCoreSchemaIndexes(t *testing.T) {
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

	// Expected indexes
	expectedIndexes := []string{
		"idx_profiles_name_unique_active",
		"idx_api_keys_profile_id",
		"idx_api_keys_key_prefix",
		"idx_audit_log_profile_timestamp",
		"idx_audit_log_timestamp",
	}

	for _, idxName := range expectedIndexes {
		var idxExists bool
		err = sqlDB.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_indexes 
				WHERE schemaname = 'public' AND indexname = $1
			)
		`, idxName).Scan(&idxExists)
		require.NoError(t, err, "should check index existence for %s", idxName)
		assert.True(t, idxExists, "index %s should exist", idxName)
	}

	// Verify the partial unique index specifically (idx_profiles_name_unique_active)
	var isUnique bool
	err = sqlDB.QueryRowContext(ctx, `
		SELECT indisunique 
		FROM pg_index 
		WHERE indrelid = 'profiles'::regclass 
		AND indpred IS NOT NULL
	`).Scan(&isUnique)
	require.NoError(t, err, "should check if profiles index is unique")
	assert.True(t, isUnique, "idx_profiles_name_unique_active should be unique")
}

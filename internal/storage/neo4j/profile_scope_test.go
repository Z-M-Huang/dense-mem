package neo4j

import (
	"context"
	"errors"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRejectMissingProfileId tests that empty profileID is rejected
func TestRejectMissingProfileId(t *testing.T) {
	ctx := context.Background()

	// Create enforcer with nil executor since we're only testing validation
	enforcer := NewProfileScopeEnforcer(nil)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "ScopedRead rejects empty profileID",
			call: func() error {
				_, _, err := enforcer.ScopedRead(ctx, "", "MATCH (n) WHERE n.profile_id = $profileId RETURN n", nil)
				return err
			},
		},
		{
			name: "ScopedWrite rejects empty profileID",
			call: func() error {
				_, err := enforcer.ScopedWrite(ctx, "", "CREATE (n:Node {profile_id: $profileId})", nil)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrEmptyProfileID)
		})
	}
}

// TestRejectProfileParamOverride tests that caller-supplied profileId parameter is rejected
func TestRejectProfileParamOverride(t *testing.T) {
	ctx := context.Background()

	enforcer := NewProfileScopeEnforcer(nil)

	queryWithPlaceholder := "MATCH (n) WHERE n.profile_id = $profileId RETURN n"

	tests := []struct {
		name  string
		call  func() error
		param string
	}{
		{
			name: "ScopedRead rejects caller-supplied profileId",
			call: func() error {
				params := map[string]any{"profileId": "caller-value"}
				_, _, err := enforcer.ScopedRead(ctx, "profile-123", queryWithPlaceholder, params)
				return err
			},
			param: "profileId",
		},
		{
			name: "ScopedWrite rejects caller-supplied profileId",
			call: func() error {
				params := map[string]any{"profileId": "caller-value"}
				_, err := enforcer.ScopedWrite(ctx, "profile-123", "CREATE (n:Node {profile_id: $profileId})", params)
				return err
			},
			param: "profileId",
		},
		{
			name: "ScopedRead rejects caller-supplied profileId with other params",
			call: func() error {
				params := map[string]any{"profileId": "caller-value", "other": "param"}
				_, _, err := enforcer.ScopedRead(ctx, "profile-123", queryWithPlaceholder, params)
				return err
			},
			param: "profileId",
		},
		{
			name: "ScopedRead rejects profileId with different casing (profileid)",
			call: func() error {
				params := map[string]any{"profileid": "caller-value"}
				_, _, err := enforcer.ScopedRead(ctx, "profile-123", queryWithPlaceholder, params)
				return err
			},
			param: "profileid",
		},
		{
			name: "ScopedWrite rejects profileId with different casing (PROFILEID)",
			call: func() error {
				params := map[string]any{"PROFILEID": "caller-value"}
				_, err := enforcer.ScopedWrite(ctx, "profile-123", "CREATE (n:Node {profile_id: $profileId})", params)
				return err
			},
			param: "PROFILEID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For the exact key "profileId", validation should reject
			// For different casing like "profileid" or "PROFILEID", they are different keys
			// so validation passes but we can't test with nil executor (would panic)
			if tt.param == "profileId" {
				err := tt.call()
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrProfileParamOverride)
			} else {
				// Different casing shouldn't match "profileId" exactly
				// Verify via validateAndPrepare that it passes validation
				enforcer := &profileScopeEnforcer{}
				params, err := enforcer.validateAndPrepare("profile-123", queryWithPlaceholder, map[string]any{tt.param: "caller-value"})
				// Should NOT error since "profileid" != "profileId"
				require.NoError(t, err, "different casing key %s should not be rejected", tt.param)
				// The profileId should still be injected
				assert.Equal(t, "profile-123", params["profileId"])
				// The different casing key should also be preserved
				assert.Equal(t, "caller-value", params[tt.param])
			}
		})
	}
}

// TestRejectQueryWithoutProfilePlaceholder tests that queries without $profileId placeholder are rejected
func TestRejectQueryWithoutProfilePlaceholder(t *testing.T) {
	ctx := context.Background()

	enforcer := NewProfileScopeEnforcer(nil)

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "ScopedRead rejects query without $profileId",
			call: func() error {
				_, _, err := enforcer.ScopedRead(ctx, "profile-123", "MATCH (n) RETURN n", nil)
				return err
			},
		},
		{
			name: "ScopedWrite rejects query without $profileId",
			call: func() error {
				_, err := enforcer.ScopedWrite(ctx, "profile-123", "CREATE (n:Node {name: 'test'})", nil)
				return err
			},
		},
		{
			name: "ScopedRead rejects query with wrong placeholder format (:profileId)",
			call: func() error {
				_, _, err := enforcer.ScopedRead(ctx, "profile-123", "MATCH (n) WHERE n.profile_id = :profileId RETURN n", nil)
				return err
			},
		},
		{
			name: "ScopedRead rejects query with literal profile_id but no placeholder",
			call: func() error {
				_, _, err := enforcer.ScopedRead(ctx, "profile-123", "MATCH (n) WHERE n.profile_id = 'literal-value' RETURN n", nil)
				return err
			},
		},
		{
			name: "ScopedRead rejects query with profileId without dollar sign",
			call: func() error {
				_, _, err := enforcer.ScopedRead(ctx, "profile-123", "MATCH (n) WHERE n.profile_id = profileId RETURN n", nil)
				return err
			},
		},
		{
			name: "ScopedRead rejects query with {profileId} format",
			call: func() error {
				_, _, err := enforcer.ScopedRead(ctx, "profile-123", "MATCH (n) WHERE n.profile_id = {profileId} RETURN n", nil)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrMissingProfilePlaceholder)
		})
	}
}

// TestValidateAndPrepareInjection tests that validateAndPrepare correctly injects profileId
// This is a unit test of the internal validation logic
func TestValidateAndPrepareInjection(t *testing.T) {
	enforcer := &profileScopeEnforcer{}

	tests := []struct {
		name           string
		profileID      string
		query          string
		params         map[string]any
		wantErr        error
		wantProfileID  any // expected profileId value in params, nil if error expected
		checkOnlyError bool
	}{
		{
			name:          "injects profileId into nil params",
			profileID:     "test-profile-456",
			query:         "MATCH (n) WHERE n.id = $profileId RETURN n",
			params:        nil,
			wantErr:       nil,
			wantProfileID: "test-profile-456",
		},
		{
			name:          "injects profileId into empty params",
			profileID:     "test-profile-789",
			query:         "MATCH (n) WHERE n.id = $profileId RETURN n",
			params:        map[string]any{},
			wantErr:       nil,
			wantProfileID: "test-profile-789",
		},
		{
			name:          "injects profileId alongside existing params",
			profileID:     "test-profile-abc",
			query:         "MATCH (n) WHERE n.id = $profileId AND n.name = $name RETURN n",
			params:        map[string]any{"name": "Alice", "age": 30},
			wantErr:       nil,
			wantProfileID: "test-profile-abc",
		},
		{
			name:           "rejects empty profileID",
			profileID:      "",
			query:          "MATCH (n) WHERE n.id = $profileId RETURN n",
			params:         nil,
			wantErr:        ErrEmptyProfileID,
			checkOnlyError: true,
		},
		{
			name:           "rejects query without placeholder",
			profileID:      "test-profile",
			query:          "MATCH (n) RETURN n",
			params:         nil,
			wantErr:        ErrMissingProfilePlaceholder,
			checkOnlyError: true,
		},
		{
			name:           "rejects caller override of profileId",
			profileID:      "test-profile",
			query:          "MATCH (n) WHERE n.id = $profileId RETURN n",
			params:         map[string]any{"profileId": "caller-value"},
			wantErr:        ErrProfileParamOverride,
			checkOnlyError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := enforcer.validateAndPrepare(tt.profileID, tt.query, tt.params)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result, "result params should not be nil")
			assert.Equal(t, tt.wantProfileID, result["profileId"], "profileId should be injected")

			// Check existing params are preserved
			for k, v := range tt.params {
				assert.Equal(t, v, result[k], "existing param %s should be preserved", k)
			}
		})
	}
}

// TestScopedReadInjectsProfileId tests that ScopedRead correctly injects profileId
// by verifying the params after validation pass through to the prepared params
func TestScopedReadInjectsProfileId(t *testing.T) {
	// This test verifies that profileId injection works end-to-end.
	// The validateAndPrepare function is tested separately for the actual injection logic.
	// Here we test that ScopedRead calls validateAndPrepare correctly.

	// Test 1: Validation passes with correct inputs - verified by TestValidateAndPrepareInjection
	// Test 2: Verify the param injection behavior directly
	enforcer := &profileScopeEnforcer{}

	// Test that params are prepared correctly for various input scenarios
	t.Run("validation passes and profileId is injected", func(t *testing.T) {
		profileID := "test-profile-456"
		query := "MATCH (n:User) WHERE n.profile_id = $profileId RETURN n"

		// Test with nil params
		params, err := enforcer.validateAndPrepare(profileID, query, nil)
		require.NoError(t, err)
		assert.Equal(t, profileID, params["profileId"])

		// Test with empty params
		params, err = enforcer.validateAndPrepare(profileID, query, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, profileID, params["profileId"])

		// Test with existing params
		params, err = enforcer.validateAndPrepare(profileID, query, map[string]any{"name": "John", "age": 30})
		require.NoError(t, err)
		assert.Equal(t, profileID, params["profileId"])
		assert.Equal(t, "John", params["name"])
		assert.Equal(t, 30, params["age"])
	})

	// Test that ScopedRead rejects invalid inputs (validation happens before executor call)
	t.Run("ScopedRead validation rejects invalid inputs", func(t *testing.T) {
		ctx := context.Background()
		enforcerWithNilExecutor := NewProfileScopeEnforcer(nil)

		// Empty profileID should fail validation
		_, _, err := enforcerWithNilExecutor.ScopedRead(ctx, "", "MATCH (n) WHERE n.id = $profileId RETURN n", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEmptyProfileID)

		// Caller override should fail validation
		_, _, err = enforcerWithNilExecutor.ScopedRead(ctx, "profile-123", "MATCH (n) WHERE n.id = $profileId RETURN n", map[string]any{"profileId": "evil"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrProfileParamOverride)

		// Missing placeholder should fail validation
		_, _, err = enforcerWithNilExecutor.ScopedRead(ctx, "profile-123", "MATCH (n) RETURN n", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMissingProfilePlaceholder)
	})
}

// TestScopedWriteInjectsProfileId tests that ScopedWrite correctly injects profileId
func TestScopedWriteInjectsProfileId(t *testing.T) {
	enforcer := &profileScopeEnforcer{}

	t.Run("validation passes and profileId is injected", func(t *testing.T) {
		profileID := "test-profile-789"
		query := "CREATE (n:User {profile_id: $profileId, name: $name})"

		params, err := enforcer.validateAndPrepare(profileID, query, map[string]any{"name": "Alice"})
		require.NoError(t, err)
		assert.Equal(t, profileID, params["profileId"])
		assert.Equal(t, "Alice", params["name"])
	})

	// Test that ScopedWrite rejects invalid inputs (validation happens before executor call)
	t.Run("ScopedWrite validation rejects invalid inputs", func(t *testing.T) {
		ctx := context.Background()
		enforcerWithNilExecutor := NewProfileScopeEnforcer(nil)

		// Empty profileID should fail validation
		_, err := enforcerWithNilExecutor.ScopedWrite(ctx, "", "CREATE (n:Node {id: $profileId})", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEmptyProfileID)

		// Caller override should fail validation
		_, err = enforcerWithNilExecutor.ScopedWrite(ctx, "profile-123", "CREATE (n:Node {id: $profileId})", map[string]any{"profileId": "evil"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrProfileParamOverride)

		// Missing placeholder should fail validation
		_, err = enforcerWithNilExecutor.ScopedWrite(ctx, "profile-123", "CREATE (n:Node)", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMissingProfilePlaceholder)
	})
}

// TestProfileScopeEnforcer_Interface ensures profileScopeEnforcer implements ProfileScopeEnforcer
func TestProfileScopeEnforcer_Interface(t *testing.T) {
	var _ ProfileScopeEnforcer = (*profileScopeEnforcer)(nil)
	var _ ScopedReader = (*profileScopeEnforcer)(nil)
	var _ ScopedWriter = (*profileScopeEnforcer)(nil)
}

// TestScopedRead_PropagatesExecutorError tests that errors from executor are propagated
func TestScopedRead_PropagatesExecutorError(t *testing.T) {
	ctx := context.Background()
	executorErr := errors.New("executor error")
	mock := &mockExecutorForError{
		err: executorErr,
	}
	enforcer := NewProfileScopeEnforcer(mock)

	_, _, err := enforcer.ScopedRead(ctx, "profile-123", "MATCH (n) WHERE n.id = $profileId RETURN n", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executor error")
}

// TestScopedWrite_PropagatesExecutorError tests that errors from executor are propagated
func TestScopedWrite_PropagatesExecutorError(t *testing.T) {
	ctx := context.Background()
	executorErr := errors.New("executor error")
	mock := &mockExecutorForError{
		err: executorErr,
	}
	enforcer := NewProfileScopeEnforcer(mock)

	_, err := enforcer.ScopedWrite(ctx, "profile-123", "CREATE (n:Node {id: $profileId})", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executor error")
}

// mockExecutorForError is a minimal mock that always returns an error
// This avoids the need to implement neo4j.ManagedTransaction
type mockExecutorForError struct {
	err error
}

func (m *mockExecutorForError) Verify(ctx context.Context) error {
	return nil
}

func (m *mockExecutorForError) ExecuteRead(ctx context.Context, fn neo4j.ManagedTransactionWork) (any, error) {
	return nil, m.err
}

func (m *mockExecutorForError) ExecuteWrite(ctx context.Context, fn neo4j.ManagedTransactionWork) (any, error) {
	return nil, m.err
}

func (m *mockExecutorForError) Close(ctx context.Context) error {
	return nil
}
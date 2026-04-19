package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// stubClaimLocker is a test-local implementation of ClaimLocker.
// It records every call for post-call assertions and can be configured to
// return a fixed error without invoking fn.
type stubClaimLocker struct {
	calls     []stubClaimLockCall
	returnErr error
}

type stubClaimLockCall struct {
	profileID string
	claimID   string
	timeout   time.Duration
}

func (s *stubClaimLocker) WithClaimLock(
	_ context.Context,
	_ *gorm.DB,
	profileID, claimID string,
	timeout time.Duration,
	fn func(tx *gorm.DB) error,
) error {
	s.calls = append(s.calls, stubClaimLockCall{
		profileID: profileID,
		claimID:   claimID,
		timeout:   timeout,
	})
	if s.returnErr != nil {
		return s.returnErr
	}
	// Pass nil tx — unit tests must not exercise real DB paths via the stub.
	return fn(nil)
}

// Compile-time interface satisfaction check.
var _ ClaimLocker = (*stubClaimLocker)(nil)

// TestWithClaimLock verifies the ClaimLocker interface contract, ClaimLock
// construction, and lock-key derivation without a real database connection.
func TestWithClaimLock(t *testing.T) {
	t.Run("stub_implements_interface", func(t *testing.T) {
		ctx := context.Background()
		stub := &stubClaimLocker{}
		var locker ClaimLocker = stub

		called := false
		err := locker.WithClaimLock(ctx, nil, "profile-1", "claim-1", 5*time.Second,
			func(_ *gorm.DB) error {
				called = true
				return nil
			})
		require.NoError(t, err)
		require.True(t, called, "fn must be invoked by the stub when no error is set")
		require.Len(t, stub.calls, 1)
		require.Equal(t, "profile-1", stub.calls[0].profileID)
		require.Equal(t, "claim-1", stub.calls[0].claimID)
		require.Equal(t, 5*time.Second, stub.calls[0].timeout)
	})

	t.Run("stub_propagates_fn_error", func(t *testing.T) {
		ctx := context.Background()
		stub := &stubClaimLocker{}
		want := errors.New("fn failure")

		err := stub.WithClaimLock(ctx, nil, "p1", "c1", time.Second,
			func(_ *gorm.DB) error { return want })
		require.ErrorIs(t, err, want,
			"error returned by fn must be propagated to the caller")
	})

	t.Run("stub_returns_locker_error_without_calling_fn", func(t *testing.T) {
		ctx := context.Background()
		want := errors.New("lock acquire failure")
		stub := &stubClaimLocker{returnErr: want}

		called := false
		err := stub.WithClaimLock(ctx, nil, "p1", "c1", time.Second,
			func(_ *gorm.DB) error {
				called = true
				return nil
			})
		require.ErrorIs(t, err, want)
		require.False(t, called, "fn must not be invoked when the locker returns an error")
	})

	t.Run("new_claim_lock_nil_metrics_uses_noop", func(t *testing.T) {
		cl := NewClaimLock(nil)
		require.NotNil(t, cl, "NewClaimLock must never return nil")
		// Verify the noop metrics is set (no panic on use).
		cl.metrics.ObservePromoteLockWait(0.5)
	})

	t.Run("new_claim_lock_explicit_noop_metrics", func(t *testing.T) {
		cl := NewClaimLock(observability.NoopDiscoverabilityMetrics())
		require.NotNil(t, cl)
		var _ ClaimLocker = cl // compile-time interface check
		cl.metrics.ObservePromoteLockWait(0.1)
	})

	t.Run("lock_key_format", func(t *testing.T) {
		key := claimLockKey("my-profile", "my-claim")
		require.Equal(t, "claim:my-profile:my-claim", key,
			"lock key must follow the 'claim:<profileID>:<claimID>' format "+
				"required by the binding spec")
	})
}

// TestWithClaimLock_CrossProfileIsolation verifies that the advisory lock key
// embeds profileID so that promotions of the same claimID across different
// profiles never contend on the same Postgres advisory lock.
func TestWithClaimLock_CrossProfileIsolation(t *testing.T) {
	t.Run("different_profiles_produce_different_lock_keys", func(t *testing.T) {
		keyA := claimLockKey("profile-a", "claim-1")
		keyB := claimLockKey("profile-b", "claim-1")
		require.NotEqual(t, keyA, keyB,
			"same claimID in different profiles must produce different advisory lock keys")
	})

	t.Run("different_claims_same_profile_produce_different_keys", func(t *testing.T) {
		key1 := claimLockKey("profile-a", "claim-1")
		key2 := claimLockKey("profile-a", "claim-2")
		require.NotEqual(t, key1, key2,
			"different claims in the same profile must produce different lock keys")
	})

	t.Run("key_embeds_profile_id", func(t *testing.T) {
		profileID := "unique-profile-id-9a3f"
		key := claimLockKey(profileID, "some-claim")
		require.Contains(t, key, profileID,
			"lock key must embed profileID to enforce per-profile isolation")
	})

	t.Run("key_embeds_claim_id", func(t *testing.T) {
		claimID := "unique-claim-id-7b2e"
		key := claimLockKey("some-profile", claimID)
		require.Contains(t, key, claimID,
			"lock key must embed claimID to scope the lock to a specific claim")
	})

	t.Run("key_is_deterministic", func(t *testing.T) {
		// Same inputs must always produce the same key — required for idempotent
		// re-attempts to contend on the same lock.
		key1 := claimLockKey("profile-x", "claim-y")
		key2 := claimLockKey("profile-x", "claim-y")
		require.Equal(t, key1, key2, "lock key must be deterministic for the same inputs")
	})

	t.Run("stub_isolation_profile_a_not_in_profile_b_calls", func(t *testing.T) {
		ctx := context.Background()
		stubA := &stubClaimLocker{}
		stubB := &stubClaimLocker{}

		// Profile A acquires lock for claim-1.
		_ = stubA.WithClaimLock(ctx, nil, "profile-a", "claim-1", time.Second,
			func(_ *gorm.DB) error { return nil })
		// Profile B acquires lock for the same claimID (different profile).
		_ = stubB.WithClaimLock(ctx, nil, "profile-b", "claim-1", time.Second,
			func(_ *gorm.DB) error { return nil })

		// Verify data from profile A is not present in profile B's lock calls.
		for _, call := range stubB.calls {
			require.NotEqual(t, "profile-a", call.profileID,
				"profile B's lock calls must not contain profile A's profileID")
		}
	})
}

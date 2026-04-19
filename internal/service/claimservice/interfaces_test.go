package claimservice

import (
	"context"
	"testing"
	"time"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/stretchr/testify/require"
)

// stubCreate is a minimal no-op implementation of CreateClaimService used only
// to verify that the interface shape is correct at compile time.
type stubCreate struct{}

func (s *stubCreate) Create(_ context.Context, _ string, _ *domain.Claim) (*CreateResult, error) {
	return nil, nil
}

// stubGet is a minimal no-op implementation of GetClaimService.
type stubGet struct{}

func (s *stubGet) Get(_ context.Context, _ string, _ string) (*domain.Claim, error) {
	return nil, nil
}

// stubList is a minimal no-op implementation of ListClaimsService.
type stubList struct{}

func (s *stubList) List(_ context.Context, _ string, _, _ int) ([]*domain.Claim, int, error) {
	return nil, 0, nil
}

// stubDelete is a minimal no-op implementation of DeleteClaimService.
type stubDelete struct{}

func (s *stubDelete) Delete(_ context.Context, _ string, _ string) error { return nil }

// stubVerify is a minimal no-op implementation of VerifyClaimService.
type stubVerify struct{}

func (s *stubVerify) Verify(_ context.Context, _ string, _ string) (*domain.Claim, error) {
	return nil, nil
}

// stubAudit is a minimal no-op implementation of AuditEmitter.
type stubAudit struct{}

func (s *stubAudit) Append(_ context.Context, _ AuditLogEntry) error { return nil }

// Compile-time interface satisfaction checks.
var (
	_ CreateClaimService = (*stubCreate)(nil)
	_ GetClaimService    = (*stubGet)(nil)
	_ ListClaimsService  = (*stubList)(nil)
	_ DeleteClaimService = (*stubDelete)(nil)
	_ VerifyClaimService = (*stubVerify)(nil)
	_ AuditEmitter       = (*stubAudit)(nil)
)

// TestClaimServiceInterfaces verifies that all exported interface types and
// value types defined in interfaces.go are correctly shaped and satisfy their
// contracts. This test acts as a living contract guard: if any symbol is
// renamed, moved, or has its signature changed the test will fail to compile
// or fail at runtime.
func TestClaimServiceInterfaces(t *testing.T) {
	t.Run("CreateResult zero value", func(t *testing.T) {
		var r CreateResult
		require.Nil(t, r.Claim)
		require.False(t, r.Duplicate)
		require.Empty(t, r.DuplicateOf)
	})

	t.Run("CreateResult fields populated", func(t *testing.T) {
		c := &domain.Claim{ClaimID: "c1", ProfileID: "p1"}
		r := CreateResult{
			Claim:       c,
			Duplicate:   true,
			DuplicateOf: "c0",
		}
		require.Equal(t, c, r.Claim)
		require.True(t, r.Duplicate)
		require.Equal(t, "c0", r.DuplicateOf)
	})

	t.Run("AuditLogEntry zero value", func(t *testing.T) {
		var e AuditLogEntry
		require.Empty(t, e.ProfileID)
		require.Empty(t, e.Operation)
		require.Empty(t, e.EntityType)
		require.Empty(t, e.EntityID)
		require.True(t, e.Timestamp.IsZero())
	})

	t.Run("AuditLogEntry fields populated", func(t *testing.T) {
		now := time.Now().UTC()
		e := AuditLogEntry{
			ProfileID:     "p1",
			Timestamp:     now,
			Operation:     "CREATE",
			EntityType:    "claim",
			EntityID:      "c1",
			BeforePayload: map[string]any{"status": "candidate"},
			AfterPayload:  map[string]any{"status": "validated"},
			ActorKeyID:    "k1",
			ActorRole:     "user",
			ClientIP:      "127.0.0.1",
			CorrelationID: "req-1",
			Metadata:      map[string]any{"pipeline_run_id": "run-1"},
		}
		require.Equal(t, "p1", e.ProfileID)
		require.Equal(t, now, e.Timestamp)
		require.Equal(t, "CREATE", e.Operation)
		require.Equal(t, "claim", e.EntityType)
		require.Equal(t, "c1", e.EntityID)
		require.Equal(t, "candidate", e.BeforePayload["status"])
		require.Equal(t, "validated", e.AfterPayload["status"])
		require.Equal(t, "k1", e.ActorKeyID)
		require.Equal(t, "user", e.ActorRole)
		require.Equal(t, "127.0.0.1", e.ClientIP)
		require.Equal(t, "req-1", e.CorrelationID)
		require.Equal(t, "run-1", e.Metadata["pipeline_run_id"])
	})

	t.Run("stub implementations satisfy interfaces at runtime", func(t *testing.T) {
		ctx := context.Background()

		// CreateClaimService
		var cs CreateClaimService = &stubCreate{}
		res, err := cs.Create(ctx, "p1", &domain.Claim{})
		require.NoError(t, err)
		require.Nil(t, res)

		// GetClaimService
		var gs GetClaimService = &stubGet{}
		claim, err := gs.Get(ctx, "p1", "c1")
		require.NoError(t, err)
		require.Nil(t, claim)

		// ListClaimsService
		var ls ListClaimsService = &stubList{}
		claims, total, err := ls.List(ctx, "p1", 10, 0)
		require.NoError(t, err)
		require.Nil(t, claims)
		require.Equal(t, 0, total)

		// DeleteClaimService
		var ds DeleteClaimService = &stubDelete{}
		require.NoError(t, ds.Delete(ctx, "p1", "c1"))

		// VerifyClaimService
		var vs VerifyClaimService = &stubVerify{}
		verified, err := vs.Verify(ctx, "p1", "c1")
		require.NoError(t, err)
		require.Nil(t, verified)

		// AuditEmitter
		var ae AuditEmitter = &stubAudit{}
		require.NoError(t, ae.Append(ctx, AuditLogEntry{Operation: "CREATE"}))
	})
}

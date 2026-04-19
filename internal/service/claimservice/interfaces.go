// Package claimservice provides claim creation, retrieval, verification, and
// lifecycle management for the knowledge-pipeline claim stage.
//
// Profile isolation invariant: every method on every service interface in this
// package receives profileID as an explicit parameter. Implementations MUST
// use that value to scope all database queries — no cross-profile reads or
// writes are permitted.
package claimservice

import (
	"context"
	"time"

	"github.com/dense-mem/dense-mem/internal/domain"
)

// CreateResult contains the outcome of a claim creation operation.
//
// When Duplicate is true the caller received an existing claim (matched via
// idempotency key or content hash) and no new node was written to the graph.
// DuplicateOf holds the ClaimID of the existing claim in that case.
type CreateResult struct {
	Claim       *domain.Claim
	Duplicate   bool   // true when returned via idempotency/content-hash dedupe
	DuplicateOf string // ClaimID of the existing match when Duplicate == true
}

// CreateClaimService defines the interface for claim creation.
//
// Implementations are responsible for embedding generation (if required),
// persistence to the graph, deduplication, and audit emission.
type CreateClaimService interface {
	// Create persists a new claim scoped to profileID.
	// Returns CreateResult with Duplicate=true when the claim already exists.
	Create(ctx context.Context, profileID string, claim *domain.Claim) (*CreateResult, error)
}

// GetClaimService defines the interface for claim retrieval by ID.
type GetClaimService interface {
	// Get retrieves the claim identified by claimID within profileID.
	// Returns a not-found error when the claim does not exist or belongs to a
	// different profile.
	Get(ctx context.Context, profileID string, claimID string) (*domain.Claim, error)
}

// ListClaimsService defines the interface for paginated claim listing.
type ListClaimsService interface {
	// List returns up to limit claims for profileID starting at offset, plus
	// the total count of matching claims for pagination metadata.
	List(ctx context.Context, profileID string, limit, offset int) ([]*domain.Claim, int, error)
}

// DeleteClaimService defines the interface for claim deletion.
type DeleteClaimService interface {
	// Delete permanently removes the claim identified by claimID from the graph.
	// Callers must supply the owning profileID for isolation enforcement.
	Delete(ctx context.Context, profileID string, claimID string) error
}

// VerifyClaimService defines the interface for claim entailment verification.
//
// Verification runs an entailment check against the supporting source
// fragments and transitions the claim status (candidate → validated |
// rejected). The updated claim is returned after the status transition.
type VerifyClaimService interface {
	// Verify runs entailment verification for claimID within profileID and
	// returns the claim with its updated status and verdict fields populated.
	Verify(ctx context.Context, profileID string, claimID string) (*domain.Claim, error)
}

// AuditLogEntry is a local representation of an audit event emitted by the
// claim service layer. It is defined here rather than imported from the
// top-level service package to prevent an import cycle.
//
// Fields mirror the audit_log table schema. Nullable columns are represented
// as pointer types so callers can distinguish absent values from zero values.
type AuditLogEntry struct {
	ProfileID     string
	Timestamp     time.Time
	Operation     string
	EntityType    string
	EntityID      string
	BeforePayload map[string]any
	AfterPayload  map[string]any
	ActorKeyID    string
	ActorRole     string
	ClientIP      string
	CorrelationID string
	Metadata      map[string]any
}

// AuditEmitter defines the minimal interface for emitting claim-related audit
// events. Defined locally to avoid an import cycle with the top-level service
// package.
//
// Implementations must be safe for concurrent use and must not log secrets.
type AuditEmitter interface {
	// Append writes a single audit entry. Implementations must not return an
	// error that causes a claim operation to fail — audit failures should be
	// logged and swallowed so the primary operation succeeds.
	Append(ctx context.Context, entry AuditLogEntry) error
}

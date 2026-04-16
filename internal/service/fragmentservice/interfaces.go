// Package fragmentservice provides fragment creation and management services.
package fragmentservice

import (
	"context"

	"github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/domain"
)

// CreateResult contains the result of a fragment creation operation.
type CreateResult struct {
	Fragment    *domain.Fragment
	Duplicate   bool   // true when returned via idempotency/content-hash dedupe
	DuplicateOf string // fragment id of existing match when Duplicate == true
}

// CreateFragmentService defines the interface for fragment creation.
// Implementations must handle embedding generation, persistence, deduplication, and audit.
type CreateFragmentService interface {
	// Create creates a new fragment with server-side embedding.
	// Returns CreateResult with Duplicate=true if the fragment already exists
	// (via idempotency key or content hash deduplication).
	// Returns error if embedding generation fails (AC-23: no silent unembedded writes).
	Create(ctx context.Context, profileID string, req *dto.CreateFragmentRequest) (*CreateResult, error)
}
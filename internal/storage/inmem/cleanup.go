package inmem

import (
	"context"
)

// NoopCleanupRepository is a non-nil no-op implementation of cleanup
// operations for profile state and API key sessions. It returns nil
// for all cleanup calls, making it safe to inject in no-Redis mode.
type NoopCleanupRepository struct{}

// NewNoopCleanupRepository creates a new no-op cleanup repository.
func NewNoopCleanupRepository() *NoopCleanupRepository {
	return &NoopCleanupRepository{}
}

// PurgeProfileState is a no-op implementation that returns nil.
func (r *NoopCleanupRepository) PurgeProfileState(ctx context.Context, profileID string) error {
	return nil
}

// InvalidateKeySessions is a no-op implementation that returns nil.
func (r *NoopCleanupRepository) InvalidateKeySessions(ctx context.Context, profileID, keyID string) error {
	return nil
}

// NoopStreamCleanupRepository is a non-nil no-op implementation of
// SSE stream cleanup operations. It returns nil for all cleanup calls.
type NoopStreamCleanupRepository struct{}

// NewNoopStreamCleanupRepository creates a new no-op stream cleanup repository.
func NewNoopStreamCleanupRepository() *NoopStreamCleanupRepository {
	return &NoopStreamCleanupRepository{}
}

// PurgeProfileStreamState is a no-op implementation that returns nil.
func (r *NoopStreamCleanupRepository) PurgeProfileStreamState(ctx context.Context, profileID string) error {
	return nil
}

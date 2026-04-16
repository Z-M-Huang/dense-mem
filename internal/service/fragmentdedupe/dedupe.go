// Package fragmentdedupe provides deduplication lookup for fragments.
package fragmentdedupe

import (
	"context"
	"fmt"
	"time"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/storage/neo4j"
)

// DedupeKey represents the key components for fragment deduplication.
type DedupeKey struct {
	ProfileID      string
	IdempotencyKey string // empty when not provided
	ContentHash    string
}

// DedupeLookup defines the interface for looking up fragments for deduplication.
// Lookups are profile-scoped (AC-21, AC-22).
type DedupeLookup interface {
	// ByIdempotencyKey finds a fragment by its idempotency key within a profile.
	// Returns (nil, nil) on miss; (*domain.Fragment, nil) on hit.
	ByIdempotencyKey(ctx context.Context, profileID, key string) (*domain.Fragment, error)

	// ByContentHash finds a fragment by its content hash within a profile.
	// Returns (nil, nil) on miss; (*domain.Fragment, nil) on hit.
	ByContentHash(ctx context.Context, profileID, hash string) (*domain.Fragment, error)
}

// ScopedReader defines the interface for profile-scoped read operations.
// This mirrors the interface from neo4j package to avoid import cycles.
type ScopedReader interface {
	ScopedRead(ctx context.Context, profileID string, query string, params map[string]any) (any, []map[string]any, error)
}

// neo4jDedupeLookup implements DedupeLookup using Neo4j with profile scoping.
type neo4jDedupeLookup struct {
	reader ScopedReader
}

// Ensure neo4jDedupeLookup implements DedupeLookup
var _ DedupeLookup = (*neo4jDedupeLookup)(nil)

// NewNeo4jDedupeLookup creates a new dedupe lookup using Neo4j.
func NewNeo4jDedupeLookup(reader ScopedReader) DedupeLookup {
	return &neo4jDedupeLookup{
		reader: reader,
	}
}

// ByIdempotencyKey finds a fragment by its idempotency key within a profile.
// The query is profile-scoped to ensure AC-21 compliance.
// Returns (nil, nil) on miss; (*domain.Fragment, nil) on hit.
func (l *neo4jDedupeLookup) ByIdempotencyKey(ctx context.Context, profileID, key string) (*domain.Fragment, error) {
	// Use composite index for O(log n) lookup (from Unit 12)
	// The query must contain $profileId placeholder for ScopedRead validation
	query := `
		MATCH (sf:SourceFragment {profile_id: $profileId, idempotency_key: $key})
		RETURN sf.fragment_id AS fragment_id,
		       sf.profile_id AS profile_id,
		       sf.content AS content,
		       sf.source AS source,
		       sf.source_type AS source_type,
		       sf.labels AS labels,
		       sf.metadata AS metadata,
		       sf.content_hash AS content_hash,
		       sf.idempotency_key AS idempotency_key,
		       sf.embedding_model AS embedding_model,
		       sf.embedding_dimensions AS embedding_dimensions,
		       sf.created_at AS created_at,
		       sf.updated_at AS updated_at
		LIMIT 1
	`
	params := map[string]any{
		"key": key,
	}

	_, results, err := l.reader.ScopedRead(ctx, profileID, query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup fragment by idempotency key: %w", err)
	}

	if len(results) == 0 {
		return nil, nil // miss
	}

	return mapToFragment(results[0]), nil
}

// ByContentHash finds a fragment by its content hash within a profile.
// The query is profile-scoped to ensure AC-22 compliance.
// Returns (nil, nil) on miss; (*domain.Fragment, nil) on hit.
func (l *neo4jDedupeLookup) ByContentHash(ctx context.Context, profileID, hash string) (*domain.Fragment, error) {
	// Use composite index for O(log n) lookup (from Unit 12)
	// The query must contain $profileId placeholder for ScopedRead validation
	query := `
		MATCH (sf:SourceFragment {profile_id: $profileId, content_hash: $hash})
		RETURN sf.fragment_id AS fragment_id,
		       sf.profile_id AS profile_id,
		       sf.content AS content,
		       sf.source AS source,
		       sf.source_type AS source_type,
		       sf.labels AS labels,
		       sf.metadata AS metadata,
		       sf.content_hash AS content_hash,
		       sf.idempotency_key AS idempotency_key,
		       sf.embedding_model AS embedding_model,
		       sf.embedding_dimensions AS embedding_dimensions,
		       sf.created_at AS created_at,
		       sf.updated_at AS updated_at
		LIMIT 1
	`
	params := map[string]any{
		"hash": hash,
	}

	_, results, err := l.reader.ScopedRead(ctx, profileID, query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup fragment by content hash: %w", err)
	}

	if len(results) == 0 {
		return nil, nil // miss
	}

	return mapToFragment(results[0]), nil
}

// mapToFragment converts a Neo4j result map to a domain.Fragment.
func mapToFragment(m map[string]any) *domain.Fragment {
	fragment := &domain.Fragment{}

	if v, ok := m["fragment_id"]; ok {
		fragment.FragmentID, _ = v.(string)
	}
	if v, ok := m["profile_id"]; ok {
		fragment.ProfileID, _ = v.(string)
	}
	if v, ok := m["content"]; ok {
		fragment.Content, _ = v.(string)
	}
	if v, ok := m["source"]; ok {
		fragment.Source, _ = v.(string)
	}
	fragment.SourceType = neo4j.CoerceSourceType(m["source_type"])
	if v, ok := m["labels"]; ok {
		if arr, ok := v.([]any); ok {
			labels := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					labels = append(labels, s)
				}
			}
			fragment.Labels = labels
		}
	}
	if v, ok := m["metadata"]; ok {
		if meta, ok := v.(map[string]any); ok {
			fragment.Metadata = meta
		}
	}
	if v, ok := m["content_hash"]; ok {
		fragment.ContentHash, _ = v.(string)
	}
	if v, ok := m["idempotency_key"]; ok {
		fragment.IdempotencyKey, _ = v.(string)
	}
	if v, ok := m["embedding_model"]; ok {
		fragment.EmbeddingModel, _ = v.(string)
	}
	if v, ok := m["embedding_dimensions"]; ok {
		switch dim := v.(type) {
		case int64:
			fragment.EmbeddingDimensions = int(dim)
		case int:
			fragment.EmbeddingDimensions = dim
		}
	}
	if v, ok := m["created_at"].(time.Time); ok {
		fragment.CreatedAt = v
	}
	if v, ok := m["updated_at"].(time.Time); ok {
		fragment.UpdatedAt = v
	}

	return fragment
}
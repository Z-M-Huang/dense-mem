package fragmentdedupe

import (
	"context"
	"strings"
	"testing"

	neo4jstorage "github.com/dense-mem/dense-mem/internal/storage/neo4j"
)

// TestDedupe_ByIdempotencyKey_ExcludesRetracted verifies that the ByIdempotencyKey
// query contains the shared FragmentActiveFilter so retracted nodes are never
// returned as dedupe hits (AC-44).
func TestDedupe_ByIdempotencyKey_ExcludesRetracted(t *testing.T) {
	reader := &fakeScopedReader{}
	lookup := NewNeo4jDedupeLookup(reader)

	_, _ = lookup.ByIdempotencyKey(context.Background(), "pA", "k1")

	if !strings.Contains(reader.LastQuery, neo4jstorage.FragmentActiveFilter) {
		t.Errorf("ByIdempotencyKey query missing FragmentActiveFilter\ngot query:\n%s\nwant substring: %q",
			reader.LastQuery, neo4jstorage.FragmentActiveFilter)
	}
}

// TestDedupe_ByContentHash_ExcludesRetracted verifies that the ByContentHash
// query contains the shared FragmentActiveFilter so retracted nodes are never
// returned as dedupe hits (AC-44).
func TestDedupe_ByContentHash_ExcludesRetracted(t *testing.T) {
	reader := &fakeScopedReader{}
	lookup := NewNeo4jDedupeLookup(reader)

	_, _ = lookup.ByContentHash(context.Background(), "pA", "h1")

	if !strings.Contains(reader.LastQuery, neo4jstorage.FragmentActiveFilter) {
		t.Errorf("ByContentHash query missing FragmentActiveFilter\ngot query:\n%s\nwant substring: %q",
			reader.LastQuery, neo4jstorage.FragmentActiveFilter)
	}
}

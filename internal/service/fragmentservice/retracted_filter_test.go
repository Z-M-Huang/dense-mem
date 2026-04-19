package fragmentservice

import (
	"context"
	"strings"
	"testing"

	neo4jstorage "github.com/dense-mem/dense-mem/internal/storage/neo4j"
)

// queryScopedReader captures the Cypher query sent to ScopedRead so tests can
// assert that the active-fragment filter is present (AC-44).
type queryScopedReader struct {
	lastQuery string
	results   []map[string]any
	err       error
}

func (q *queryScopedReader) ScopedRead(_ context.Context, _ string, query string, _ map[string]any) (any, []map[string]any, error) {
	q.lastQuery = query
	return nil, q.results, q.err
}

// TestGetByID_ExcludesRetracted verifies that the GetByID query contains the
// shared FragmentActiveFilter so retracted nodes are never returned (AC-44).
func TestGetByID_ExcludesRetracted(t *testing.T) {
	reader := &queryScopedReader{}
	svc := NewGetFragmentService(reader)

	_, _ = svc.GetByID(context.Background(), "pA", "f1")

	if !strings.Contains(reader.lastQuery, neo4jstorage.FragmentActiveFilter) {
		t.Errorf("GetByID query missing FragmentActiveFilter\ngot query:\n%s\nwant substring: %q",
			reader.lastQuery, neo4jstorage.FragmentActiveFilter)
	}
}

// TestList_ExcludesRetracted verifies that the List query contains the shared
// FragmentActiveFilter so retracted nodes are excluded from paginated results (AC-44).
func TestList_ExcludesRetracted(t *testing.T) {
	reader := &queryScopedReader{}
	svc := NewListFragmentsService(reader)

	_, _, _ = svc.List(context.Background(), "pA", ListOptions{})

	if !strings.Contains(reader.lastQuery, neo4jstorage.FragmentActiveFilter) {
		t.Errorf("List query missing FragmentActiveFilter\ngot query:\n%s\nwant substring: %q",
			reader.lastQuery, neo4jstorage.FragmentActiveFilter)
	}
}

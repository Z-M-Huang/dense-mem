package keywordsearch

import (
	"context"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeScopedReader implements ScopedReaderInterface for testing the neo4j searchers directly.
type fakeScopedReader struct {
	rows []map[string]any
}

func (f *fakeScopedReader) ScopedRead(ctx context.Context, profileID string, query string, params map[string]any) (neo4j.ResultSummary, []map[string]any, error) {
	return nil, f.rows, nil
}

// mockFragmentSearcher implements FragmentSearcherInterface for testing.
type mockFragmentSearcher struct {
	searchContentFunc func(ctx context.Context, profileID string, query string, labels []string, limit int) ([]FragmentSearchResult, error)
}

func (m *mockFragmentSearcher) SearchContent(ctx context.Context, profileID string, query string, labels []string, limit int) ([]FragmentSearchResult, error) {
	if m.searchContentFunc != nil {
		return m.searchContentFunc(ctx, profileID, query, labels, limit)
	}
	return []FragmentSearchResult{}, nil
}

// mockFactSearcher implements FactSearcherInterface for testing.
type mockFactSearcher struct {
	searchPredicateFunc func(ctx context.Context, profileID string, query string, labels []string, limit int) ([]FactSearchResult, error)
}

func (m *mockFactSearcher) SearchPredicate(ctx context.Context, profileID string, query string, labels []string, limit int) ([]FactSearchResult, error) {
	if m.searchPredicateFunc != nil {
		return m.searchPredicateFunc(ctx, profileID, query, labels, limit)
	}
	return []FactSearchResult{}, nil
}

// TestKeywordSearchProfileFiltering tests that profile B results contain zero profile A data.
// This verifies defense-in-depth profile filtering works at the Go post-filter level.
func TestKeywordSearchProfileFiltering(t *testing.T) {
	profileA := "profile-a-id"
	profileB := "profile-b-id"

	tests := []struct {
		name                string
		requestingProfile   string
		fragmentResults     []FragmentSearchResult
		factResults         []FactSearchResult
		expectedProfileIDs  []string // All should match requesting profile
	}{
		{
			name:               "profile B sees only profile B fragments and facts",
			requestingProfile:  profileB,
			fragmentResults: []FragmentSearchResult{
				{FragmentID: "frag-1", Content: "content from profile A", Score: 0.9, ProfileID: profileA},
				{FragmentID: "frag-2", Content: "content from profile B", Score: 0.8, ProfileID: profileB},
				{FragmentID: "frag-3", Content: "more from profile A", Score: 0.95, ProfileID: profileA},
			},
			factResults: []FactSearchResult{
				{FactID: "fact-1", Predicate: "fact from profile A", Score: 0.7, ProfileID: profileA},
				{FactID: "fact-2", Predicate: "fact from profile B", Score: 0.6, ProfileID: profileB},
			},
			expectedProfileIDs: []string{profileB, profileB}, // Only profile B results
		},
		{
			name:               "profile A sees only profile A fragments and facts",
			requestingProfile:  profileA,
			fragmentResults: []FragmentSearchResult{
				{FragmentID: "frag-1", Content: "content from profile A", Score: 0.9, ProfileID: profileA},
				{FragmentID: "frag-2", Content: "content from profile B", Score: 0.95, ProfileID: profileB},
			},
			factResults: []FactSearchResult{
				{FactID: "fact-1", Predicate: "fact from profile A", Score: 0.7, ProfileID: profileA},
				{FactID: "fact-2", Predicate: "fact from profile B", Score: 0.8, ProfileID: profileB},
				{FactID: "fact-3", Predicate: "more from profile A", Score: 0.6, ProfileID: profileA},
			},
			expectedProfileIDs: []string{profileA, profileA, profileA}, // Only profile A results
		},
		{
			name:               "all results from other profile - empty result",
			requestingProfile:  profileB,
			fragmentResults: []FragmentSearchResult{
				{FragmentID: "frag-1", Content: "content from profile A", Score: 0.9, ProfileID: profileA},
				{FragmentID: "frag-2", Content: "more from profile A", Score: 0.95, ProfileID: profileA},
			},
			factResults: []FactSearchResult{
				{FactID: "fact-1", Predicate: "fact from profile A", Score: 0.7, ProfileID: profileA},
			},
			expectedProfileIDs: []string{}, // No results for profile B
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			mockFragment := &mockFragmentSearcher{
				searchContentFunc: func(ctx context.Context, pid string, query string, labels []string, limit int) ([]FragmentSearchResult, error) {
					// Return all results regardless of profile (simulating Cypher filter bypass attempt)
					return tt.fragmentResults, nil
				},
			}

			mockFact := &mockFactSearcher{
				searchPredicateFunc: func(ctx context.Context, pid string, query string, labels []string, limit int) ([]FactSearchResult, error) {
					// Return all results regardless of profile (simulating Cypher filter bypass attempt)
					return tt.factResults, nil
				},
			}

			svc := NewKeywordSearchService(mockFragment, mockFact)

			req := &KeywordSearchRequest{
				Query: "test query",
				Limit: 10,
			}

			result, err := svc.Search(ctx, tt.requestingProfile, req)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify all results belong to the requesting profile (defense-in-depth post-filter)
			for _, hit := range result.Data {
				assert.Equal(t, tt.requestingProfile, hit.ProfileID, "result should belong to requesting profile")
			}

			// Verify expected count
			assert.Len(t, result.Data, len(tt.expectedProfileIDs), "result count should match expected")
		})
	}
}

// TestKeywordSearchLimitCap tests that limit capping applies at 100 with meta.limit_applied set.
func TestKeywordSearchLimitCap(t *testing.T) {
	profileID := "test-profile-id"

	tests := []struct {
		name              string
		requestLimit      int
		expectedLimitCap  int
		expect422         bool
	}{
		{
			name:             "limit 0 returns 422 validation error",
			requestLimit:     0,
			expect422:        true,
		},
		{
			name:             "limit 50 is not capped",
			requestLimit:     50,
			expectedLimitCap: 50,
		},
		{
			name:             "limit 100 is not capped (at max)",
			requestLimit:     100,
			expectedLimitCap: 100,
		},
		{
			name:             "limit 150 capped to 100",
			requestLimit:     150,
			expectedLimitCap: 100,
		},
		{
			name:             "limit 1000 capped to 100",
			requestLimit:     1000,
			expectedLimitCap: 100,
		},
		{
			name:             "default limit when negative",
			requestLimit:     -1,
			expectedLimitCap: DefaultLimit, // 20
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			mockFragment := &mockFragmentSearcher{
				searchContentFunc: func(ctx context.Context, pid string, query string, labels []string, limit int) ([]FragmentSearchResult, error) {
					// Generate more results than limit to test capping
					results := make([]FragmentSearchResult, 200)
					for i := 0; i < 200; i++ {
						results[i] = FragmentSearchResult{
							FragmentID: "frag-" + string(rune(i)),
							Content:    "content",
							Score:      float64(200 - i) / 200.0, // Descending scores
							ProfileID:  pid,
						}
					}
					return results, nil
				},
			}

			mockFact := &mockFactSearcher{
				searchPredicateFunc: func(ctx context.Context, pid string, query string, labels []string, limit int) ([]FactSearchResult, error) {
					return []FactSearchResult{}, nil
				},
			}

			svc := NewKeywordSearchService(mockFragment, mockFact)

			req := &KeywordSearchRequest{
				Query: "test query",
				Limit: tt.requestLimit,
			}

			result, err := svc.Search(ctx, profileID, req)

			if tt.expect422 {
				require.Error(t, err)
				assert.True(t, IsValidationError(err), "expected validation error for limit 0")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify limit_applied in meta
			assert.Equal(t, tt.expectedLimitCap, result.Meta.LimitApplied, "limit_applied should match expected cap")

			// Verify results are capped
			assert.LessOrEqual(t, len(result.Data), tt.expectedLimitCap, "result count should not exceed limit cap")
		})
	}
}

// TestKeywordSearchEmptyResult tests that empty result set returns 200 with {"data":[]}.
func TestKeywordSearchEmptyResult(t *testing.T) {
	profileID := "test-profile-id"

	tests := []struct {
		name            string
		fragmentResults []FragmentSearchResult
		factResults     []FactSearchResult
	}{
		{
			name:            "no results from both indexes",
			fragmentResults: []FragmentSearchResult{},
			factResults:     []FactSearchResult{},
		},
		{
			name:            "results filtered out by profile mismatch",
			fragmentResults: []FragmentSearchResult{
				{FragmentID: "frag-1", Content: "content from other profile", Score: 0.9, ProfileID: "other-profile"},
			},
			factResults: []FactSearchResult{
				{FactID: "fact-1", Predicate: "fact from other profile", Score: 0.7, ProfileID: "other-profile"},
			},
		},
		{
			name:            "results filtered out by labels mismatch",
			fragmentResults: []FragmentSearchResult{
				{FragmentID: "frag-1", Content: "content without matching labels", Score: 0.9, ProfileID: profileID, Labels: []string{"label-a"}},
			},
			factResults: []FactSearchResult{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			mockFragment := &mockFragmentSearcher{
				searchContentFunc: func(ctx context.Context, pid string, query string, labels []string, limit int) ([]FragmentSearchResult, error) {
					return tt.fragmentResults, nil
				},
			}

			mockFact := &mockFactSearcher{
				searchPredicateFunc: func(ctx context.Context, pid string, query string, labels []string, limit int) ([]FactSearchResult, error) {
					return tt.factResults, nil
				},
			}

			svc := NewKeywordSearchService(mockFragment, mockFact)

			req := &KeywordSearchRequest{
				Query:  "test query",
				Limit:  20,
				Labels: []string{"required-label"}, // For the labels mismatch test
			}

			result, err := svc.Search(ctx, profileID, req)
			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify empty data array
			assert.Empty(t, result.Data, "data should be empty array")
			assert.Equal(t, []SearchHit{}, result.Data, "data should be empty array, not nil")

			// Verify meta is set correctly
			assert.Equal(t, 20, result.Meta.LimitApplied, "limit_applied should be set")
		})
	}
}
// TestFragmentSearcher_ScorePropagated tests that BM25 scores are propagated from the fulltext search.
func TestFragmentSearcher_ScorePropagated(t *testing.T) {
	reader := &fakeScopedReader{rows: []map[string]any{
		{"fragment_id": "f1", "content": "hello", "score": 0.87, "profile_id": "p"},
		{"fragment_id": "f2", "content": "world", "score": 0.42, "profile_id": "p"},
	}}
	s := NewFragmentSearcher(reader)
	got, err := s.SearchContent(context.Background(), "p", "hello", nil, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.InDelta(t, 0.87, got[0].Score, 1e-6)
	assert.InDelta(t, 0.42, got[1].Score, 1e-6)
	assert.NotEqual(t, 1.0, got[0].Score, "score must not be hardcoded")
}

// TestFactSearcher_ScorePropagated tests that BM25 scores are propagated from the fulltext search.
func TestFactSearcher_ScorePropagated(t *testing.T) {
	reader := &fakeScopedReader{rows: []map[string]any{
		{"fact_id": "fact-1", "predicate": "knows", "score": 0.33, "profile_id": "p"},
		{"fact_id": "fact-2", "predicate": "likes", "score": 0.55, "profile_id": "p"},
	}}
	s := NewFactSearcher(reader)
	got, err := s.SearchPredicate(context.Background(), "p", "knows", nil, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.InDelta(t, 0.33, got[0].Score, 1e-6)
	assert.InDelta(t, 0.55, got[1].Score, 1e-6)
	assert.NotEqual(t, 1.0, got[0].Score, "score must not be hardcoded")
}

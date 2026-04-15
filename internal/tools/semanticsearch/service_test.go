package semanticsearch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEmbeddingSearcher implements EmbeddingSearcherInterface for testing.
type mockEmbeddingSearcher struct {
	queryVectorIndexFunc func(ctx context.Context, profileID string, embedding []float32, limit int) ([]SearchHit, error)
}

func (m *mockEmbeddingSearcher) QueryVectorIndex(ctx context.Context, profileID string, embedding []float32, limit int) ([]SearchHit, error) {
	if m.queryVectorIndexFunc != nil {
		return m.queryVectorIndexFunc(ctx, profileID, embedding, limit)
	}
	return []SearchHit{}, nil
}

// TestSemanticSearchProfileFiltering tests that profile B vector search excludes profile A vectors
// even when they are nearest globally.
// This verifies defense-in-depth profile filtering works at the Go post-filter level.
func TestSemanticSearchProfileFiltering(t *testing.T) {
	profileA := "profile-a-id"
	profileB := "profile-b-id"
	embeddingDimensions := 1536

	// Create a valid embedding for testing
	validEmbedding := make([]float32, embeddingDimensions)
	for i := range validEmbedding {
		validEmbedding[i] = 0.1
	}

	tests := []struct {
		name                string
		requestingProfile   string
		vectorResults       []SearchHit
		expectedProfileIDs  []string // All should match requesting profile
	}{
		{
			name:               "profile B sees only profile B fragments even when profile A has nearest vector",
			requestingProfile:  profileB,
			vectorResults: []SearchHit{
				{ID: "frag-1", Type: "fragment", Content: "nearest globally from profile A", Score: 0.99, ProfileID: profileA},
				{ID: "frag-2", Type: "fragment", Content: "content from profile B", Score: 0.80, ProfileID: profileB},
				{ID: "frag-3", Type: "fragment", Content: "second nearest from profile A", Score: 0.95, ProfileID: profileA},
			},
			expectedProfileIDs: []string{profileB}, // Only profile B results, even though profile A has higher scores
		},
		{
			name:               "profile A sees only profile A fragments even when profile B has nearest vector",
			requestingProfile:  profileA,
			vectorResults: []SearchHit{
				{ID: "frag-1", Type: "fragment", Content: "nearest globally from profile B", Score: 0.99, ProfileID: profileB},
				{ID: "frag-2", Type: "fragment", Content: "content from profile A", Score: 0.80, ProfileID: profileA},
				{ID: "frag-3", Type: "fragment", Content: "second from profile A", Score: 0.75, ProfileID: profileA},
			},
			expectedProfileIDs: []string{profileA, profileA}, // Only profile A results
		},
		{
			name:               "all results from other profile - empty result",
			requestingProfile:  profileB,
			vectorResults: []SearchHit{
				{ID: "frag-1", Type: "fragment", Content: "nearest from profile A", Score: 0.99, ProfileID: profileA},
				{ID: "frag-2", Type: "fragment", Content: "second from profile A", Score: 0.95, ProfileID: profileA},
			},
			expectedProfileIDs: []string{}, // No results for profile B
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			mockSearcher := &mockEmbeddingSearcher{
				queryVectorIndexFunc: func(ctx context.Context, pid string, emb []float32, limit int) ([]SearchHit, error) {
					// Return all results regardless of profile (simulating Cypher filter bypass attempt)
					return tt.vectorResults, nil
				},
			}

			svc := NewSemanticSearchService(mockSearcher, embeddingDimensions)

			req := &SemanticSearchRequest{
				Embedding: validEmbedding,
				Limit:     10,
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

// TestSemanticSearchBadDimensions tests that wrong embedding dimension returns validation error.
func TestSemanticSearchBadDimensions(t *testing.T) {
	profileID := "test-profile-id"
	embeddingDimensions := 1536

	mockSearcher := &mockEmbeddingSearcher{}
	svc := NewSemanticSearchService(mockSearcher, embeddingDimensions)

	tests := []struct {
		name           string
		embeddingLen   int
		expectError    bool
	}{
		{
			name:         "correct dimensions",
			embeddingLen: 1536,
			expectError:  false,
		},
		{
			name:         "too few dimensions",
			embeddingLen: 512,
			expectError:  true,
		},
		{
			name:         "too many dimensions",
			embeddingLen: 2048,
			expectError:  true,
		},
		{
			name:         "empty embedding",
			embeddingLen: 0,
			expectError:  true, // This is the 501 case, not dimension mismatch
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			embedding := make([]float32, tt.embeddingLen)
			for i := range embedding {
				embedding[i] = 0.1
			}

			req := &SemanticSearchRequest{
				Embedding: embedding,
				Limit:     10,
			}

			result, err := svc.Search(ctx, profileID, req)

			if tt.expectError {
				require.Error(t, err)
				if tt.embeddingLen == 0 {
					// Empty embedding returns 501 error
					assert.True(t, IsEmbeddingGenerationNotConfiguredError(err), "expected embedding generation not configured error for empty embedding")
				} else {
					// Wrong dimensions returns dimension mismatch error
					assert.True(t, IsDimensionMismatchError(err), "expected dimension mismatch error")
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
		})
	}
}

// TestSemanticSearchMissingEmbedding501 tests that absent embedding returns exactly 501.
func TestSemanticSearchMissingEmbedding501(t *testing.T) {
	profileID := "test-profile-id"
	embeddingDimensions := 1536

	mockSearcher := &mockEmbeddingSearcher{}
	svc := NewSemanticSearchService(mockSearcher, embeddingDimensions)

	ctx := context.Background()

	// Empty embedding (nil or zero-length)
	req := &SemanticSearchRequest{
		Embedding: []float32{}, // Empty embedding
		Limit:     10,
	}

	result, err := svc.Search(ctx, profileID, req)

	require.Error(t, err)
	require.Nil(t, result)

	// Verify it's exactly EmbeddingGenerationNotConfiguredError (501 case)
	assert.True(t, IsEmbeddingGenerationNotConfiguredError(err), "expected EmbeddingGenerationNotConfiguredError")
	assert.Contains(t, err.Error(), "embedding generation not configured")
}

// TestSemanticSearchLimitValidation tests limit validation and capping.
func TestSemanticSearchLimitValidation(t *testing.T) {
	profileID := "test-profile-id"
	embeddingDimensions := 1536

	validEmbedding := make([]float32, embeddingDimensions)
	for i := range validEmbedding {
		validEmbedding[i] = 0.1
	}

	tests := []struct {
		name              string
		requestLimit      int
		expectedLimitCap  int
		expectError       bool
	}{
		{
			name:         "limit 0 returns 422 validation error",
			requestLimit: 0,
			expectError:  true,
		},
		{
			name:             "limit 10 is not capped",
			requestLimit:     10,
			expectedLimitCap: 10,
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
			name:             "default limit when not specified",
			requestLimit:     -1, // Negative means use default
			expectedLimitCap: DefaultLimit, // 10
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			mockSearcher := &mockEmbeddingSearcher{
				queryVectorIndexFunc: func(ctx context.Context, pid string, emb []float32, limit int) ([]SearchHit, error) {
					// Return results with the requesting profile to pass post-filter
					results := make([]SearchHit, 200)
					for i := 0; i < 200; i++ {
						results[i] = SearchHit{
							ID:        "frag-" + string(rune(i)),
							Type:      "fragment",
							Content:   "content",
							Score:     float64(200 - i) / 200.0, // Descending scores
							ProfileID: pid,
						}
					}
					return results, nil
				},
			}

			svc := NewSemanticSearchService(mockSearcher, embeddingDimensions)

			req := &SemanticSearchRequest{
				Embedding: validEmbedding,
				Limit:     tt.requestLimit,
			}

			result, err := svc.Search(ctx, profileID, req)

			if tt.expectError {
				require.Error(t, err)
				assert.True(t, IsValidationError(err), "expected validation error")
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

// TestSemanticSearchEmptyResult tests that empty result set returns 200 with {"data":[]}.
func TestSemanticSearchEmptyResult(t *testing.T) {
	profileID := "test-profile-id"
	embeddingDimensions := 1536

	validEmbedding := make([]float32, embeddingDimensions)
	for i := range validEmbedding {
		validEmbedding[i] = 0.1
	}

	tests := []struct {
		name          string
		vectorResults []SearchHit
	}{
		{
			name:          "no results from vector index",
			vectorResults: []SearchHit{},
		},
		{
			name: "results filtered out by profile mismatch",
			vectorResults: []SearchHit{
				{ID: "frag-1", Type: "fragment", Content: "content from other profile", Score: 0.9, ProfileID: "other-profile"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			mockSearcher := &mockEmbeddingSearcher{
				queryVectorIndexFunc: func(ctx context.Context, pid string, emb []float32, limit int) ([]SearchHit, error) {
					return tt.vectorResults, nil
				},
			}

			svc := NewSemanticSearchService(mockSearcher, embeddingDimensions)

			req := &SemanticSearchRequest{
				Embedding: validEmbedding,
				Limit:     20,
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
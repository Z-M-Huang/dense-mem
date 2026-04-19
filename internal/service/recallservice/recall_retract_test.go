package recallservice

import (
	"context"
	"testing"

	"github.com/dense-mem/dense-mem/internal/tools/keywordsearch"
	"github.com/dense-mem/dense-mem/internal/tools/semanticsearch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecallService_SkipsRetractedFragment verifies that a fragment whose
// hydration returns "not found" (the signal for retraction after AC-44) is
// silently skipped rather than failing the whole Recall call.
// The active fragments must still be returned.
func TestRecallService_SkipsRetractedFragment(t *testing.T) {
	sem := &fakeSemanticSearcher{
		hits: []semanticsearch.SearchHit{
			{ID: "f-active", Type: "fragment"},
			{ID: "f-retracted", Type: "fragment"},
		},
	}
	kw := &fakeKeywordSearcher{
		hits: []keywordsearch.FragmentSearchResult{
			{FragmentID: "f-active"},
		},
	}
	// fakeHydrator returns an error for "f-retracted", simulating a retracted
	// fragment whose GetByID now returns not-found (unit 47 behaviour).
	hydrator := &fakeHydrator{
		missIDs: map[string]bool{"f-retracted": true},
	}
	emb := &stubEmbedding{DimensionsResult: 4}
	svc := NewRecallService(emb, sem, kw, hydrator, nil, nil)

	out, err := svc.Recall(context.Background(), "pA", RecallRequest{Query: "q", Limit: 10})
	require.NoError(t, err, "Recall must succeed even when a retracted fragment is skipped")

	// f-retracted must not appear in the output.
	for _, h := range out {
		assert.NotEqual(t, "f-retracted", h.Fragment.FragmentID,
			"retracted fragment must not be present in recall output (AC-44)")
	}

	// f-active must be present.
	found := false
	for _, h := range out {
		if h.Fragment.FragmentID == "f-active" {
			found = true
		}
	}
	assert.True(t, found, "active fragment must be present in recall output")
}

// TestRecallService_AllRetractedReturnsEmpty verifies that when every merged
// candidate is retracted (hydration miss), Recall returns an empty slice
// rather than an error.
func TestRecallService_AllRetractedReturnsEmpty(t *testing.T) {
	sem := &fakeSemanticSearcher{
		hits: []semanticsearch.SearchHit{
			{ID: "f-gone-1", Type: "fragment"},
			{ID: "f-gone-2", Type: "fragment"},
		},
	}
	kw := &fakeKeywordSearcher{}
	hydrator := &fakeHydrator{
		missIDs: map[string]bool{
			"f-gone-1": true,
			"f-gone-2": true,
		},
	}
	emb := &stubEmbedding{DimensionsResult: 4}
	svc := NewRecallService(emb, sem, kw, hydrator, nil, nil)

	out, err := svc.Recall(context.Background(), "pA", RecallRequest{Query: "q", Limit: 10})
	require.NoError(t, err, "Recall must not error when all candidates are retracted")
	assert.Empty(t, out, "output must be empty when all candidates are retracted")
}

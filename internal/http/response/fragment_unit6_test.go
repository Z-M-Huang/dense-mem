package response_test

import (
	"testing"
	"time"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/http/response"
	"github.com/stretchr/testify/require"
)

// TestFragmentSourceQualityAndClassification is the red-test gate for Unit 6.
// It verifies that:
//   - domain.Fragment carries SourceQuality and Classification fields
//   - ToFragmentResponse maps them into FragmentResponse
//   - FragmentResponse exposes both "id" (backward compat) and "fragment_id" (UAT-02/UAT-03)
func TestFragmentSourceQualityAndClassification(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	frag := &domain.Fragment{
		FragmentID:     "frag-abc",
		ProfileID:      "prof-1",
		Content:        "hello world",
		SourceType:     domain.SourceTypeDocument,
		SourceQuality:  0.85,
		Classification: map[string]any{"topic": "testing", "lang": "go"},
		ContentHash:    "deadbeef",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	resp := response.ToFragmentResponse(frag)

	require.NotNil(t, resp)

	// backward-compat "id" alias
	require.Equal(t, "frag-abc", resp.ID)

	// UAT-02/UAT-03 "fragment_id" alias — must equal ID
	require.Equal(t, "frag-abc", resp.FragmentID)

	// source_quality propagated
	require.InDelta(t, 0.85, resp.SourceQuality, 1e-9)

	// classification propagated
	require.Equal(t, map[string]any{"topic": "testing", "lang": "go"}, resp.Classification)

	// nil fragment guard
	require.Nil(t, response.ToFragmentResponse(nil))
}

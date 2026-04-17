package recallservice

import "context"

// stubEmbedding is a test-only implementation of the local EmbeddingProvider
// interface defined in recall.go. Fields mirror the authoritative mock in
// internal/embedding/mock_test.go so recall_test.go's construction syntax stays
// identical after the rename.
type stubEmbedding struct {
	EmbedFunc func(ctx context.Context, text string) ([]float32, string, error)

	DimensionsResult int
	ModelNameResult  string
}

var _ EmbeddingProvider = (*stubEmbedding)(nil)

func (s *stubEmbedding) Embed(ctx context.Context, text string) ([]float32, string, error) {
	if s.EmbedFunc != nil {
		return s.EmbedFunc(ctx, text)
	}
	return make([]float32, s.DimensionsResult), s.ModelNameResult, nil
}

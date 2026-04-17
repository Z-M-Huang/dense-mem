package fragmentservice

import (
	"context"
	"sync"

	"github.com/dense-mem/dense-mem/internal/embedding"
)

// stubEmbedding is a test-only implementation of embedding.EmbeddingProviderInterface.
// It mirrors the authoritative mock in internal/embedding/mock_test.go so this
// package's tests can inject a controllable embedding provider without a
// cross-package _test.go import (which Go forbids).
type stubEmbedding struct {
	mu sync.Mutex

	EmbedFunc      func(ctx context.Context, text string) ([]float32, string, error)
	EmbedBatchFunc func(ctx context.Context, texts []string) ([][]float32, string, error)

	DimensionsResult  int
	ModelNameResult   string
	IsAvailableResult bool

	CallCount int
	LastText  string
}

var _ embedding.EmbeddingProviderInterface = (*stubEmbedding)(nil)

func (s *stubEmbedding) Embed(ctx context.Context, text string) ([]float32, string, error) {
	s.mu.Lock()
	s.CallCount++
	s.LastText = text
	s.mu.Unlock()

	if s.EmbedFunc != nil {
		return s.EmbedFunc(ctx, text)
	}
	return make([]float32, s.DimensionsResult), s.ModelNameResult, nil
}

func (s *stubEmbedding) EmbedBatch(ctx context.Context, texts []string) ([][]float32, string, error) {
	if s.EmbedBatchFunc != nil {
		return s.EmbedBatchFunc(ctx, texts)
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, s.DimensionsResult)
	}
	return out, s.ModelNameResult, nil
}

func (s *stubEmbedding) ModelName() string { return s.ModelNameResult }
func (s *stubEmbedding) Dimensions() int   { return s.DimensionsResult }
func (s *stubEmbedding) IsAvailable() bool { return s.IsAvailableResult }

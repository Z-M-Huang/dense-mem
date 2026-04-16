package service

import (
	"context"
	"fmt"

	"github.com/dense-mem/dense-mem/internal/storage/postgres"
)

// EmbeddingConfig represents the configured embedding model parameters.
type EmbeddingConfig interface {
	GetAIEmbeddingModel() string
	GetAIEmbeddingDimensions() int
	IsEmbeddingConfigured() bool
}

// EmbeddingConsistencyService ensures embedding model and dimension consistency.
// It validates that the configured embedding model matches what's stored in the database
// and that returned embedding vectors have the correct dimensions.
type EmbeddingConsistencyService interface {
	// CheckAtStartup compares stored vs configured embedding config.
	// Returns nil if no stored config (pending first-write bootstrap).
	// Returns error if stored config differs from configured config.
	CheckAtStartup(ctx context.Context) error

	// RecordFirstWrite initializes the embedding config on first successful write.
	// This is called when the first embedding is successfully generated.
	RecordFirstWrite(ctx context.Context, model string, dimensions int) error

	// ValidateVectorLength checks that an embedding vector has the expected dimensions.
	// Returns error if the vector length doesn't match configured dimensions.
	ValidateVectorLength(vec []float32) error
}

// embeddingConsistencyServiceImpl implements EmbeddingConsistencyService.
type embeddingConsistencyServiceImpl struct {
	repo postgres.EmbeddingConfigRepository
	cfg  EmbeddingConfig
}

// Ensure embeddingConsistencyServiceImpl implements EmbeddingConsistencyService
var _ EmbeddingConsistencyService = (*embeddingConsistencyServiceImpl)(nil)

// NewEmbeddingConsistencyService creates a new embedding consistency service.
func NewEmbeddingConsistencyService(repo postgres.EmbeddingConfigRepository, cfg EmbeddingConfig) EmbeddingConsistencyService {
	return &embeddingConsistencyServiceImpl{
		repo: repo,
		cfg:  cfg,
	}
}

// CheckAtStartup compares stored vs configured embedding config.
// Returns nil if no stored config (pending first-write bootstrap).
// Returns error if stored config differs from configured config with an actionable message.
func (s *embeddingConsistencyServiceImpl) CheckAtStartup(ctx context.Context) error {
	// If embedding is not configured, skip the check entirely
	if !s.cfg.IsEmbeddingConfigured() {
		return nil
	}

	// Fetch stored config
	record, err := s.repo.GetActive(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch embedding config: %w", err)
	}

	// No stored config means first-write bootstrap is pending
	if record == nil {
		return nil
	}

	// Compare stored vs configured
	configuredModel := s.cfg.GetAIEmbeddingModel()
	configuredDims := s.cfg.GetAIEmbeddingDimensions()

	if record.Model != configuredModel || record.Dimensions != configuredDims {
		return fmt.Errorf("embedding model mismatch: configured=%s/%d, stored=%s/%d. To change models, clear embedding_config and re-embed all fragments",
			configuredModel, configuredDims, record.Model, record.Dimensions)
	}

	return nil
}

// RecordFirstWrite initializes the embedding config on first successful write.
func (s *embeddingConsistencyServiceImpl) RecordFirstWrite(ctx context.Context, model string, dimensions int) error {
	return s.repo.Upsert(ctx, model, dimensions)
}

// ValidateVectorLength checks that an embedding vector has the expected dimensions.
func (s *embeddingConsistencyServiceImpl) ValidateVectorLength(vec []float32) error {
	expected := s.cfg.GetAIEmbeddingDimensions()
	if expected == 0 {
		return fmt.Errorf("embedding dimensions not configured")
	}

	actual := len(vec)
	if actual != expected {
		return fmt.Errorf("embedding vector length mismatch: expected=%d, actual=%d. Ensure the configured embedding dimensions match the model output",
			expected, actual)
	}

	return nil
}
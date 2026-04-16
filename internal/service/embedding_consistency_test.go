package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dense-mem/dense-mem/internal/storage/postgres"
)

// fakeEmbeddingConfigRepo is a test double for EmbeddingConfigRepository
type fakeEmbeddingConfigRepo struct {
	record *postgres.EmbeddingConfigRecord
}

func (f *fakeEmbeddingConfigRepo) GetActive(ctx context.Context) (*postgres.EmbeddingConfigRecord, error) {
	return f.record, nil
}

func (f *fakeEmbeddingConfigRepo) Upsert(ctx context.Context, model string, dimensions int) error {
	f.record = &postgres.EmbeddingConfigRecord{
		Model:      model,
		Dimensions: dimensions,
	}
	return nil
}

// fakeEmbeddingConfig is a test double for EmbeddingConfig
type fakeEmbeddingConfig struct {
	model      string
	dimensions int
	configured bool
}

func (f *fakeEmbeddingConfig) GetAIEmbeddingModel() string {
	return f.model
}

func (f *fakeEmbeddingConfig) GetAIEmbeddingDimensions() int {
	return f.dimensions
}

func (f *fakeEmbeddingConfig) IsEmbeddingConfigured() bool {
	return f.configured
}

// cfgWith creates a fake config with the given model and dimensions
func cfgWith(model string, dimensions int) *fakeEmbeddingConfig {
	return &fakeEmbeddingConfig{
		model:      model,
		dimensions: dimensions,
		configured: true,
	}
}

// TestEmbeddingConsistency_MismatchFailsStartup verifies that a mismatch between
// stored and configured embedding models causes startup to fail.
func TestEmbeddingConsistency_MismatchFailsStartup(t *testing.T) {
	repo := &fakeEmbeddingConfigRepo{record: &postgres.EmbeddingConfigRecord{Model: "m1", Dimensions: 1536}}
	svc := NewEmbeddingConsistencyService(repo, cfgWith("m2", 1536))
	err := svc.CheckAtStartup(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model mismatch")
}

// TestEmbeddingConsistency_FirstWriteInitializesRow verifies that first write
// initializes the embedding config row when no record exists.
func TestEmbeddingConsistency_FirstWriteInitializesRow(t *testing.T) {
	repo := &fakeEmbeddingConfigRepo{record: nil}
	svc := NewEmbeddingConsistencyService(repo, cfgWith("m1", 1536))
	require.NoError(t, svc.CheckAtStartup(context.Background()))
	require.NoError(t, svc.RecordFirstWrite(context.Background(), "m1", 1536))
	assert.Equal(t, "m1", repo.record.Model)
	assert.Equal(t, 1536, repo.record.Dimensions)
}

// TestEmbeddingConsistency_VectorLengthMismatch verifies that wrong-length vectors
// are caught and return an error.
func TestEmbeddingConsistency_VectorLengthMismatch(t *testing.T) {
	svc := NewEmbeddingConsistencyService(nil, cfgWith("m1", 4))
	err := svc.ValidateVectorLength([]float32{1, 2, 3})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mismatch")
}

// TestEmbeddingConsistency_CheckAtStartup_SkipIfNotConfigured verifies that
// the startup check is skipped if embedding is not configured.
func TestEmbeddingConsistency_CheckAtStartup_SkipIfNotConfigured(t *testing.T) {
	repo := &fakeEmbeddingConfigRepo{record: &postgres.EmbeddingConfigRecord{Model: "m1", Dimensions: 1536}}
	cfg := &fakeEmbeddingConfig{model: "m2", dimensions: 1536, configured: false}
	svc := NewEmbeddingConsistencyService(repo, cfg)
	err := svc.CheckAtStartup(context.Background())
	require.NoError(t, err, "Should skip check when embedding not configured")
}

// TestEmbeddingConsistency_CheckAtStartup_MatchSucceeds verifies that matching
// stored and configured values succeed.
func TestEmbeddingConsistency_CheckAtStartup_MatchSucceeds(t *testing.T) {
	repo := &fakeEmbeddingConfigRepo{record: &postgres.EmbeddingConfigRecord{Model: "text-embedding-3-small", Dimensions: 1536}}
	svc := NewEmbeddingConsistencyService(repo, cfgWith("text-embedding-3-small", 1536))
	err := svc.CheckAtStartup(context.Background())
	require.NoError(t, err, "Should succeed when stored and configured match")
}

// TestEmbeddingConsistency_ValidateVectorLength_Success verifies that correct-length
// vectors pass validation.
func TestEmbeddingConsistency_ValidateVectorLength_Success(t *testing.T) {
	svc := NewEmbeddingConsistencyService(nil, cfgWith("m1", 4))
	err := svc.ValidateVectorLength([]float32{1, 2, 3, 4})
	require.NoError(t, err, "Should succeed when vector length matches configured dimensions")
}

// TestEmbeddingConsistency_ValidateVectorLength_NotConfigured verifies that
// validation fails when embedding is not configured.
func TestEmbeddingConsistency_ValidateVectorLength_NotConfigured(t *testing.T) {
	cfg := &fakeEmbeddingConfig{model: "", dimensions: 0, configured: false}
	svc := NewEmbeddingConsistencyService(nil, cfg)
	err := svc.ValidateVectorLength([]float32{1, 2, 3})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}
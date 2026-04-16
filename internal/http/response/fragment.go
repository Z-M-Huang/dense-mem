package response

import (
	"github.com/dense-mem/dense-mem/internal/domain"
	dto "github.com/dense-mem/dense-mem/internal/http/dto"
)

// ToFragmentResponse converts a domain Fragment to a DTO FragmentResponse.
// The embedding vector is deliberately excluded from the response (AC-28).
func ToFragmentResponse(f *domain.Fragment) *dto.FragmentResponse {
	if f == nil {
		return nil
	}

	return &dto.FragmentResponse{
		ID:                  f.FragmentID,
		Content:             f.Content,
		SourceType:          string(f.SourceType),
		Source:              f.Source,
		Labels:              f.Labels,
		Metadata:            f.Metadata,
		ContentHash:         f.ContentHash,
		IdempotencyKey:      f.IdempotencyKey,
		EmbeddingModel:      f.EmbeddingModel,
		EmbeddingDimensions: f.EmbeddingDimensions,
		CreatedAt:           f.CreatedAt,
		UpdatedAt:           f.UpdatedAt,
	}
}
package response

import (
	"github.com/dense-mem/dense-mem/internal/domain"
	dto "github.com/dense-mem/dense-mem/internal/http/dto"
)

// ToFactResponse converts a domain Fact to a DTO FactResponse.
// Returns nil when f is nil.
func ToFactResponse(f *domain.Fact) *dto.FactResponse {
	if f == nil {
		return nil
	}
	return &dto.FactResponse{
		FactID:                       f.FactID,
		ProfileID:                    f.ProfileID,
		Subject:                      f.Subject,
		Predicate:                    f.Predicate,
		Object:                       f.Object,
		Status:                       string(f.Status),
		TruthScore:                   f.TruthScore,
		ValidFrom:                    f.ValidFrom,
		ValidTo:                      f.ValidTo,
		RecordedAt:                   f.RecordedAt,
		RetractedAt:                  f.RetractedAt,
		LastConfirmedAt:              f.LastConfirmedAt,
		PromotedFromClaimID:          f.PromotedFromClaimID,
		Classification:               f.Classification,
		ClassificationLatticeVersion: f.ClassificationLatticeVersion,
		SourceQuality:                f.SourceQuality,
		Labels:                       f.Labels,
		Metadata:                     f.Metadata,
	}
}

// ToListFactsResponse converts a slice of domain Facts to a paginated DTO response.
// nextCursor is the opaque pagination token returned to the caller; pass "" when there are no more pages.
func ToListFactsResponse(facts []*domain.Fact, nextCursor string) *dto.ListFactsResponse {
	items := make([]dto.FactResponse, 0, len(facts))
	for _, f := range facts {
		r := ToFactResponse(f)
		if r != nil {
			items = append(items, *r)
		}
	}
	return &dto.ListFactsResponse{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}
}

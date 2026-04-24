package response

import (
	"github.com/dense-mem/dense-mem/internal/domain"
	dto "github.com/dense-mem/dense-mem/internal/http/dto"
)

// ToCommunityResponse converts a domain Community into its public DTO form.
func ToCommunityResponse(c *domain.Community) *dto.CommunityResponse {
	if c == nil {
		return nil
	}
	return &dto.CommunityResponse{
		CommunityID:      c.CommunityID,
		ProfileID:        c.ProfileID,
		Level:            c.Level,
		Summary:          c.Summary,
		SummaryVersion:   c.SummaryVersion,
		MemberCount:      c.MemberCount,
		TopEntities:      c.TopEntities,
		TopPredicates:    c.TopPredicates,
		LastSummarizedAt: c.LastSummarizedAt,
	}
}

// ToListCommunitiesResponse converts domain communities into the list DTO.
func ToListCommunitiesResponse(communities []*domain.Community) *dto.ListCommunitiesResponse {
	items := make([]dto.CommunityResponse, 0, len(communities))
	for _, community := range communities {
		resp := ToCommunityResponse(community)
		if resp != nil {
			items = append(items, *resp)
		}
	}
	return &dto.ListCommunitiesResponse{
		Items: items,
		Total: len(items),
	}
}

// ToCommunityDetectResponse converts a detect result into the public response DTO.
func ToCommunityDetectResponse(communities []*domain.Community) *dto.CommunityDetectResponse {
	items := make([]dto.CommunityResponse, 0, len(communities))
	nodeCount := 0
	for _, community := range communities {
		resp := ToCommunityResponse(community)
		if resp != nil {
			items = append(items, *resp)
			nodeCount += resp.MemberCount
		}
	}
	return &dto.CommunityDetectResponse{
		Detected:       true,
		CommunityCount: len(items),
		NodeCount:      nodeCount,
		Communities:    items,
	}
}

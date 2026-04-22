package communityservice

import (
	"context"

	"github.com/dense-mem/dense-mem/internal/domain"
)

type getCommunitySummaryServiceImpl struct {
	store communityReadStore
}

type listCommunitiesServiceImpl struct {
	store communityReadStore
}

var _ GetCommunitySummaryService = (*getCommunitySummaryServiceImpl)(nil)
var _ ListCommunitiesService = (*listCommunitiesServiceImpl)(nil)

// NewGetCommunitySummaryService constructs a ready-to-use get service.
func NewGetCommunitySummaryService(client gdsClient) GetCommunitySummaryService {
	return &getCommunitySummaryServiceImpl{store: newNeo4jCommunityStore(client)}
}

// NewListCommunitiesService constructs a ready-to-use list service.
func NewListCommunitiesService(client gdsClient) ListCommunitiesService {
	return &listCommunitiesServiceImpl{store: newNeo4jCommunityStore(client)}
}

func (s *getCommunitySummaryServiceImpl) Get(ctx context.Context, profileID string, communityID string) (*domain.Community, error) {
	return s.store.Get(ctx, profileID, communityID)
}

func (s *listCommunitiesServiceImpl) List(ctx context.Context, profileID string, limit int) ([]*domain.Community, error) {
	return s.store.List(ctx, profileID, limit)
}

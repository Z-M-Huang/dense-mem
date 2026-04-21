package claimservice

import (
	"context"

	"github.com/dense-mem/dense-mem/internal/domain"
)

// listClaimsCompatService adapts the newer cursor-paginated list implementation
// to the older ListClaimsService interface still used by the HTTP handler and
// tool registry wiring.
type listClaimsCompatService struct {
	filtered ListClaimsFilteredService
}

var _ ListClaimsService = (*listClaimsCompatService)(nil)

// NewListClaimsService constructs the compatibility list service over the
// existing keyset-paginated implementation.
func NewListClaimsService(reader claimReader) ListClaimsService {
	return &listClaimsCompatService{
		filtered: NewListClaimsFilteredService(reader),
	}
}

// List adapts limit+offset pagination onto the keyset-backed implementation.
// The returned total is exact when the result set is exhausted and
// "offset+len(items)+1" when there is at least one more page, which is enough
// for the caller's has-more check.
func (s *listClaimsCompatService) List(ctx context.Context, profileID string, limit, offset int) ([]*domain.Claim, int, error) {
	cursor := ""
	skipped := 0

	for skipped < offset {
		chunk := offset - skipped
		if chunk > maxClaimListLimit {
			chunk = maxClaimListLimit
		}

		page, err := s.filtered.List(ctx, profileID, ListClaimOptions{
			Limit:  chunk,
			Cursor: cursor,
		})
		if err != nil {
			return nil, 0, err
		}
		if len(page.Items) == 0 {
			return []*domain.Claim{}, skipped, nil
		}

		skipped += len(page.Items)
		cursor = page.NextCursor
		if !page.HasMore {
			return []*domain.Claim{}, skipped, nil
		}
	}

	page, err := s.filtered.List(ctx, profileID, ListClaimOptions{
		Limit:  limit,
		Cursor: cursor,
	})
	if err != nil {
		return nil, 0, err
	}

	total := skipped + len(page.Items)
	if page.HasMore {
		total++
	}

	return page.Items, total, nil
}

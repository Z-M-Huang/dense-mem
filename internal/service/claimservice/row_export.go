package claimservice

import "github.com/dense-mem/dense-mem/internal/domain"

// RowToClaimForExternalUse exposes the shared Neo4j row-to-domain mapping to
// adjacent infrastructure packages that need to hydrate claims without
// re-implementing the conversion logic.
func RowToClaimForExternalUse(profileID string, row map[string]any) *domain.Claim {
	return rowToClaim(profileID, row)
}

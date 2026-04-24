package neo4j

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Edition describes the connected Neo4j edition reported by dbms.components().
type Edition string

const (
	EditionUnknown    Edition = "unknown"
	EditionCommunity  Edition = "community"
	EditionEnterprise Edition = "enterprise"
)

// DetectEdition probes dbms.components() and normalizes the reported edition.
func DetectEdition(ctx context.Context, client Neo4jClientInterface) (Edition, error) {
	raw, err := client.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, runErr := tx.Run(ctx,
			"CALL dbms.components() YIELD edition RETURN edition LIMIT 1",
			nil,
		)
		if runErr != nil {
			return nil, runErr
		}
		if !result.Next(ctx) {
			if err := result.Err(); err != nil {
				return nil, err
			}
			return "", nil
		}
		record := result.Record()
		if record == nil {
			return "", nil
		}
		edition, _ := record.Get("edition")
		return edition, nil
	})
	if err != nil {
		return EditionUnknown, fmt.Errorf("neo4j edition probe failed: %w", err)
	}

	edition := normalizeEdition(fmt.Sprintf("%v", raw))
	if edition == EditionUnknown {
		return EditionUnknown, fmt.Errorf("neo4j edition probe returned %q", fmt.Sprintf("%v", raw))
	}
	return edition, nil
}

func normalizeEdition(raw string) Edition {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(EditionCommunity):
		return EditionCommunity
	case string(EditionEnterprise):
		return EditionEnterprise
	default:
		return EditionUnknown
	}
}

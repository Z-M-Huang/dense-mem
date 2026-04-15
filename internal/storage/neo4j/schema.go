package neo4j

import (
	"context"
	"fmt"

	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// SchemaBootstrapperInterface is the companion interface for SchemaBootstrapper.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type SchemaBootstrapperInterface interface {
	EnsureSchema(ctx context.Context) error
}

// SchemaBootstrapper creates Neo4j schema elements (constraints, indexes).
// It is idempotent - re-running against an existing database must not error.
type SchemaBootstrapper struct {
	client             Neo4jClientInterface
	embeddingDimensions int
	logger             observability.LogProvider
}

// Ensure SchemaBootstrapper implements SchemaBootstrapperInterface
var _ SchemaBootstrapperInterface = (*SchemaBootstrapper)(nil)

// NewSchemaBootstrapper creates a new schema bootstrapper.
func NewSchemaBootstrapper(client Neo4jClientInterface, embeddingDimensions int, logger observability.LogProvider) *SchemaBootstrapper {
	return &SchemaBootstrapper{
		client:             client,
		embeddingDimensions: embeddingDimensions,
		logger:             logger,
	}
}

// EnsureSchema creates all required constraints and indexes if they don't exist.
// All CREATE statements use IF NOT EXISTS for idempotency.
func (s *SchemaBootstrapper) EnsureSchema(ctx context.Context) error {
	s.logger.Info("Ensuring Neo4j schema exists")

	// Create unique constraints
	constraints := []string{
		"CREATE CONSTRAINT sourcefragment_fragment_id_unique IF NOT EXISTS FOR (sf:SourceFragment) REQUIRE sf.fragment_id IS UNIQUE",
		"CREATE CONSTRAINT claim_claim_id_unique IF NOT EXISTS FOR (c:Claim) REQUIRE c.claim_id IS UNIQUE",
		"CREATE CONSTRAINT fact_fact_id_unique IF NOT EXISTS FOR (f:Fact) REQUIRE f.fact_id IS UNIQUE",
	}

	for _, cypher := range constraints {
		_, err := s.client.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			_, err := tx.Run(ctx, cypher, nil)
			return nil, err
		})
		if err != nil {
			return fmt.Errorf("failed to create constraint: %w", err)
		}
		s.logger.Debug("Created constraint", observability.String("query", cypher))
	}

	// Create profile_id indexes
	indexes := []string{
		"CREATE INDEX sourcefragment_profile_id_idx IF NOT EXISTS FOR (sf:SourceFragment) ON (sf.profile_id)",
		"CREATE INDEX claim_profile_id_idx IF NOT EXISTS FOR (c:Claim) ON (c.profile_id)",
		"CREATE INDEX fact_profile_id_idx IF NOT EXISTS FOR (f:Fact) ON (f.profile_id)",
	}

	for _, cypher := range indexes {
		_, err := s.client.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			_, err := tx.Run(ctx, cypher, nil)
			return nil, err
		})
		if err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
		s.logger.Debug("Created index", observability.String("query", cypher))
	}

	// Create full-text indexes
	fullTextIndexes := []string{
		"CREATE FULLTEXT INDEX sourcefragment_content IF NOT EXISTS FOR (sf:SourceFragment) ON EACH [sf.content]",
		"CREATE FULLTEXT INDEX fact_predicate IF NOT EXISTS FOR (f:Fact) ON EACH [f.predicate]",
	}

	for _, cypher := range fullTextIndexes {
		_, err := s.client.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			_, err := tx.Run(ctx, cypher, nil)
			return nil, err
		})
		if err != nil {
			return fmt.Errorf("failed to create full-text index: %w", err)
		}
		s.logger.Debug("Created full-text index", observability.String("query", cypher))
	}

	// Create vector index
	vectorIndex := fmt.Sprintf(
		"CREATE VECTOR INDEX sourcefragment_embedding IF NOT EXISTS FOR (sf:SourceFragment) ON sf.embedding OPTIONS {indexConfig: {`vector.dimensions`: %d, `vector.similarity_function`: 'cosine'}}",
		s.embeddingDimensions,
	)

	_, err := s.client.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, vectorIndex, nil)
		return nil, err
	})
	if err != nil {
		return fmt.Errorf("failed to create vector index: %w", err)
	}
	s.logger.Debug("Created vector index", observability.Int("dimensions", s.embeddingDimensions))

	s.logger.Info("Neo4j schema ensured successfully")
	return nil
}
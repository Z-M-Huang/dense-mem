package neo4j

import (
	"context"
	"fmt"

	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Canonical index names for Neo4j schema elements.
const (
	IndexFragmentContent   = "fragment_content_idx"
	IndexFragmentEmbedding = "fragment_embedding_idx"
	IndexFactPredicate     = "fact_predicate_idx"

	// Composite indexes for fragment deduplication and lookup (Unit 12)
	IndexFragmentProfileIdempotency = "fragment_profile_idempotency_idx"
	IndexFragmentProfileContentHash = "fragment_profile_content_hash_idx"
	IndexFragmentProfileCreatedAt   = "fragment_profile_created_at_idx"

	// Composite indexes for Claim nodes — profile_id is leading key (Unit 12, AC-3)
	IndexClaimProfileClaimID           = "claim_profile_claim_id_idx"
	IndexClaimProfileStatus            = "claim_profile_status_idx"
	IndexClaimProfilePredicate         = "claim_profile_predicate_idx"
	IndexClaimProfileSubjectPredicate  = "claim_profile_subject_predicate_idx"
	IndexClaimProfileIdempotency       = "claim_profile_idempotency_idx"
	IndexClaimProfileContentHash       = "claim_profile_content_hash_idx"

	// Composite indexes for Fact nodes — profile_id is leading key (Unit 12, AC-4)
	IndexFactProfileStatus                 = "fact_profile_status_idx"
	IndexFactProfileSubjectPredicateStatus = "fact_profile_subject_predicate_status_idx"

	// Composite index for SourceFragment nodes — profile_id is leading key (Unit 12, AC-5)
	IndexSourceFragmentProfileStatus = "sourcefragment_profile_status_idx"

	// Relationship profile_id existence constraints (Unit 13, AC-X1)
	// These names are canonical identifiers stored in Neo4j metadata.
	ConstraintSupportedByProfileIDExists  = "supported_by_profile_id_exists"
	ConstraintPromotesToProfileIDExists   = "promotes_to_profile_id_exists"
	ConstraintSupersededByProfileIDExists = "superseded_by_profile_id_exists"
	ConstraintContradictsProfileIDExists  = "contradicts_profile_id_exists"
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

	// Create relationship profile_id existence constraints (Unit 13, AC-X1).
	// profile_id is required on all pipeline edges so that no relationship can
	// escape profile isolation if a node-level filter is accidentally omitted.
	relationshipConstraints := []struct {
		cypher string
		name   string
	}{
		{
			"CREATE CONSTRAINT supported_by_profile_id_exists IF NOT EXISTS FOR ()-[r:SUPPORTED_BY]-() REQUIRE r.profile_id IS NOT NULL",
			ConstraintSupportedByProfileIDExists,
		},
		{
			"CREATE CONSTRAINT promotes_to_profile_id_exists IF NOT EXISTS FOR ()-[r:PROMOTES_TO]-() REQUIRE r.profile_id IS NOT NULL",
			ConstraintPromotesToProfileIDExists,
		},
		{
			"CREATE CONSTRAINT superseded_by_profile_id_exists IF NOT EXISTS FOR ()-[r:SUPERSEDED_BY]-() REQUIRE r.profile_id IS NOT NULL",
			ConstraintSupersededByProfileIDExists,
		},
		{
			"CREATE CONSTRAINT contradicts_profile_id_exists IF NOT EXISTS FOR ()-[r:CONTRADICTS]-() REQUIRE r.profile_id IS NOT NULL",
			ConstraintContradictsProfileIDExists,
		},
	}

	for _, rc := range relationshipConstraints {
		_, err := s.client.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			_, err := tx.Run(ctx, rc.cypher, nil)
			return nil, err
		})
		if err != nil {
			return fmt.Errorf("failed to create relationship constraint %s: %w", rc.name, err)
		}
		s.logger.Debug("Created relationship constraint", observability.String("name", rc.name))
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

	// Drop legacy indexes before creating canonical ones.
	// Also drops fact_predicate_idx by canonical name so it can be recreated
	// if a prior deployment created it with incorrect configuration.
	legacyDrops := []string{
		"DROP INDEX sourcefragment_content IF EXISTS",
		"DROP INDEX sourcefragment_embedding IF EXISTS",
		"DROP INDEX fact_predicate IF EXISTS",
		"DROP INDEX fact_predicate_idx IF EXISTS",
	}

	for _, cypher := range legacyDrops {
		_, err := s.client.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			_, err := tx.Run(ctx, cypher, nil)
			return nil, err
		})
		if err != nil {
			// Soft-fail: legacy drops are opportunistic cleanups, the index may not exist
			s.logger.Debug("could not drop legacy index (may not exist)", observability.String("query", cypher), observability.String("error", err.Error()))
			continue
		}
		s.logger.Debug("Dropped legacy index", observability.String("query", cypher))
	}

	// Create full-text indexes with canonical names
	fullTextIndexes := []struct {
		cypher string
		name   string
	}{
		{
			"CREATE FULLTEXT INDEX fragment_content_idx IF NOT EXISTS FOR (sf:SourceFragment) ON EACH [sf.content]",
			"fragment_content_idx",
		},
		{
			"CREATE FULLTEXT INDEX fact_predicate_idx IF NOT EXISTS FOR (f:Fact) ON EACH [f.predicate]",
			"fact_predicate_idx",
		},
	}

	for _, idx := range fullTextIndexes {
		_, err := s.client.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			_, err := tx.Run(ctx, idx.cypher, nil)
			return nil, err
		})
		if err != nil {
			return fmt.Errorf("failed to create full-text index: %w", err)
		}
		s.logger.Info("ensured index", observability.String("name", idx.name))
	}

	// Create vector index with canonical name
	vectorIndex := fmt.Sprintf(
		"CREATE VECTOR INDEX fragment_embedding_idx IF NOT EXISTS FOR (sf:SourceFragment) ON sf.embedding OPTIONS {indexConfig: {`vector.dimensions`: %d, `vector.similarity_function`: 'cosine'}}",
		s.embeddingDimensions,
	)

	_, err := s.client.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, vectorIndex, nil)
		return nil, err
	})
	if err != nil {
		return fmt.Errorf("failed to create vector index: %w", err)
	}
	s.logger.Info("ensured index", observability.String("name", "fragment_embedding_idx"))

	// Create composite indexes for fragment deduplication and lookup (Unit 12)
	// These are ADDITIVE migrations - no DROP of existing indexes.
	// AC-44: Idempotency-key uniqueness scoped to (profile_id, idempotency_key)
	// AC-45: Content-hash lookup profile-scoped
	// AC-29: Created-at ordering profile-scoped
	compositeIndexes := []struct {
		cypher string
		name   string
	}{
		{
			"CREATE INDEX fragment_profile_idempotency_idx IF NOT EXISTS FOR (sf:SourceFragment) ON (sf.profile_id, sf.idempotency_key)",
			"fragment_profile_idempotency_idx",
		},
		{
			"CREATE INDEX fragment_profile_content_hash_idx IF NOT EXISTS FOR (sf:SourceFragment) ON (sf.profile_id, sf.content_hash)",
			"fragment_profile_content_hash_idx",
		},
		{
			"CREATE INDEX fragment_profile_created_at_idx IF NOT EXISTS FOR (sf:SourceFragment) ON (sf.profile_id, sf.created_at)",
			"fragment_profile_created_at_idx",
		},
	}

	for _, idx := range compositeIndexes {
		_, err := s.client.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			_, err := tx.Run(ctx, idx.cypher, nil)
			return nil, err
		})
		if err != nil {
			return fmt.Errorf("failed to create composite index: %w", err)
		}
		s.logger.Info("ensured index", observability.String("name", idx.name))
	}

	// Create claim, fact, and sourcefragment composite indexes (Unit 12)
	// profile_id is always the leading key for efficient profile-scoped lookups.
	// AC-3: Claim composite indexes, AC-4: Fact composite indexes, AC-5: SF status index.
	pipelineIndexes := []struct {
		cypher string
		name   string
	}{
		// Claim indexes (AC-3)
		{
			"CREATE INDEX claim_profile_claim_id_idx IF NOT EXISTS FOR (c:Claim) ON (c.profile_id, c.claim_id)",
			IndexClaimProfileClaimID,
		},
		{
			"CREATE INDEX claim_profile_status_idx IF NOT EXISTS FOR (c:Claim) ON (c.profile_id, c.status)",
			IndexClaimProfileStatus,
		},
		{
			"CREATE INDEX claim_profile_predicate_idx IF NOT EXISTS FOR (c:Claim) ON (c.profile_id, c.predicate)",
			IndexClaimProfilePredicate,
		},
		{
			"CREATE INDEX claim_profile_subject_predicate_idx IF NOT EXISTS FOR (c:Claim) ON (c.profile_id, c.subject, c.predicate)",
			IndexClaimProfileSubjectPredicate,
		},
		{
			"CREATE INDEX claim_profile_idempotency_idx IF NOT EXISTS FOR (c:Claim) ON (c.profile_id, c.idempotency_key)",
			IndexClaimProfileIdempotency,
		},
		{
			"CREATE INDEX claim_profile_content_hash_idx IF NOT EXISTS FOR (c:Claim) ON (c.profile_id, c.content_hash)",
			IndexClaimProfileContentHash,
		},
		// Fact indexes (AC-4)
		{
			"CREATE INDEX fact_profile_status_idx IF NOT EXISTS FOR (f:Fact) ON (f.profile_id, f.status)",
			IndexFactProfileStatus,
		},
		{
			"CREATE INDEX fact_profile_subject_predicate_status_idx IF NOT EXISTS FOR (f:Fact) ON (f.profile_id, f.subject, f.predicate, f.status)",
			IndexFactProfileSubjectPredicateStatus,
		},
		// SourceFragment status index (AC-5)
		{
			"CREATE INDEX sourcefragment_profile_status_idx IF NOT EXISTS FOR (sf:SourceFragment) ON (sf.profile_id, sf.status)",
			IndexSourceFragmentProfileStatus,
		},
	}

	for _, idx := range pipelineIndexes {
		_, err := s.client.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			_, err := tx.Run(ctx, idx.cypher, nil)
			return nil, err
		})
		if err != nil {
			return fmt.Errorf("failed to create pipeline index %s: %w", idx.name, err)
		}
		s.logger.Info("ensured index", observability.String("name", idx.name))
	}

	s.logger.Info("Neo4j schema ensured successfully")
	return nil
}
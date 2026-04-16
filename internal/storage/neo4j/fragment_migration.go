package neo4j

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// FragmentMigrationRunner provides batch-safe migration operations for SourceFragment nodes.
//
// ADDITIVE MIGRATION CONTRACT (AC-42, AC-48):
// - All migrations in this file are ADDITIVE ONLY.
// - No DROP operations on indexes beyond Unit 2 legacy drops.
// - No destructive rewrite of existing nodes.
// - Legacy fragments remain readable with null-safe reads.
// - New properties are added without breaking backward compatibility.
// - Migration is safe to rerun (idempotent).
//
// NULL-SAFE READS (AC-42):
// - Properties like content_hash, idempotency_key, source_type may be null on legacy nodes.
// - Readers MUST use CoerceSourceType() to handle null source_type.
// - Readers MUST treat missing content_hash as empty string for comparison purposes.
// - Readers MUST treat missing idempotency_key as empty string.
type FragmentMigrationRunner interface {
	// BackfillContentHashes populates content_hash for SourceFragment nodes where it is null.
	// It processes in batches of batchSize to avoid a single monolithic transaction (AC-43).
	// Returns the total number of nodes processed.
	// The migration is idempotent - it only processes nodes with null content_hash.
	BackfillContentHashes(ctx context.Context, batchSize int) (processed int, err error)
}

// fragmentMigrationRunner implements FragmentMigrationRunner.
type fragmentMigrationRunner struct {
	client Neo4jClientInterface
	logger observability.LogProvider
}

// Ensure fragmentMigrationRunner implements FragmentMigrationRunner
var _ FragmentMigrationRunner = (*fragmentMigrationRunner)(nil)

// NewFragmentMigrationRunner creates a new migration runner for SourceFragment nodes.
func NewFragmentMigrationRunner(client Neo4jClientInterface, logger observability.LogProvider) FragmentMigrationRunner {
	return &fragmentMigrationRunner{
		client: client,
		logger: logger,
	}
}

// BackfillContentHashes populates content_hash for SourceFragment nodes where it is null.
// It processes in batches of batchSize to avoid a single monolithic transaction (AC-43).
//
// The algorithm:
// 1. Fetch a batch of nodes with null content_hash (using LIMIT for batch safety)
// 2. Compute SHA-256 hash of content in Go (Neo4j cannot compute SHA-256 in Cypher)
// 3. Write hashes back using UNWIND for efficient batch update
// 4. Repeat until no more null rows or context cancelled
//
// This approach ensures:
// - No single giant transaction (AC-43)
// - Progress logging for observability
// - Context cancellation support
// - Idempotency (only processes null content_hash)
func (r *fragmentMigrationRunner) BackfillContentHashes(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 100 // Default batch size
	}

	totalProcessed := 0

	for {
		// Check for context cancellation before each batch
		select {
		case <-ctx.Done():
			r.logger.Info("backfill interrupted by context", observability.Int("processed", totalProcessed))
			return totalProcessed, ctx.Err()
		default:
		}

		// Fetch a batch of nodes with null content_hash
		// Using LIMIT ensures batch-safe processing (AC-43)
		batch, err := r.fetchNullContentHashBatch(ctx, batchSize)
		if err != nil {
			return totalProcessed, fmt.Errorf("failed to fetch batch: %w", err)
		}

		if len(batch) == 0 {
			r.logger.Info("backfill complete, no more null content_hash nodes")
			break
		}

		// Compute hashes and write back
		processed, err := r.writeContentHashes(ctx, batch)
		if err != nil {
			return totalProcessed, fmt.Errorf("failed to write hashes: %w", err)
		}

		totalProcessed += processed
		r.logger.Info("backfill batch processed",
			observability.Int("batch_size", len(batch)),
			observability.Int("processed", processed),
			observability.Int("total", totalProcessed),
		)

		// If we got fewer than batchSize, we're done
		if len(batch) < batchSize {
			break
		}
	}

	return totalProcessed, nil
}

// fragmentWithContent represents a fragment node with its ID and content for hash computation.
type fragmentWithContent struct {
	FragmentID string
	Content    string
}

// fetchNullContentHashBatch fetches a batch of SourceFragment nodes where content_hash is null.
// It uses LIMIT to ensure batch-safe processing (AC-43).
func (r *fragmentMigrationRunner) fetchNullContentHashBatch(ctx context.Context, limit int) ([]fragmentWithContent, error) {
	query := `
		MATCH (sf:SourceFragment)
		WHERE sf.content_hash IS NULL
		WITH sf LIMIT $limit
		RETURN sf.fragment_id AS fragment_id, sf.content AS content
	`
	params := map[string]interface{}{"limit": limit}

	var fragments []fragmentWithContent

	_, err := r.client.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		// Collect all records
		records, err := result.Collect(ctx)
		if err != nil {
			return nil, err
		}

		fragments = make([]fragmentWithContent, 0, len(records))
		for _, record := range records {
			fragmentID, _ := record.Get("fragment_id")
			content, _ := record.Get("content")

			fragmentIDStr, _ := fragmentID.(string)
			contentStr, _ := content.(string)

			fragments = append(fragments, fragmentWithContent{
				FragmentID: fragmentIDStr,
				Content:    contentStr,
			})
		}

		return nil, nil
	})

	if err != nil {
		return nil, err
	}

	return fragments, nil
}

// computeContentHash computes SHA-256 hash of the content.
func computeContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// writeContentHashes writes computed hashes back to the database using UNWIND for batch efficiency.
func (r *fragmentMigrationRunner) writeContentHashes(ctx context.Context, fragments []fragmentWithContent) (int, error) {
	if len(fragments) == 0 {
		return 0, nil
	}

	// Prepare rows for UNWIND
	rows := make([]map[string]interface{}, len(fragments))
	for i, f := range fragments {
		rows[i] = map[string]interface{}{
			"fragment_id":  f.FragmentID,
			"content_hash": computeContentHash(f.Content),
		}
	}

	query := `
		UNWIND $rows AS row
		MATCH (sf:SourceFragment {fragment_id: row.fragment_id})
		SET sf.content_hash = row.content_hash
		RETURN count(sf) AS processed
	`

	var processed int

	_, err := r.client.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, map[string]interface{}{"rows": rows})
		if err != nil {
			return nil, err
		}

		// Collect result to get the count
		records, err := result.Collect(ctx)
		if err != nil {
			return nil, err
		}

		if len(records) > 0 {
			record := records[0]
			val, ok := record.Get("processed")
			if ok {
				if count, ok := val.(int64); ok {
					processed = int(count)
				}
			}
		}

		return nil, nil
	})

	if err != nil {
		return 0, err
	}

	return processed, nil
}

// CoerceSourceType handles null-safe reads for source_type (AC-46).
// When the raw value is nil or empty, it returns SourceTypeManual as the default.
// This ensures backward compatibility with legacy fragments that don't have source_type set.
func CoerceSourceType(raw interface{}) domain.SourceType {
	if raw == nil {
		return domain.SourceTypeManual
	}

	switch v := raw.(type) {
	case string:
		if v == "" {
			return domain.SourceTypeManual
		}
		st := domain.SourceType(v)
		if st.IsValid() {
			return st
		}
		return domain.SourceTypeManual
	case domain.SourceType:
		if v.IsValid() {
			return v
		}
		return domain.SourceTypeManual
	default:
		return domain.SourceTypeManual
	}
}
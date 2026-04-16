package keywordsearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// neo4jFragmentSearcher implements FragmentSearcherInterface using Neo4j.
type neo4jFragmentSearcher struct {
	reader ScopedReaderInterface
}

// ScopedReaderInterface is the interface for scoped read operations.
// This matches neo4j.ProfileScopeEnforcer's ScopedRead method.
type ScopedReaderInterface interface {
	ScopedRead(ctx context.Context, profileID string, query string, params map[string]any) (neo4j.ResultSummary, []map[string]any, error)
}

// Ensure neo4jFragmentSearcher implements FragmentSearcherInterface.
var _ FragmentSearcherInterface = (*neo4jFragmentSearcher)(nil)

// NewFragmentSearcher creates a new FragmentSearcherInterface using Neo4j.
func NewFragmentSearcher(reader ScopedReaderInterface) FragmentSearcherInterface {
	return &neo4jFragmentSearcher{reader: reader}
}

// SearchContent performs full-text search on SourceFragment content.
// Results are filtered by profile_id in the Cypher query.
func (s *neo4jFragmentSearcher) SearchContent(ctx context.Context, profileID string, query string, labels []string, limit int) ([]FragmentSearchResult, error) {
	// Build the Cypher query with full-text index search
	// Uses db.index.fulltext.queryNodes for content search
	cypherQuery := `
		CALL db.index.fulltext.queryNodes('fragment_content_idx', $searchQuery) YIELD node AS f, score
		WHERE f.profile_id = $profileId
		RETURN f.id AS fragment_id, f.content AS content, f.labels AS labels, f.metadata AS metadata, f.profile_id AS profile_id, score
		LIMIT $limit
	`

	// Build params
	params := map[string]any{
		"searchQuery": query,
		"limit":       limit,
	}

	// Add label filter if specified
	if len(labels) > 0 {
		labelConditions := make([]string, len(labels))
		for i, label := range labels {
			labelConditions[i] = fmt.Sprintf("'%s' IN f.labels", label)
		}
		cypherQuery = strings.Replace(cypherQuery, "WHERE f.profile_id = $profileId",
			fmt.Sprintf("WHERE f.profile_id = $profileId AND (%s)", strings.Join(labelConditions, " OR ")), 1)
	}

	// Execute via ScopedRead
	_, results, err := s.reader.ScopedRead(ctx, profileID, cypherQuery, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search fragments: %w", err)
	}

	// Convert results to FragmentSearchResult
	searchResults := make([]FragmentSearchResult, len(results))
	for i, row := range results {
		searchResults[i] = FragmentSearchResult{
			FragmentID: getString(row, "fragment_id"),
			Content:    getString(row, "content"),
			Score:      getFloat64Val(row, "score"),
			Labels:     getLabels(row, "labels"),
			Metadata:   getMetadata(row, "metadata"),
			ProfileID:  getString(row, "profile_id"),
		}
	}

	return searchResults, nil
}

// neo4jFactSearcher implements FactSearcherInterface using Neo4j.
type neo4jFactSearcher struct {
	reader ScopedReaderInterface
}

// Ensure neo4jFactSearcher implements FactSearcherInterface.
var _ FactSearcherInterface = (*neo4jFactSearcher)(nil)

// NewFactSearcher creates a new FactSearcherInterface using Neo4j.
func NewFactSearcher(reader ScopedReaderInterface) FactSearcherInterface {
	return &neo4jFactSearcher{reader: reader}
}

// SearchPredicate performs full-text search on Fact predicates.
// Results are filtered by profile_id in the Cypher query.
func (s *neo4jFactSearcher) SearchPredicate(ctx context.Context, profileID string, query string, labels []string, limit int) ([]FactSearchResult, error) {
	// Build the Cypher query with full-text index search
	// Uses db.index.fulltext.queryRelationships for predicate search
	cypherQuery := `
		CALL db.index.fulltext.queryRelationships('fact_predicate_idx', $searchQuery) YIELD relationship AS r, score
		WHERE r.profile_id = $profileId
		RETURN r.id AS fact_id, r.predicate AS predicate, r.labels AS labels, r.metadata AS metadata, r.profile_id AS profile_id, score
		LIMIT $limit
	`

	// Build params
	params := map[string]any{
		"searchQuery": query,
		"limit":       limit,
	}

	// Add label filter if specified
	if len(labels) > 0 {
		labelConditions := make([]string, len(labels))
		for i, label := range labels {
			labelConditions[i] = fmt.Sprintf("'%s' IN r.labels", label)
		}
		cypherQuery = strings.Replace(cypherQuery, "WHERE r.profile_id = $profileId",
			fmt.Sprintf("WHERE r.profile_id = $profileId AND (%s)", strings.Join(labelConditions, " OR ")), 1)
	}

	// Execute via ScopedRead
	_, results, err := s.reader.ScopedRead(ctx, profileID, cypherQuery, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search facts: %w", err)
	}

	// Convert results to FactSearchResult
	searchResults := make([]FactSearchResult, len(results))
	for i, row := range results {
		searchResults[i] = FactSearchResult{
			FactID:    getString(row, "fact_id"),
			Predicate: getString(row, "predicate"),
			Score:     getFloat64Val(row, "score"),
			Labels:    getLabels(row, "labels"),
			Metadata:  getMetadata(row, "metadata"),
			ProfileID: getString(row, "profile_id"),
		}
	}

	return searchResults, nil
}

// Helper functions for extracting values from Neo4j result maps
func getString(row map[string]any, key string) string {
	if val, ok := row[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
		return fmt.Sprintf("%v", val)
	}
	return ""
}

func getLabels(row map[string]any, key string) []string {
	if val, ok := row[key]; ok {
		if arr, ok := val.([]any); ok {
			labels := make([]string, len(arr))
			for i, v := range arr {
				labels[i] = fmt.Sprintf("%v", v)
			}
			return labels
		}
		if arr, ok := val.([]string); ok {
			return arr
		}
	}
	return nil
}

func getMetadata(row map[string]any, key string) map[string]any {
	if val, ok := row[key]; ok {
		if m, ok := val.(map[string]any); ok {
			return m
		}
	}
	return nil
}

func getFloat64Val(row map[string]any, key string) float64 {
	if val, ok := row[key]; ok {
		if f, ok := val.(float64); ok {
			return f
		}
		if f, ok := val.(float32); ok {
			return float64(f)
		}
	}
	return 0.0
}
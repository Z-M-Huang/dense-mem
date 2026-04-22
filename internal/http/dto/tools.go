package dto

import (
	"github.com/google/uuid"
	"time"
)

// GraphQueryRequest represents a request to execute a graph query.
// Validation rules:
//   - Query: max 5000 characters, not blank
//   - Parameters: optional query parameters
//   - TimeoutSeconds: optional query timeout
type GraphQueryRequest struct {
	Query          string         `json:"query" validate:"required,max=5000,notblank"`
	Parameters     map[string]any `json:"parameters"`
	TimeoutSeconds int            `json:"timeout_seconds"`
}

// KeywordSearchRequest represents a request for keyword-based search.
// Validation rules:
//   - Keywords: max 512 characters, not blank
//   - Limit: optional result limit (default: 10, max: 100)
type KeywordSearchRequest struct {
	Keywords        string     `json:"keywords" validate:"required,max=512,notblank"`
	Limit           int        `json:"limit" validate:"min=1,max=100"`
	ValidAt         *time.Time `json:"valid_at,omitempty"`
	KnownAt         *time.Time `json:"known_at,omitempty"`
	IncludeEvidence bool       `json:"include_evidence,omitempty"`
}

// SemanticSearchRequest represents a request for semantic search using embeddings.
// Validation rules:
//   - Embedding: exact length must match EmbeddingDimensions from config
//     (enforced via the embedding_dim custom validator; call
//     validation.SetEmbeddingDimensions at startup to activate)
//   - Limit: optional result limit (default: 10, max: 100)
type SemanticSearchRequest struct {
	Embedding       []float32  `json:"embedding" validate:"required,embedding_dim"`
	Limit           int        `json:"limit" validate:"min=1,max=100"`
	ValidAt         *time.Time `json:"valid_at,omitempty"`
	KnownAt         *time.Time `json:"known_at,omitempty"`
	IncludeEvidence bool       `json:"include_evidence,omitempty"`
}

// QueryStreamRequest represents a request to stream query results via SSE.
// Validation rules:
//   - Query: max 5000 characters, not blank
//   - Parameters: optional query parameters
type QueryStreamRequest struct {
	Query      string         `json:"query" validate:"required,max=5000,notblank"`
	Parameters map[string]any `json:"parameters"`
}

// AdminGraphQueryRequest represents an admin request to execute arbitrary graph queries.
// Validation rules:
//   - Query: max 5000 characters, not blank
//   - Parameters: optional query parameters
//   - TimeoutSeconds: optional query timeout (admin override available)
type AdminGraphQueryRequest struct {
	Query          string         `json:"query" validate:"required,max=5000,notblank"`
	Parameters     map[string]any `json:"parameters"`
	TimeoutSeconds int            `json:"timeout_seconds"`
}

// SearchResult represents a single search result.
type SearchResult struct {
	ID       uuid.UUID      `json:"id"`
	Type     string         `json:"type"`
	Score    float64        `json:"score"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
}

// GraphQueryResult represents the result of a graph query.
type GraphQueryResult struct {
	Columns []string       `json:"columns"`
	Rows    [][]any        `json:"rows"`
	Meta    map[string]any `json:"meta"`
}

// StreamEvent represents a single event in a query stream.
type StreamEvent struct {
	Type  string         `json:"type"`
	Data  map[string]any `json:"data"`
	Error string         `json:"error,omitempty"`
}

package domain

import (
	"time"
)

// SourceType represents the origin type of a Fragment.
type SourceType string

const (
	SourceTypeConversation SourceType = "conversation"
	SourceTypeDocument     SourceType = "document"
	SourceTypeObservation  SourceType = "observation"
	SourceTypeManual       SourceType = "manual"
)

// Fragment represents a knowledge fragment stored in the system.
// The FragmentID field uses Go name FragmentID but JSON tag "id" so the public API surface
// reads as "id": "..." (AC-41 compatibility clause) while internal code references FragmentID
// to match the Neo4j constraint name sourcefragment_fragment_id_unique.
type Fragment struct {
	FragmentID          string         `json:"id"` // API field "id" maps to stored fragment_id
	ProfileID           string         `json:"profile_id"`
	Content             string         `json:"content"`
	Source              string         `json:"source,omitempty"`
	SourceType          SourceType     `json:"source_type"`
	Labels              []string       `json:"labels,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
	ContentHash         string         `json:"content_hash"`
	IdempotencyKey      string         `json:"idempotency_key,omitempty"`
	EmbeddingModel      string         `json:"embedding_model"`
	EmbeddingDimensions int            `json:"embedding_dimensions"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	// Embedding vector deliberately NOT included in default read response (AC-28).
}

// ValidSourceTypes returns all valid SourceType values.
func ValidSourceTypes() []SourceType {
	return []SourceType{
		SourceTypeConversation,
		SourceTypeDocument,
		SourceTypeObservation,
		SourceTypeManual,
	}
}

// IsValid returns true if the SourceType is a valid value.
func (s SourceType) IsValid() bool {
	switch s {
	case SourceTypeConversation, SourceTypeDocument, SourceTypeObservation, SourceTypeManual:
		return true
	default:
		return false
	}
}
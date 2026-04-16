package dto

import (
	"encoding/json"
	"time"
)

// CreateFragmentRequest represents a request to create a new fragment.
// Validation rules:
//   - Content: required, non-blank, max 8192 bytes
//   - SourceType: optional enum (conversation, document, observation, manual)
//   - Source: optional, max 256 characters
//   - IdempotencyKey: optional, max 128 characters
//   - Labels: max 20 items, each item max 64 chars, non-blank
//   - Metadata: optional JSON object, size checked via ValidateMetadataSize
type CreateFragmentRequest struct {
	Content        string         `json:"content" validate:"required,max=8192,notblank"`
	SourceType     string         `json:"source_type,omitempty" validate:"omitempty,oneof=conversation document observation manual"`
	Source         string         `json:"source,omitempty" validate:"max=256"`
	IdempotencyKey string         `json:"idempotency_key,omitempty" validate:"max=128"`
	Labels         []string       `json:"labels,omitempty" validate:"max=20,dive,max=64,notblank"`
	Metadata       map[string]any `json:"metadata,omitempty"` // size check in handler
}

// FragmentResponse represents a fragment in API responses.
type FragmentResponse struct {
	ID                  string         `json:"id"`
	Content             string         `json:"content"`
	SourceType          string         `json:"source_type"`
	Source              string         `json:"source,omitempty"`
	Labels              []string       `json:"labels,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
	ContentHash         string         `json:"content_hash"`
	IdempotencyKey      string         `json:"idempotency_key,omitempty"`
	EmbeddingModel      string         `json:"embedding_model"`
	EmbeddingDimensions int            `json:"embedding_dimensions"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// ListFragmentsRequest represents query parameters for listing fragments.
// Validation rules:
//   - Limit: optional, 0-100
//   - Cursor: optional, max 256 characters
//   - SourceType: optional enum (conversation, document, observation, manual)
type ListFragmentsRequest struct {
	Limit      int    `query:"limit" validate:"min=0,max=100"`
	Cursor     string `query:"cursor" validate:"max=256"`
	SourceType string `query:"source_type" validate:"omitempty,oneof=conversation document observation manual"`
}

// ListFragmentsResponse represents a paginated list of fragments.
type ListFragmentsResponse struct {
	Items      []FragmentResponse `json:"items"`
	NextCursor string             `json:"next_cursor,omitempty"`
	HasMore    bool               `json:"has_more"`
}

// MaxMetadataBytes is the maximum size for metadata JSON after re-encoding.
const MaxMetadataBytes = 4096

// ValidateMetadataSize checks if metadata exceeds the maximum allowed size.
// This must be called in the handler because struct tag validation cannot
// size arbitrary maps. Returns nil if metadata is nil or within limits.
func ValidateMetadataSize(m map[string]any) error {
	if m == nil || len(m) == 0 {
		return nil
	}

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	if len(data) > MaxMetadataBytes {
		return &MetadataSizeError{Size: len(data), Max: MaxMetadataBytes}
	}

	return nil
}

// MetadataSizeError indicates metadata exceeds the maximum allowed size.
type MetadataSizeError struct {
	Size int
	Max  int
}

// Error implements the error interface.
func (e *MetadataSizeError) Error() string {
	return "metadata size exceeds maximum"
}
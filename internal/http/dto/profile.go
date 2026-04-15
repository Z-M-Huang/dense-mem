package dto

import (
	"github.com/google/uuid"
)

// CreateProfileRequest represents a request to create a new profile.
// Validation rules:
//   - Name: 3-100 characters, not blank
//   - Description: max 500 characters
//   - Metadata: serialized size <= 10KB
//   - Config: serialized size <= 10KB
type CreateProfileRequest struct {
	Name        string         `json:"name" validate:"required,min=3,max=100,notblank"`
	Description string         `json:"description" validate:"max=500"`
	Metadata    map[string]any `json:"metadata" validate:"maxbytes=10240"`
	Config      map[string]any `json:"config" validate:"maxbytes=10240"`
}

// UpdateProfileRequest represents a request to update an existing profile.
// Validation rules:
//   - Name: 3-100 characters, not blank (optional)
//   - Description: max 500 characters (optional)
//   - Metadata: serialized size <= 10KB (optional)
//   - Config: serialized size <= 10KB (optional)
type UpdateProfileRequest struct {
	Name        string         `json:"name" validate:"omitempty,min=3,max=100,notblank"`
	Description string         `json:"description" validate:"max=500"`
	Metadata    map[string]any `json:"metadata" validate:"maxbytes=10240"`
	Config      map[string]any `json:"config" validate:"maxbytes=10240"`
}

// ProfileResponse represents a profile in API responses.
type ProfileResponse struct {
	ID          uuid.UUID      `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Metadata    map[string]any `json:"metadata"`
	Config      map[string]any `json:"config"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

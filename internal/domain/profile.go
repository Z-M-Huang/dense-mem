package domain

import (
	"time"

	"github.com/google/uuid"
)

// ProfileModel is the companion interface for Profile.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type ProfileModel interface {
	GetID() uuid.UUID
	GetName() string
	GetDescription() string
	GetMetadata() map[string]any
	GetConfig() map[string]any
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
	GetDeletedAt() *time.Time
}

// Profile represents a user-defined profile for organizing knowledge.
type Profile struct {
	ID          uuid.UUID
	Name        string
	Description string
	Metadata    map[string]any
	Config      map[string]any
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// Ensure Profile implements ProfileModel
var _ ProfileModel = (*Profile)(nil)

// Getters for ProfileModel interface
func (p *Profile) GetID() uuid.UUID       { return p.ID }
func (p *Profile) GetName() string        { return p.Name }
func (p *Profile) GetDescription() string { return p.Description }
func (p *Profile) GetMetadata() map[string]any { return p.Metadata }
func (p *Profile) GetConfig() map[string]any   { return p.Config }
func (p *Profile) GetCreatedAt() time.Time     { return p.CreatedAt }
func (p *Profile) GetUpdatedAt() time.Time     { return p.UpdatedAt }
func (p *Profile) GetDeletedAt() *time.Time    { return p.DeletedAt }

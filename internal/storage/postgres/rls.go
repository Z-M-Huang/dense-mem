package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// RLSHelper is the companion interface for the RLS helper struct.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type RLSHelper interface {
	WithProfileTx(ctx context.Context, db *gorm.DB, profileID string, role string, fn func(tx *gorm.DB) error) error
	WithAdminTx(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error
}

// RLS provides helper methods for executing transactions with Row Level Security
// session variables set via SET LOCAL (transaction-scoped).
type RLS struct{}

// Ensure RLS implements RLSHelper interface
var _ RLSHelper = (*RLS)(nil)

// NewRLS creates a new RLS helper instance.
func NewRLS() *RLS {
	return &RLS{}
}

// WithProfileTx executes fn inside a transaction with profile session variables set via set_config.
// The session variables are scoped to the transaction (is_local=true) so they cannot bleed
// into subsequent queries on a pooled connection.
//
// We use set_config(setting, value, is_local) rather than SET LOCAL because SET LOCAL does not
// accept parameterized values — it requires a SQL literal. set_config is a regular function that
// accepts a bound parameter, which both avoids SQL-injection risk and lets gorm/pgx bind the value.
func (r *RLS) WithProfileTx(ctx context.Context, db *gorm.DB, profileID string, role string, fn func(tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_profile_id', ?, true)", profileID).Error; err != nil {
			return fmt.Errorf("failed to set app.current_profile_id: %w", err)
		}
		if err := tx.Exec("SELECT set_config('app.role', ?, true)", role).Error; err != nil {
			return fmt.Errorf("failed to set app.role: %w", err)
		}
		return fn(tx)
	})
}

// WithAdminTx executes fn inside a transaction with admin session variables set via set_config.
// The session variables are scoped to the transaction (is_local=true) so they cannot bleed
// into subsequent queries on a pooled connection. app.current_profile_id is cleared to '' so
// profile-scoped RLS policies reject admin transactions unless they target role='admin'.
func (r *RLS) WithAdminTx(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_profile_id', '', true)").Error; err != nil {
			return fmt.Errorf("failed to set app.current_profile_id: %w", err)
		}
		if err := tx.Exec("SELECT set_config('app.role', 'admin', true)").Error; err != nil {
			return fmt.Errorf("failed to set app.role: %w", err)
		}
		return fn(tx)
	})
}

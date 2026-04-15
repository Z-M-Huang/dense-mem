package admingraph

import (
	"fmt"
	"regexp"
	"strings"
)

// AdminGraphValidator is the companion interface for adminGraphValidator.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type AdminGraphValidator interface {
	Validate(query string) error
}

// ValidationError represents a query validation failure with a specific reason.
type ValidationError struct {
	Reason string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("admin graph validation failed: %s", e.Reason)
}

// adminGraphValidator validates Cypher queries for admin graph execution.
// It enforces read-only operations and blocks unsafe procedures.
type adminGraphValidator struct{}

// Ensure adminGraphValidator implements AdminGraphValidator
var _ AdminGraphValidator = (*adminGraphValidator)(nil)

// NewAdminGraphValidator creates a new AdminGraphValidator.
func NewAdminGraphValidator() AdminGraphValidator {
	return &adminGraphValidator{}
}

// writeClausesPattern matches write operations in Cypher queries.
var writeClausesPattern = regexp.MustCompile(`(?i)\b(CREATE|MERGE|DELETE|SET|REMOVE|DROP|FOREACH)\b`)

// loadCSVPattern matches LOAD CSV clause.
var loadCSVPattern = regexp.MustCompile(`(?i)\bLOAD\s+CSV\b`)

// semicolonPattern matches semicolons that would indicate multiple statements.
var semicolonPattern = regexp.MustCompile(`;`)

// unsafeProcedurePattern matches unsafe APOC, db, network, and file procedures.
// This list is exhaustive and covers all known dangerous procedures.
var unsafeProcedurePattern = regexp.MustCompile(`(?i)\bCALL\s+(` +
	// APOC destructive procedures
	`apoc\.destroy|` +
	`apoc\.delete|` +
	`apoc\.remove|` +
	`apoc\.drop|` +
	`apoc\.create|` +
	`apoc\.merge|` +
	`apoc\.set|` +
	`apoc\.update|` +
	`apoc\.transform|` +
	`apoc\.load\.json|` +
	`apoc\.load\.csv|` +
	`apoc\.load\.xml|` +
	`apoc\.import|` +
	`apoc\.export|` +
	`apoc\.periodic\.iterate|` +
	`apoc\.periodic\.commit|` +
	`apoc\.do|` +
	`apoc\.cypher\.run|` +
	`apoc\.cypher\.runMany|` +
	`apoc\.cypher\.mapped|` +
	`apoc\.run|` +
	// APOC network procedures
	`apoc\.util\.http|` +
	`apoc\.util\.sleep|` +
	// Database administration procedures
	`db\.create|` +
	`db\.drop|` +
	`db\.clear|` +
	`db\.labels|` +      // Can reveal schema
	`db\.propertyKeys|` + // Can reveal schema
	`db\.relationshipTypes|` + // Can reveal schema
	`db\.indexes|` +     // Can reveal schema
	`db\.constraints|` + // Can reveal schema
	`db\.awaitIndex|` +
	`db\.awaitIndexes|` +
	`db\.createIndex|` +
	`db\.createConstraint|` +
	`db\.dropIndex|` +
	`db\.dropConstraint|` +
	// System/database switching
	`db\.switchTo|` +
	// Network procedures
	`net\.|` +
	// File system procedures (custom or APOC)
	`file\.|` +
	// Schema manipulation
	`schema\.create|` +
	`schema\.drop|` +
	`schema\.assert|` +
	// Any procedure containing these dangerous keywords
	`apoc\.schema|` +
	`)`)

// schemaChangePattern matches schema change operations.
var schemaChangePattern = regexp.MustCompile(`(?i)\b(SCHEMA|INDEX|CONSTRAINT)\b.*\b(CREATE|DROP|ASSERT)\b|\b(CREATE|DROP)\b.*\b(SCHEMA|INDEX|CONSTRAINT)\b`)

// Validate checks if a Cypher query is safe for admin graph execution.
// It rejects queries containing:
// - Write clauses: CREATE, MERGE, DELETE, SET, REMOVE, DROP, FOREACH
// - LOAD CSV
// - Multiple statements (semicolons)
// - Unsafe APOC procedures
// - Database administration procedures
// - Network/file procedures
// - Schema changes
func (v *adminGraphValidator) Validate(query string) error {
	query = strings.TrimSpace(query)

	// Check for semicolons (multiple statements)
	if semicolonPattern.MatchString(query) {
		return &ValidationError{Reason: "multiple statements are not allowed (semicolon detected)"}
	}

	// Check for LOAD CSV
	if loadCSVPattern.MatchString(query) {
		return &ValidationError{Reason: "LOAD CSV is not allowed"}
	}

	// Check for write clauses
	if match := writeClausesPattern.FindString(query); match != "" {
		return &ValidationError{Reason: fmt.Sprintf("query contains forbidden write clause: %s", strings.ToUpper(match))}
	}

	// Check for unsafe procedures (CALL clause with dangerous procedures)
	if unsafeProcedurePattern.MatchString(query) {
		return &ValidationError{Reason: "query contains unsafe procedure call"}
	}

	// Check for schema changes
	if schemaChangePattern.MatchString(query) {
		return &ValidationError{Reason: "schema changes are not allowed"}
	}

	return nil
}

// IsValidationError checks if an error is a ValidationError.
func IsValidationError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "admin graph validation failed")
}
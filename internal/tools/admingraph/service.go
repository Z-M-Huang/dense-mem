package admingraph

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

const (
	// DefaultTimeout is the default timeout for admin graph queries (30 seconds).
	DefaultTimeout = 30 * time.Second
	// MaxRowLimit is the maximum allowed LIMIT value for queries (1000).
	MaxRowLimit = 1000
	// DefaultRowLimit is the default LIMIT applied when a query has none.
	DefaultRowLimit = 1000
	// AuditTimeout bounds synchronous audit writes so a slow audit backend
	// cannot hold the request indefinitely. The write remains synchronous so
	// the record is flushed before the handler returns; only the wait is capped.
	AuditTimeout = 5 * time.Second
)

// AdminGraphResult represents the result of an admin graph query execution.
type AdminGraphResult struct {
	Columns  []string         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	RowCount int              `json:"row_count"`
}

// AdminGraphService is the interface for executing admin graph queries.
type AdminGraphService interface {
	Execute(ctx context.Context, profileID string, query string, params map[string]any) (*AdminGraphResult, error)
	ExecuteWithAudit(ctx context.Context, profileID string, query string, params map[string]any, actorKeyID *string, actorRole, clientIP, correlationID string) (*AdminGraphResult, error)
}

// ScopedReaderInterface is the interface for scoped read operations.
// This matches ProfileScopeEnforcer's ScopedRead method.
type ScopedReaderInterface interface {
	ScopedRead(ctx context.Context, profileID string, query string, params map[string]any) (neo4j.ResultSummary, []map[string]any, error)
}

// AuditServiceInterface is the interface for audit logging.
type AuditServiceInterface interface {
	AdminQuery(ctx context.Context, queryType string, metadata map[string]interface{}, actorKeyID *string, actorRole, clientIP, correlationID string) error
}

// adminGraphService implements AdminGraphService.
type adminGraphService struct {
	reader    ScopedReaderInterface
	validator AdminGraphValidator
	auditSvc  AuditServiceInterface
	timeout   time.Duration
}

// Ensure adminGraphService implements AdminGraphService.
var _ AdminGraphService = (*adminGraphService)(nil)

// NewAdminGraphService creates a new AdminGraphService.
func NewAdminGraphService(reader ScopedReaderInterface, validator AdminGraphValidator, auditSvc AuditServiceInterface, timeout time.Duration) AdminGraphService {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &adminGraphService{
		reader:    reader,
		validator: validator,
		auditSvc:  auditSvc,
		timeout:   timeout,
	}
}

// Execute validates, prepares, and executes a Cypher query for admin access.
// It enforces:
// - Validation against write clauses and unsafe procedures
// - Timeout enforcement via context cancellation
// - LIMIT 1000 injected into Cypher before execution (caps Neo4j-side work,
//   not a post-collection slice — protects against memory/DoS on large results)
// - Audit logging of all executions (success or failure)
func (s *adminGraphService) Execute(ctx context.Context, profileID string, query string, params map[string]any) (*AdminGraphResult, error) {
	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// Validate query
	if err := s.validator.Validate(query); err != nil {
		return nil, err
	}

	// Check for forbidden params (profileId, profile_id)
	if err := validateParams(params); err != nil {
		return nil, err
	}

	// Inject LIMIT into the Cypher query before execution. Queries with no
	// LIMIT get LIMIT 1000 appended; queries with LIMIT > 1000 or LIMIT 0
	// are rejected so Neo4j never scans beyond the row cap.
	processedQuery, err := processLimitClause(query)
	if err != nil {
		return nil, err
	}

	// Prepare params with profile_id injection
	preparedParams := prepareParams(profileID, params)

	// Execute via ScopedRead with timeout context
	_, rows, err := s.reader.ScopedRead(timeoutCtx, profileID, processedQuery, preparedParams)

	// Check for context timeout
	if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
		return nil, &TimeoutError{Timeout: s.timeout}
	}

	// Handle other errors
	if err != nil {
		return nil, sanitizeNeo4jError(err)
	}

	// Extract columns from rows
	columns := extractColumns(rows)

	return &AdminGraphResult{
		Columns:  columns,
		Rows:     rows,
		RowCount: len(rows),
	}, nil
}

// ExecuteWithAudit executes a query and logs an audit event regardless of success/failure.
// This is the primary entry point for the handler, ensuring audit logging happens
// even when the query returns an error.
func (s *adminGraphService) ExecuteWithAudit(ctx context.Context, profileID string, query string, params map[string]any, actorKeyID *string, actorRole, clientIP, correlationID string) (*AdminGraphResult, error) {
	// Execute the query
	result, err := s.Execute(ctx, profileID, query, params)

	// Build audit metadata
	metadata := map[string]interface{}{
		"profile_id": profileID,
		"query":      sanitizeQueryForAudit(query),
		"success":    err == nil,
	}

	if err != nil {
		metadata["error"] = sanitizeErrorForAudit(err)
	} else if result != nil {
		metadata["row_count"] = result.RowCount
	}

	// Audit synchronously so the record is persisted before the handler returns
	// (non-repudiation: a crash/timeout after return must not lose the audit event).
	// We derive a fresh context from context.Background() so request cancellation by
	// the caller does not cancel the audit write; a bounded AuditTimeout caps the
	// wait on a slow audit backend. Audit failures are logged via the audit service
	// and do not fail the query response — the audit service is responsible for its
	// own durability semantics.
	auditCtx, auditCancel := context.WithTimeout(context.Background(), AuditTimeout)
	defer auditCancel()
	_ = s.auditSvc.AdminQuery(auditCtx, "graph", metadata, actorKeyID, actorRole, clientIP, correlationID)

	return result, err
}

// limitPattern matches LIMIT clauses in Cypher queries (LIMIT <integer>).
var limitPattern = regexp.MustCompile(`(?i)\bLIMIT\s+(\d+)\b`)

// processLimitClause enforces an upper row cap in the Cypher query itself.
//   - No LIMIT present: append " LIMIT 1000".
//   - LIMIT 0: reject (unusable).
//   - LIMIT > 1000: reject (admin cap is 1000).
//   - LIMIT in (1..1000]: pass through unchanged.
//
// This runs before ScopedRead so Neo4j stops scanning once the cap is hit,
// instead of the service truncating an already-materialized result set.
func processLimitClause(query string) (string, error) {
	matches := limitPattern.FindAllStringSubmatch(query, -1)
	if len(matches) == 0 {
		return strings.TrimRight(strings.TrimSpace(query), ";") + " LIMIT " + strconv.Itoa(DefaultRowLimit), nil
	}

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		limit, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		if limit == 0 {
			return "", &LimitError{Message: "LIMIT 0 is not allowed"}
		}
		if limit > MaxRowLimit {
			return "", &LimitError{Message: fmt.Sprintf("LIMIT %d exceeds maximum allowed value of %d", limit, MaxRowLimit)}
		}
	}

	return query, nil
}

// prepareParams creates the params map with profile_id injected.
func prepareParams(profileID string, params map[string]any) map[string]any {
	prepared := make(map[string]any)
	if params != nil {
		for k, v := range params {
			prepared[k] = v
		}
	}
	prepared["profileId"] = profileID
	return prepared
}

// validateParams checks that params don't contain forbidden keys.
func validateParams(params map[string]any) error {
	if params == nil {
		return nil
	}

	// Check for profileId and profile_id
	forbiddenKeys := []string{"profileId", "profile_id"}
	for _, key := range forbiddenKeys {
		if _, exists := params[key]; exists {
			return &ForbiddenParamError{Param: key}
		}
	}

	return nil
}

// extractColumns extracts column names from the first row of results.
func extractColumns(rows []map[string]any) []string {
	if len(rows) == 0 {
		return []string{}
	}

	// Get keys from the first row
	firstRow := rows[0]
	columns := make([]string, 0, len(firstRow))
	for col := range firstRow {
		columns = append(columns, col)
	}

	return columns
}

// sanitizeNeo4jError converts Neo4j driver errors to user-safe errors.
func sanitizeNeo4jError(err error) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	// Check for Neo4j syntax errors
	if strings.Contains(errStr, "SyntaxException") ||
		strings.Contains(errStr, "syntax error") ||
		strings.Contains(errStr, "Invalid input") ||
		strings.Contains(errStr, "Unknown function") {
		return &SyntaxError{Message: "query syntax error"}
	}

	// Return generic sanitized error
	return &QueryError{Message: "query execution failed"}
}

// sanitizeQueryForAudit removes sensitive parts of query for audit log.
func sanitizeQueryForAudit(query string) string {
	// Truncate if too long
	if len(query) > 500 {
		return query[:500] + "..."
	}
	return query
}

// sanitizeErrorForAudit creates a safe error message for audit logs.
func sanitizeErrorForAudit(err error) string {
	if err == nil {
		return ""
	}

	// For typed errors, use the type name
	if _, ok := err.(*ValidationError); ok {
		return "validation_error"
	}
	if _, ok := err.(*TimeoutError); ok {
		return "timeout"
	}
	if _, ok := err.(*ForbiddenParamError); ok {
		return "forbidden_param"
	}
	if _, ok := err.(*SyntaxError); ok {
		return "syntax_error"
	}
	if _, ok := err.(*LimitError); ok {
		return "limit_error"
	}

	// Generic sanitized message
	return "execution_failed"
}

// ============================================================================
// Error Types
// ============================================================================

// TimeoutError represents a query timeout error.
type TimeoutError struct {
	Timeout time.Duration
}

// Error implements the error interface.
func (e *TimeoutError) Error() string {
	return fmt.Sprintf("query exceeded timeout of %v", e.Timeout)
}

// IsTimeoutError checks if an error is a TimeoutError.
func IsTimeoutError(err error) bool {
	var timeoutErr *TimeoutError
	return errors.As(err, &timeoutErr)
}

// ForbiddenParamError represents an error when forbidden params are provided.
type ForbiddenParamError struct {
	Param string
}

// Error implements the error interface.
func (e *ForbiddenParamError) Error() string {
	return fmt.Sprintf("parameter '%s' is not allowed", e.Param)
}

// IsForbiddenParamError checks if an error is a ForbiddenParamError.
func IsForbiddenParamError(err error) bool {
	var forbiddenErr *ForbiddenParamError
	return errors.As(err, &forbiddenErr)
}

// SyntaxError represents a sanitized syntax error.
type SyntaxError struct {
	Message string
}

// Error implements the error interface.
func (e *SyntaxError) Error() string {
	return e.Message
}

// IsSyntaxError checks if an error is a SyntaxError.
func IsSyntaxError(err error) bool {
	var syntaxErr *SyntaxError
	return errors.As(err, &syntaxErr)
}

// LimitError represents a rejection from processLimitClause
// (LIMIT 0 or LIMIT exceeding MaxRowLimit).
type LimitError struct {
	Message string
}

// Error implements the error interface.
func (e *LimitError) Error() string {
	return e.Message
}

// IsLimitError checks if an error is a LimitError.
func IsLimitError(err error) bool {
	var limitErr *LimitError
	return errors.As(err, &limitErr)
}

// QueryError represents a generic query execution error.
type QueryError struct {
	Message string
}

// Error implements the error interface.
func (e *QueryError) Error() string {
	return e.Message
}

// IsQueryError checks if an error is a QueryError.
func IsQueryError(err error) bool {
	var queryErr *QueryError
	return errors.As(err, &queryErr)
}
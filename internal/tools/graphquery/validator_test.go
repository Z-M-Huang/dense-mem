package graphquery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// CypherValidator Tests (Pure Unit Tests)
// ============================================================================

// TestRejectWriteClauses tests that write clauses and forbidden constructs are rejected.
func TestRejectWriteClauses(t *testing.T) {
	validator := NewCypherValidator()

	tests := []struct {
		name          string
		query         string
		shouldReject  bool
		expectedError string
	}{
		{
			name:          "CREATE clause",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId}) CREATE (n:Test)",
			shouldReject:  true,
			expectedError: "CREATE",
		},
		{
			name:          "MERGE clause",
			query:         "MERGE (n:Test {profile_id: $profileId})",
			shouldReject:  true,
			expectedError: "MERGE",
		},
		{
			name:          "DELETE clause",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId}) DELETE n",
			shouldReject:  true,
			expectedError: "DELETE",
		},
		{
			name:          "SET clause",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId}) SET n.value = 1",
			shouldReject:  true,
			expectedError: "SET",
		},
		{
			name:          "REMOVE clause",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId}) REMOVE n.value",
			shouldReject:  true,
			expectedError: "REMOVE",
		},
		{
			name:          "DROP clause",
			query:         "DROP INDEX test_index",
			shouldReject:  true,
			expectedError: "DROP",
		},
		{
			name:          "FOREACH clause",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId}) FOREACH (x IN [1,2,3] | CREATE (:Test))",
			shouldReject:  true,
			expectedError: "FOREACH",
		},
		{
			name:          "CALL clause",
			query:         "CALL db.labels()",
			shouldReject:  true,
			expectedError: "CALL",
		},
		{
			name:          "UNION clause",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId}) RETURN n UNION MATCH (m:SourceFragment {profile_id: $profileId}) RETURN m",
			shouldReject:  true,
			expectedError: "UNION",
		},
		{
			name:          "USE clause",
			query:         "USE myDatabase MATCH (n:SourceFragment {profile_id: $profileId}) RETURN n",
			shouldReject:  true,
			expectedError: "USE",
		},
		{
			name:          "LOAD CSV clause",
			query:         "LOAD CSV FROM 'file:///data.csv' AS row CREATE (:Test)",
			shouldReject:  true,
			expectedError: "LOAD CSV", // LOAD CSV is checked before CREATE
		},
		{
			name:          "semicolon (multiple statements)",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId}) RETURN n; MATCH (m:SourceFragment {profile_id: $profileId}) RETURN m",
			shouldReject:  true,
			expectedError: "semicolon",
		},
		{
			name:          "lowercase create",
			query:         "match (n:SourceFragment {profile_id: $profileId}) create (n:Test)",
			shouldReject:  true,
			expectedError: "CREATE",
		},
		{
			name:          "mixed case SET",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId}) SeT n.value = 1",
			shouldReject:  true,
			expectedError: "SET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.query)
			if tt.shouldReject {
				assert.Error(t, err, "Query should be rejected: %s", tt.query)
				assert.Contains(t, err.Error(), tt.expectedError, "Error should contain expected text")
				// Verify it's a ValidationError
				var validationErr *ValidationError
				assert.ErrorAs(t, err, &validationErr, "Error should be a ValidationError")
			} else {
				assert.NoError(t, err, "Query should be allowed: %s", tt.query)
			}
		})
	}
}

// TestRejectMissingProfilePredicate tests that queries without profile_id predicates are rejected.
func TestRejectMissingProfilePredicate(t *testing.T) {
	validator := NewCypherValidator()

	tests := []struct {
		name          string
		query         string
		shouldReject  bool
		expectedError string
	}{
		{
			name:          "MATCH without profile_id",
			query:         "MATCH (n:SourceFragment) RETURN n",
			shouldReject:  true,
			expectedError: "profile_id",
		},
		{
			name:          "MATCH with wrong parameter name",
			query:         "MATCH (n:SourceFragment {profile_id: $id}) RETURN n",
			shouldReject:  true,
			expectedError: "profile_id",
		},
		{
			name:          "MATCH with alias but no profile_id in WHERE",
			query:         "MATCH (n:SourceFragment) WHERE n.active = true RETURN n",
			shouldReject:  true,
			expectedError: "profile_id",
		},
		{
			name:          "multiple aliases none with profile_id",
			query:         "MATCH (n:SourceFragment)-[:REL]->(m:Target) RETURN n, m",
			shouldReject:  true,
			expectedError: "profile_id",
		},
		{
			name:          "valid inline profile_id",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId}) RETURN n",
			shouldReject:  false,
		},
		{
			name:          "valid WHERE clause profile_id",
			query:         "MATCH (n:SourceFragment) WHERE n.profile_id = $profileId RETURN n",
			shouldReject:  false,
		},
		{
			name:          "RETURN without MATCH (no node patterns)",
			query:         "RETURN 1 AS value",
			shouldReject:  false,
		},
		{
			name:          "CASE statement with profile_id in WHERE",
			query:         "MATCH (n:SourceFragment) WHERE n.profile_id = $profileId RETURN n",
			shouldReject:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.query)
			if tt.shouldReject {
				assert.Error(t, err, "Query should be rejected: %s", tt.query)
				assert.Contains(t, err.Error(), tt.expectedError, "Error should contain expected text")
				var validationErr *ValidationError
				assert.ErrorAs(t, err, &validationErr, "Error should be a ValidationError")
			} else {
				assert.NoError(t, err, "Query should be allowed: %s", tt.query)
			}
		})
	}
}

// TestRejectUnfilteredTraversal tests that traversals with unfiltered endpoints are rejected.
func TestRejectUnfilteredTraversal(t *testing.T) {
	validator := NewCypherValidator()

	tests := []struct {
		name          string
		query         string
		shouldReject  bool
		expectedError string
	}{
		{
			name:          "anonymous start node",
			query:         "MATCH ()-[:REL]->(n:SourceFragment {profile_id: $profileId}) RETURN n",
			shouldReject:  true,
			expectedError: "alias",
		},
		{
			name:          "anonymous end node",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId})-[:REL]->() RETURN n",
			shouldReject:  true, // Anonymous node is not allowed
			expectedError: "alias",
		},
		{
			name:          "unfiltered second endpoint without profile_id",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId})-[:REL]->(m:Target) RETURN n, m",
			shouldReject:  true,
			expectedError: "profile_id",
		},
		{
			name:          "both endpoints filtered",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId})-[:REL]->(m:Target {profile_id: $profileId}) RETURN n, m",
			shouldReject:  false,
		},
		{
			name:          "traversal with WHERE filter",
			query:         "MATCH (n:SourceFragment)-[:REL]->(m:Target) WHERE n.profile_id = $profileId RETURN n, m",
			shouldReject:  true, // m is not filtered
			expectedError: "profile_id",
		},
		{
			name:          "both aliases in WHERE clause",
			query:         "MATCH (n:SourceFragment)-[:REL]->(m:Target) WHERE n.profile_id = $profileId AND m.profile_id = $profileId RETURN n, m",
			shouldReject:  false,
		},
		{
			name:          "single node with inline filter",
			query:         "MATCH (n:SourceFragment {profile_id: $profileId}) RETURN n",
			shouldReject:  false,
		},
		{
			name:          "label-only node pattern without alias",
			query:         "MATCH (:SourceFragment)-[:REL]->(n:Target {profile_id: $profileId}) RETURN n",
			shouldReject:  true,
			expectedError: "alias",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.query)
			if tt.shouldReject {
				assert.Error(t, err, "Query should be rejected: %s", tt.query)
				if tt.expectedError != "" {
					assert.Contains(t, err.Error(), tt.expectedError, "Error should contain expected text")
				}
				var validationErr *ValidationError
				require.ErrorAs(t, err, &validationErr, "Error should be a ValidationError")
			} else {
				assert.NoError(t, err, "Query should be allowed: %s", tt.query)
			}
		})
	}
}

// TestAcceptScopedReadOnlyQuery tests that valid scoped read-only queries are accepted.
func TestAcceptScopedReadOnlyQuery(t *testing.T) {
	validator := NewCypherValidator()

	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "simple MATCH with inline profile_id",
			query: "MATCH (n:SourceFragment {profile_id: $profileId}) RETURN n",
		},
		{
			name:  "MATCH with WHERE profile_id",
			query: "MATCH (n:SourceFragment) WHERE n.profile_id = $profileId RETURN n",
		},
		{
			name:  "MATCH with multiple conditions including profile_id",
			query: "MATCH (n:SourceFragment) WHERE n.profile_id = $profileId AND n.active = true RETURN n",
		},
		{
			name:  "MATCH with ORDER BY and LIMIT",
			query: "MATCH (n:SourceFragment {profile_id: $profileId}) RETURN n ORDER BY n.created_at LIMIT 10",
		},
		{
			name:  "MATCH with WHERE and ORDER BY",
			query: "MATCH (n:SourceFragment) WHERE n.profile_id = $profileId RETURN n ORDER BY n.created_at",
		},
		{
			name:  "relationship traversal with both filtered",
			query: "MATCH (n:SourceFragment {profile_id: $profileId})-[:CONTAINS]->(f:Fact {profile_id: $profileId}) RETURN n, f",
		},
		{
			name:  "multi-hop traversal with all filtered",
			query: "MATCH (n:SourceFragment {profile_id: $profileId})-[:CONTAINS]->(f:Fact {profile_id: $profileId})-[:LINKS_TO]->(t:Target {profile_id: $profileId}) RETURN n, f, t",
		},
		{
			name:  "OPTIONAL MATCH with profile_id",
			query: "MATCH (n:SourceFragment {profile_id: $profileId}) OPTIONAL MATCH (n)-[:HAS_TAG]->(t:Tag) RETURN n, t",
		},
		{
			name:  "WITH clause with profile_id",
			query: "MATCH (n:SourceFragment {profile_id: $profileId}) WITH n RETURN n",
		},
		{
			name:  "multiple WHERE conditions",
			query: "MATCH (n:SourceFragment) WHERE n.profile_id = $profileId AND n.status = 'active' AND n.count > 0 RETURN n",
		},
		{
			name:  "projection with profile_id filter",
			query: "MATCH (n:SourceFragment {profile_id: $profileId}) RETURN n.id, n.name, n.created_at",
		},
		{
			name:  "COUNT aggregation",
			query: "MATCH (n:SourceFragment {profile_id: $profileId}) RETURN count(n) AS total",
		},
		{
			name:  "GROUP BY aggregation",
			query: "MATCH (n:SourceFragment {profile_id: $profileId}) RETURN n.status, count(*) GROUP BY n.status",
		},
		{
			name:  "simple RETURN without node patterns",
			query: "RETURN 1 AS value",
		},
		{
			name:  "case-insensitive profile_id parameter",
			query: "MATCH (n:SourceFragment {profile_id: $profileId}) RETURN n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.query)
			assert.NoError(t, err, "Query should be accepted: %s", tt.query)
		})
	}
}

// TestCypherValidator_Interface ensures cypherValidator implements CypherValidator.
func TestCypherValidator_Interface(t *testing.T) {
	var _ CypherValidator = (*cypherValidator)(nil)
}

// TestValidationError_Error tests the error message format.
func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{Reason: "test reason"}
	assert.Contains(t, err.Error(), "test reason")
	assert.Contains(t, err.Error(), "cypher validation failed")
}
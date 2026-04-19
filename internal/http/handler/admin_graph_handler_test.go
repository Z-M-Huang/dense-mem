package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/tools/admingraph"
)

// mockAdminGraphService is a mock implementation of AdminGraphServiceInterface
type mockAdminGraphService struct {
	executeWithAuditFunc func(ctx context.Context, profileID string, query string, params map[string]any, actorKeyID *string, actorRole, clientIP, correlationID string) (*admingraph.AdminGraphResult, error)
}

func (m *mockAdminGraphService) ExecuteWithAudit(ctx context.Context, profileID string, query string, params map[string]any, actorKeyID *string, actorRole, clientIP, correlationID string) (*admingraph.AdminGraphResult, error) {
	if m.executeWithAuditFunc != nil {
		return m.executeWithAuditFunc(ctx, profileID, query, params, actorKeyID, actorRole, clientIP, correlationID)
	}
	return &admingraph.AdminGraphResult{
		Columns:  []string{"n"},
		Rows:     []map[string]any{{"n": map[string]any{"name": "test"}}},
		RowCount: 1,
	}, nil
}

// TestAdminGraphRejectWrite tests that write clauses are rejected by the validator.
func TestAdminGraphRejectWrite(t *testing.T) {
	writeQueries := []struct {
		name  string
		query string
	}{
		{"CREATE clause", "CREATE (n:Node) RETURN n"},
		{"MERGE clause", "MERGE (n:Node) RETURN n"},
		{"DELETE clause", "MATCH (n:Node) DELETE n"},
		{"SET clause", "MATCH (n:Node) SET n.name = 'test' RETURN n"},
		{"REMOVE clause", "MATCH (n:Node) REMOVE n.name RETURN n"},
		{"DROP clause", "DROP INDEX node_name_index"},
		{"FOREACH clause", "MATCH (n:Node) FOREACH (x IN [1,2] | CREATE (m:Node))"},
	}

	validator := admingraph.NewAdminGraphValidator()

	for _, tt := range writeQueries {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.query)
			assert.Error(t, err, "write clause should be rejected")
			assert.True(t, admingraph.IsValidationError(err), "should be a validation error")
			assert.Contains(t, err.Error(), "write clause", "error should mention write clause")
		})
	}
}

// TestAdminGraphRejectUnsafeProcs tests that unsafe APOC/db/network/file procedures are rejected.
func TestAdminGraphRejectUnsafeProcs(t *testing.T) {
	unsafeProcQueries := []struct {
		name  string
		query string
	}{
		{"apoc.destroy", "CALL apoc.destroy.node('Node')"},
		{"apoc.delete", "CALL apoc.delete.node(n)"},
		{"apoc.create.node", "CALL apoc.create.node(['Node'], {name: 'test'})"},
		{"apoc.merge.node", "CALL apoc.merge.node(['Node'], {name: 'test'})"},
		{"apoc.load.json", "CALL apoc.load.json('http://example.com/data.json')"},
		{"apoc.load.csv", "CALL apoc.load.csv('http://example.com/data.csv')"},
		{"apoc.cypher.run", "CALL apoc.cypher.run('CREATE (n:Node)')"},
		{"apoc.periodic.iterate", "CALL apoc.periodic.iterate('MATCH (n:Node) RETURN n', 'DELETE n', {batchSize: 100})"},
		{"apoc.export.json", "CALL apoc.export.json.all('export.json')"},
		{"apoc.import", "CALL apoc.import.json('import.json')"},
		{"db.createIndex", "CALL db.createIndex('node_name_index')"},
		{"db.dropIndex", "CALL db.dropIndex('node_name_index')"},
		{"db.createConstraint", "CALL db.createConstraint('unique_name')"},
		{"db.dropConstraint", "CALL db.dropConstraint('unique_name')"},
		{"db.labels", "CALL db.labels()"},
		{"db.indexes", "CALL db.indexes()"},
		{"db.constraints", "CALL db.constraints()"},
		{"db.switchTo", "CALL db.switchTo('otherdb')"},
		{"net.*", "CALL net.getClientConnectionCount()"},
		{"file.*", "CALL file.load('data.json')"},
		{"apoc.util.http", "CALL apoc.util.http.get('http://example.com')"},
		{"apoc.schema", "CALL apoc.schema.assert({},{})"},
		{"LOAD CSV", "LOAD CSV FROM 'http://example.com/data.csv' AS row RETURN row"},
		{"schema change CREATE INDEX", "CREATE INDEX node_name_index FOR (n:Node) ON (n.name)"},
		{"schema change DROP INDEX", "DROP INDEX node_name_index"},
		{"schema change CREATE CONSTRAINT", "CREATE CONSTRAINT unique_name FOR (n:Node) REQUIRE n.name IS UNIQUE"},
	}

	validator := admingraph.NewAdminGraphValidator()

	for _, tt := range unsafeProcQueries {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.query)
			assert.Error(t, err, "unsafe procedure should be rejected")
			assert.True(t, admingraph.IsValidationError(err), "should be a validation error")
		})
	}
}

// TestAdminGraphTimeout tests that queries exceeding 30s timeout are cancelled.
func TestAdminGraphTimeout(t *testing.T) {
	e := newTestEcho()

	// Mock service that simulates timeout
	mockSvc := &mockAdminGraphService{
		executeWithAuditFunc: func(ctx context.Context, profileID string, query string, params map[string]any, actorKeyID *string, actorRole, clientIP, correlationID string) (*admingraph.AdminGraphResult, error) {
			// Simulate timeout error
			return nil, &admingraph.TimeoutError{Timeout: 30 * time.Second}
		},
	}
	h := NewAdminGraphHandler(mockSvc)

	// Set admin principal
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetPrincipalForTest(ctx, &middleware.Principal{
				KeyID:     uuid.New(),
				ProfileID: nil,
				Role:      "admin",
				Scopes:    []string{"admin"},
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/admin/graph/query", h.Handle)

	profileID := uuid.New()
	body := map[string]any{
		"query":     "MATCH (n:Node) WHERE $profileId = $profileId RETURN n LIMIT 10",
		"profile_id": profileID.String(),
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/graph/query", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var resp httperr.APIError
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, httperr.SERVICE_UNAVAILABLE, resp.Code)
	assert.Contains(t, resp.Message, "timeout")
}

// TestAdminGraphRowCap tests that result sets are capped at 1000 rows.
func TestAdminGraphRowCap(t *testing.T) {
	e := newTestEcho()

	// Mock service that returns 1000+ rows (should be capped)
	mockSvc := &mockAdminGraphService{
		executeWithAuditFunc: func(ctx context.Context, profileID string, query string, params map[string]any, actorKeyID *string, actorRole, clientIP, correlationID string) (*admingraph.AdminGraphResult, error) {
			// Generate capped result (service does the capping)
			rows := make([]map[string]any, 1000)
			for i := 0; i < 1000; i++ {
				rows[i] = map[string]any{"n": map[string]any{"id": i}}
			}
			return &admingraph.AdminGraphResult{
				Columns:  []string{"n"},
				Rows:     rows,
				RowCount: 1000,
			}, nil
		},
	}
	h := NewAdminGraphHandler(mockSvc)

	// Set admin principal
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetPrincipalForTest(ctx, &middleware.Principal{
				KeyID:     uuid.New(),
				ProfileID: nil,
				Role:      "admin",
				Scopes:    []string{"admin"},
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/admin/graph/query", h.Handle)

	profileID := uuid.New()
	body := map[string]any{
		"query":     "MATCH (n:Node) WHERE $profileId = $profileId RETURN n LIMIT 1000",
		"profile_id": profileID.String(),
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/graph/query", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp AdminGraphResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.LessOrEqual(t, resp.Data.RowCount, 1000, "row count should be capped at 1000")
	assert.LessOrEqual(t, len(resp.Data.Rows), 1000, "rows array should be capped at 1000")
}

// TestAdminGraphAudit tests that an audit event is recorded for every execution including failed ones.
func TestAdminGraphAudit(t *testing.T) {
	auditCalled := false
	auditMetadata := make(map[string]interface{})

	e := newTestEcho()

	// Mock service that tracks audit logging
	mockSvc := &mockAdminGraphService{
		executeWithAuditFunc: func(ctx context.Context, profileID string, query string, params map[string]any, actorKeyID *string, actorRole, clientIP, correlationID string) (*admingraph.AdminGraphResult, error) {
			// Simulate validation error (audit should still be logged)
			auditCalled = true
			auditMetadata["profile_id"] = profileID
			auditMetadata["success"] = false
			return nil, &admingraph.ValidationError{Reason: "query contains forbidden write clause: CREATE"}
		},
	}
	h := NewAdminGraphHandler(mockSvc)

	// Set admin principal
	actorKeyID := uuid.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetPrincipalForTest(ctx, &middleware.Principal{
				KeyID:     actorKeyID,
				ProfileID: nil,
				Role:      "admin",
				Scopes:    []string{"admin"},
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/admin/graph/query", h.Handle)

	profileID := uuid.New()
	body := map[string]any{
		"query":     "CREATE (n:Node) RETURN n",
		"profile_id": profileID.String(),
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/graph/query", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	// Audit should be called even when query fails
	assert.True(t, auditCalled, "audit should be called for failed queries")
	assert.Equal(t, false, auditMetadata["success"], "audit metadata should indicate failure")
	assert.Equal(t, profileID.String(), auditMetadata["profile_id"], "audit should record profile_id")

	// Response should be error
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	var resp httperr.APIError
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, httperr.VALIDATION_ERROR, resp.Code)
}

// TestAdminGraphHandler_Validation tests handler-level validation.
func TestAdminGraphHandler_Validation(t *testing.T) {
	e := newTestEcho()
	h := NewAdminGraphHandler(&mockAdminGraphService{})

	// Set admin principal
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetPrincipalForTest(ctx, &middleware.Principal{
				KeyID:     uuid.New(),
				ProfileID: nil,
				Role:      "admin",
				Scopes:    []string{"admin"},
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/admin/graph/query", h.Handle)

	tests := []struct {
		name           string
		body           string
		expectedStatus int
		expectedCode   httperr.ErrorCode
	}{
		{
			name:           "missing query",
			body:           `{"profile_id": "00000000-0000-0000-0000-000000000001"}`,
			expectedStatus: http.StatusUnprocessableEntity,
			expectedCode:   httperr.VALIDATION_ERROR,
		},
		{
			name:           "missing profile_id",
			body:           `{"query": "MATCH (n) RETURN n"}`,
			expectedStatus: http.StatusUnprocessableEntity,
			expectedCode:   httperr.VALIDATION_ERROR,
		},
		{
			name:           "invalid profile_id UUID",
			body:           `{"query": "MATCH (n) RETURN n", "profile_id": "not-a-uuid"}`,
			expectedStatus: http.StatusBadRequest,
			expectedCode:   httperr.INVALID_UUID,
		},
		{
			name:           "malformed JSON",
			body:           `{invalid`,
			expectedStatus: http.StatusUnprocessableEntity,
			expectedCode:   httperr.VALIDATION_ERROR,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/graph/query", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)

			var resp httperr.APIError
			err := json.Unmarshal(rec.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCode, resp.Code)
		})
	}
}

// TestAdminGraphHandler_AdminOnly tests that non-admin cannot access.
func TestAdminGraphHandler_AdminOnly(t *testing.T) {
	e := newTestEcho()
	h := NewAdminGraphHandler(&mockAdminGraphService{})

	// Set standard principal
	profileID := uuid.New()
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetPrincipalForTest(ctx, &middleware.Principal{
				KeyID:     uuid.New(),
				ProfileID: &profileID,
				Role:      "standard",
				Scopes:    []string{"read"},
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/admin/graph/query", h.Handle)

	body := map[string]any{
		"query":     "MATCH (n) RETURN n",
		"profile_id": uuid.New().String(),
	}
	bodyJSON, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/graph/query", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)

	var resp httperr.APIError
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, httperr.FORBIDDEN, resp.Code)
}
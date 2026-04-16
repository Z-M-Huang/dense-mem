package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/tools/graphquery"
)

// mockGraphQueryServiceForHandler implements GraphQueryServiceInterface for testing.
type mockGraphQueryServiceForHandler struct {
	executeFunc func(ctx context.Context, profileID string, query string, params map[string]any) (*graphquery.GraphQueryResult, error)
}

func (m *mockGraphQueryServiceForHandler) Execute(ctx context.Context, profileID string, query string, params map[string]any) (*graphquery.GraphQueryResult, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, profileID, query, params)
	}
	return &graphquery.GraphQueryResult{
		Columns: []string{},
		Rows:    []map[string]any{},
	}, nil
}

// TestGraphQueryHandler_AcceptsParameters tests that handler binds dto.GraphQueryRequest with "parameters" field.
func TestGraphQueryHandler_AcceptsParameters(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()

	receivedParams := map[string]any(nil)
	mockSvc := &mockGraphQueryServiceForHandler{
		executeFunc: func(ctx context.Context, pid string, query string, params map[string]any) (*graphquery.GraphQueryResult, error) {
			assert.Equal(t, profileID.String(), pid)
			assert.Equal(t, "MATCH (n) RETURN n LIMIT 1", query)
			receivedParams = params
			return &graphquery.GraphQueryResult{
				Columns: []string{"n"},
				Rows:    []map[string]any{{"n": "test"}},
				RowCount: 1,
			}, nil
		},
	}
	h := NewGraphQueryHandler(mockSvc)

	// Set resolved profile ID in context
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetResolvedProfileIDForTest(ctx, profileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/tools/graph-query", h.Handle)

	body := `{"query":"MATCH (n) RETURN n LIMIT 1","parameters":{"x":1}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/graph-query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, receivedParams)
	assert.Equal(t, map[string]any{"x": 1.0}, receivedParams)
}

// TestGraphQueryHandler_RejectsLegacyParamsField tests that "params" field is ignored (not populated into service).
func TestGraphQueryHandler_RejectsLegacyParamsField(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()

	receivedParams := map[string]any{}
	mockSvc := &mockGraphQueryServiceForHandler{
		executeFunc: func(ctx context.Context, pid string, query string, params map[string]any) (*graphquery.GraphQueryResult, error) {
			receivedParams = params
			return &graphquery.GraphQueryResult{
				Columns: []string{},
				Rows:    []map[string]any{},
				RowCount: 0,
			}, nil
		},
	}
	h := NewGraphQueryHandler(mockSvc)

	// Set resolved profile ID in context
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetResolvedProfileIDForTest(ctx, profileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/tools/graph-query", h.Handle)

	// Use legacy "params" field instead of "parameters"
	body := `{"query":"MATCH (n) RETURN n","params":{"x":1}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/graph-query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	// Request should still execute (not 500), but params must be empty/nil
	assert.NotEqual(t, http.StatusInternalServerError, rec.Code)
	// Parameters must NOT leak into service - should be nil or empty
	assert.True(t, receivedParams == nil || len(receivedParams) == 0,
		"params should not leak into service, got: %v", receivedParams)
}

// TestGraphQueryHandler_TimeoutSeconds tests that TimeoutSeconds is applied to context.
func TestGraphQueryHandler_TimeoutSeconds(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()

	timeoutReceived := false
	mockSvc := &mockGraphQueryServiceForHandler{
		executeFunc: func(ctx context.Context, pid string, query string, params map[string]any) (*graphquery.GraphQueryResult, error) {
			// Check if context has a deadline
			_, hasDeadline := ctx.Deadline()
			timeoutReceived = hasDeadline
			return &graphquery.GraphQueryResult{
				Columns: []string{},
				Rows:    []map[string]any{},
				RowCount: 0,
			}, nil
		},
	}
	h := NewGraphQueryHandler(mockSvc)

	// Set resolved profile ID in context
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetResolvedProfileIDForTest(ctx, profileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/tools/graph-query", h.Handle)

	body := `{"query":"MATCH (n) RETURN n","timeout_seconds":30}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/graph-query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, timeoutReceived, "context should have a deadline when timeout_seconds is set")
}

// TestGraphQueryHandler_TimeoutSecondsZero tests that TimeoutSeconds=0 does not set deadline.
func TestGraphQueryHandler_TimeoutSecondsZero(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()

	timeoutReceived := false
	mockSvc := &mockGraphQueryServiceForHandler{
		executeFunc: func(ctx context.Context, pid string, query string, params map[string]any) (*graphquery.GraphQueryResult, error) {
			// Check if context has a deadline
			_, hasDeadline := ctx.Deadline()
			timeoutReceived = hasDeadline
			return &graphquery.GraphQueryResult{
				Columns: []string{},
				Rows:    []map[string]any{},
				RowCount: 0,
			}, nil
		},
	}
	h := NewGraphQueryHandler(mockSvc)

	// Set resolved profile ID in context
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetResolvedProfileIDForTest(ctx, profileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/tools/graph-query", h.Handle)

	body := `{"query":"MATCH (n) RETURN n","timeout_seconds":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/graph-query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, timeoutReceived, "context should NOT have a deadline when timeout_seconds is 0")
}

// TestGraphQueryHandler_MissingQuery tests that missing query returns error.
func TestGraphQueryHandler_MissingQuery(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()

	mockSvc := &mockGraphQueryServiceForHandler{}
	h := NewGraphQueryHandler(mockSvc)

	// Set resolved profile ID in context
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetResolvedProfileIDForTest(ctx, profileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/tools/graph-query", h.Handle)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/graph-query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusOK, rec.Code)
}
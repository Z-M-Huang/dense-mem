package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/tools/keywordsearch"
)

// mockKeywordSearchService implements KeywordSearchServiceInterface for testing.
type mockKeywordSearchService struct {
	searchFunc func(ctx context.Context, profileID string, req *keywordsearch.KeywordSearchRequest) (*keywordsearch.KeywordSearchResult, error)
}

func (m *mockKeywordSearchService) Search(ctx context.Context, profileID string, req *keywordsearch.KeywordSearchRequest) (*keywordsearch.KeywordSearchResult, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, profileID, req)
	}
	return &keywordsearch.KeywordSearchResult{
		Data: []keywordsearch.SearchHit{},
		Meta: keywordsearch.KeywordSearchMeta{LimitApplied: 20},
	}, nil
}

// TestKeywordSearchHandler_Handle_Success tests successful keyword search.
func TestKeywordSearchHandler_Handle_Success(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()

	mockSvc := &mockKeywordSearchService{
		searchFunc: func(ctx context.Context, pid string, req *keywordsearch.KeywordSearchRequest) (*keywordsearch.KeywordSearchResult, error) {
			assert.Equal(t, profileID.String(), pid)
			assert.Equal(t, "test query", req.Query)
			assert.Equal(t, 20, req.Limit)
			return &keywordsearch.KeywordSearchResult{
				Data: []keywordsearch.SearchHit{
					{ID: "hit-1", Type: "fragment", Content: "test content", Score: 0.9, ProfileID: pid},
				},
				Meta: keywordsearch.KeywordSearchMeta{LimitApplied: 20},
			}, nil
		},
	}
	h := NewKeywordSearchHandler(mockSvc)

	// Set resolved profile ID in context using the proper key
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetResolvedProfileIDForTest(ctx, profileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/tools/keyword-search", h.Handle)

	body := `{"query": "test query", "limit": 20}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/keyword-search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Profile-ID", profileID.String())
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp keywordsearch.KeywordSearchResult
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, 20, resp.Meta.LimitApplied)
}

// TestKeywordSearchHandler_Handle_LimitZero_422 tests that limit=0 returns 422.
func TestKeywordSearchHandler_Handle_LimitZero_422(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()

	mockSvc := &mockKeywordSearchService{
		searchFunc: func(ctx context.Context, pid string, req *keywordsearch.KeywordSearchRequest) (*keywordsearch.KeywordSearchResult, error) {
			return nil, keywordsearch.NewValidationError("limit must be greater than 0")
		},
	}
	h := NewKeywordSearchHandler(mockSvc)

	// Set resolved profile ID in context using the proper key
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetResolvedProfileIDForTest(ctx, profileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/tools/keyword-search", h.Handle)

	body := `{"query": "test query", "limit": 0}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/keyword-search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Profile-ID", profileID.String())
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	var resp httperr.ErrorEnvelope
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, httperr.VALIDATION_ERROR, resp.Error.Code)
}

// TestKeywordSearchHandler_Handle_EmptyResult_200 tests that empty result returns 200 with {"data":[]}.
func TestKeywordSearchHandler_Handle_EmptyResult_200(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()

	mockSvc := &mockKeywordSearchService{
		searchFunc: func(ctx context.Context, pid string, req *keywordsearch.KeywordSearchRequest) (*keywordsearch.KeywordSearchResult, error) {
			return &keywordsearch.KeywordSearchResult{
				Data: []keywordsearch.SearchHit{},
				Meta: keywordsearch.KeywordSearchMeta{LimitApplied: 20},
			}, nil
		},
	}
	h := NewKeywordSearchHandler(mockSvc)

	// Set resolved profile ID in context using the proper key
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetResolvedProfileIDForTest(ctx, profileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/tools/keyword-search", h.Handle)

	body := `{"query": "nonexistent", "limit": 20}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/keyword-search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Profile-ID", profileID.String())
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp keywordsearch.KeywordSearchResult
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Empty(t, resp.Data)
	assert.Equal(t, []keywordsearch.SearchHit{}, resp.Data)
}

// TestKeywordSearchHandler_Handle_MissingProfileID tests that missing profile ID returns error.
func TestKeywordSearchHandler_Handle_MissingProfileID(t *testing.T) {
	e := newTestEcho()
	h := NewKeywordSearchHandler(&mockKeywordSearchService{})

	e.POST("/api/v1/tools/keyword-search", h.Handle)

	body := `{"query": "test", "limit": 20}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/keyword-search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No X-Profile-ID header
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	// The profile resolution middleware would catch this before the handler
	// But the handler also checks for it
	assert.NotEqual(t, http.StatusOK, rec.Code)
}

// TestKeywordSearchHandler_Handle_LimitCapped tests that limit over 100 is capped.
func TestKeywordSearchHandler_Handle_LimitCapped(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()

	mockSvc := &mockKeywordSearchService{
		searchFunc: func(ctx context.Context, pid string, req *keywordsearch.KeywordSearchRequest) (*keywordsearch.KeywordSearchResult, error) {
			// Limit 150 should be capped to 100
			assert.Equal(t, 150, req.Limit) // Request limit as received
			return &keywordsearch.KeywordSearchResult{
				Data: []keywordsearch.SearchHit{},
				Meta: keywordsearch.KeywordSearchMeta{LimitApplied: 100}, // Service caps it
			}, nil
		},
	}
	h := NewKeywordSearchHandler(mockSvc)

	// Set resolved profile ID in context using the proper key
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetResolvedProfileIDForTest(ctx, profileID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.POST("/api/v1/tools/keyword-search", h.Handle)

	body := `{"query": "test query", "limit": 150}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/keyword-search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Profile-ID", profileID.String())
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp keywordsearch.KeywordSearchResult
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 100, resp.Meta.LimitApplied, "limit should be capped to 100 in meta")
}
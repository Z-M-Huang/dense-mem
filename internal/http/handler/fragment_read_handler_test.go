package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
)

type mockGetFragmentService struct {
	getFunc func(ctx context.Context, profileID, fragmentID string) (*domain.Fragment, error)
}

func (m *mockGetFragmentService) GetByID(ctx context.Context, profileID, fragmentID string) (*domain.Fragment, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, profileID, fragmentID)
	}
	return nil, fragmentservice.ErrFragmentNotFound
}

func TestFragmentReadHandler_Returns200InScope(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockGetFragmentService{
		getFunc: func(ctx context.Context, pid, fid string) (*domain.Fragment, error) {
			if pid != profileID.String() {
				t.Errorf("service got profileID %q; want %q", pid, profileID.String())
			}
			return &domain.Fragment{
				FragmentID:          fid,
				ProfileID:           pid,
				Content:             "hello",
				SourceType:          domain.SourceTypeManual,
				ContentHash:         "abc",
				EmbeddingModel:      "m1",
				EmbeddingDimensions: 4,
			}, nil
		},
	}
	h := NewFragmentReadHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/fragments/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fragments/frag-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"embedding":`) {
		t.Errorf("response leaked embedding vector: %s", rec.Body.String())
	}
}

// TestFragmentReadHandler_Returns404ForOtherProfile — backpressure test + AC-27.
func TestFragmentReadHandler_Returns404ForOtherProfile(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockGetFragmentService{
		getFunc: func(ctx context.Context, pid, fid string) (*domain.Fragment, error) {
			// Simulate service returning ErrFragmentNotFound for cross-profile read
			return nil, fragmentservice.ErrFragmentNotFound
		},
	}
	h := NewFragmentReadHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/fragments/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fragments/frag-belongs-to-other", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404. body=%s", rec.Code, rec.Body.String())
	}
}

func TestFragmentReadHandler_Returns404OnMissing(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockGetFragmentService{} // default returns ErrFragmentNotFound
	h := NewFragmentReadHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/fragments/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fragments/does-not-exist", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", rec.Code)
	}
}

func TestFragmentReadHandler_Returns400WhenProfileMissing(t *testing.T) {
	e := echo.New()
	h := NewFragmentReadHandler(&mockGetFragmentService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.GET("/api/v1/fragments/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fragments/frag-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rec.Code)
	}
}

// Ensure the compile-time companion interface assertion stays in-package.
var _ FragmentReadHandlerInterface = (*FragmentReadHandler)(nil)

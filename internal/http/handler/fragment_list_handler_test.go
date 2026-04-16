package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/domain"
	dto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
)

type mockListService struct {
	listFunc func(ctx context.Context, profileID string, opts fragmentservice.ListOptions) ([]domain.Fragment, string, error)
}

func (m *mockListService) List(ctx context.Context, profileID string, opts fragmentservice.ListOptions) ([]domain.Fragment, string, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, profileID, opts)
	}
	return nil, "", nil
}

// TestFragmentListHandler_OrderedByCreatedAtDesc — backpressure test + AC-29.
func TestFragmentListHandler_OrderedByCreatedAtDesc(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	t1 := time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 16, 11, 0, 0, 0, time.UTC)
	svc := &mockListService{
		listFunc: func(ctx context.Context, pid string, opts fragmentservice.ListOptions) ([]domain.Fragment, string, error) {
			return []domain.Fragment{
				{FragmentID: "f2", ProfileID: pid, Content: "second", SourceType: domain.SourceTypeManual, CreatedAt: t2, UpdatedAt: t2},
				{FragmentID: "f1", ProfileID: pid, Content: "first", SourceType: domain.SourceTypeManual, CreatedAt: t1, UpdatedAt: t1},
			}, "", nil
		},
	}
	h := NewFragmentListHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/fragments", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fragments?limit=10", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
	}
	var body dto.ListFragmentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items = %d; want 2", len(body.Items))
	}
	if !body.Items[0].CreatedAt.After(body.Items[1].CreatedAt) {
		t.Error("items must be ordered by created_at DESC")
	}
	if body.HasMore {
		t.Error("HasMore should be false when nextCursor empty")
	}
}

func TestFragmentListHandler_HasMoreWhenNextCursorSet(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockListService{
		listFunc: func(ctx context.Context, pid string, opts fragmentservice.ListOptions) ([]domain.Fragment, string, error) {
			return []domain.Fragment{{FragmentID: "f1", ProfileID: pid, SourceType: domain.SourceTypeManual}}, "next-token", nil
		},
	}
	h := NewFragmentListHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/fragments", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fragments?limit=1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var body dto.ListFragmentsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.NextCursor != "next-token" {
		t.Errorf("NextCursor = %q; want next-token", body.NextCursor)
	}
	if !body.HasMore {
		t.Error("HasMore should be true when nextCursor set")
	}
}

func TestFragmentListHandler_Rejects422OnOversizedLimit(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	h := NewFragmentListHandler(&mockListService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/fragments", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fragments?limit=500", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want 422 for limit=500", rec.Code)
	}
}

func TestFragmentListHandler_RejectsInvalidSourceType(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	h := NewFragmentListHandler(&mockListService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/fragments", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fragments?source_type=invalid", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want 422", rec.Code)
	}
}

func TestFragmentListHandler_InvalidCursorMapsTo422(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockListService{
		listFunc: func(ctx context.Context, pid string, opts fragmentservice.ListOptions) ([]domain.Fragment, string, error) {
			return nil, "", fragmentservice.ErrInvalidCursor
		},
	}
	h := NewFragmentListHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/fragments", h.Handle)

	// Cursor must pass struct validation (<=256 chars). Service layer catches the invalid payload.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fragments?cursor=not-valid", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want 422 on invalid cursor", rec.Code)
	}
}

// Ensure companion interface assertion stays in-package.
var _ FragmentListHandlerInterface = (*FragmentListHandler)(nil)

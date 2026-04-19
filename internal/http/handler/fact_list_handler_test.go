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
	"github.com/dense-mem/dense-mem/internal/service/factservice"
)

// mockListFactsService implements factservice.ListFactsService for testing.
type mockListFactsService struct {
	listFunc func(ctx context.Context, profileID string, filters factservice.FactListFilters, limit int, cursor string) ([]*domain.Fact, string, error)
}

func (m *mockListFactsService) List(ctx context.Context, profileID string, filters factservice.FactListFilters, limit int, cursor string) ([]*domain.Fact, string, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, profileID, filters, limit, cursor)
	}
	return nil, "", nil
}

// Ensure mockListFactsService satisfies the interface.
var _ factservice.ListFactsService = (*mockListFactsService)(nil)

// TestFactListHandler_Returns200WithItems verifies a successful list returns 200 with items.
func TestFactListHandler_Returns200WithItems(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	now := time.Now().UTC()
	svc := &mockListFactsService{
		listFunc: func(ctx context.Context, pid string, filters factservice.FactListFilters, limit int, cursor string) ([]*domain.Fact, string, error) {
			return []*domain.Fact{
				{FactID: "f1", ProfileID: pid, Subject: "s", Predicate: "p", Object: "o", Status: domain.FactStatusActive, RecordedAt: now},
				{FactID: "f2", ProfileID: pid, Subject: "s2", Predicate: "p2", Object: "o2", Status: domain.FactStatusActive, RecordedAt: now},
			}, "", nil
		},
	}
	h := NewFactListHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/facts", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts?limit=10", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
	}
	var body dto.ListFactsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v. body=%s", err, rec.Body.String())
	}
	if len(body.Items) != 2 {
		t.Errorf("items = %d; want 2", len(body.Items))
	}
	if body.HasMore {
		t.Error("HasMore should be false when nextCursor is empty")
	}
}

// TestFactListHandler_Returns200EmptyList verifies an empty result is handled correctly.
func TestFactListHandler_Returns200EmptyList(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockListFactsService{} // default returns nil, "", nil
	h := NewFactListHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/facts", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
	}
	var body dto.ListFactsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Items) != 0 {
		t.Errorf("items = %d; want 0", len(body.Items))
	}
	if body.HasMore {
		t.Error("HasMore should be false for empty result")
	}
}

// TestFactListHandler_HasMoreWhenNextCursorSet verifies has_more and next_cursor when a cursor is returned.
func TestFactListHandler_HasMoreWhenNextCursorSet(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockListFactsService{
		listFunc: func(ctx context.Context, pid string, filters factservice.FactListFilters, limit int, cursor string) ([]*domain.Fact, string, error) {
			return []*domain.Fact{
				{FactID: "f1", ProfileID: pid, Status: domain.FactStatusActive},
			}, "next-token", nil
		},
	}
	h := NewFactListHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/facts", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts?limit=1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var body dto.ListFactsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.NextCursor != "next-token" {
		t.Errorf("NextCursor = %q; want next-token", body.NextCursor)
	}
	if !body.HasMore {
		t.Error("HasMore should be true when nextCursor is set")
	}
}

// TestFactListHandler_Rejects422OnLimitExceedsMax verifies limit > 100 is rejected by struct validator.
func TestFactListHandler_Rejects422OnLimitExceedsMax(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	h := NewFactListHandler(&mockListFactsService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/facts", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts?limit=101", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want 422 for limit=101", rec.Code)
	}
}

// TestFactListHandler_Rejects422OnInvalidStatus verifies invalid status value is rejected.
func TestFactListHandler_Rejects422OnInvalidStatus(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	h := NewFactListHandler(&mockListFactsService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/facts", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts?status=invalid", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want 422 for invalid status", rec.Code)
	}
}

// TestFactListHandler_Returns400WhenProfileMissing verifies missing profile ID returns 400.
func TestFactListHandler_Returns400WhenProfileMissing(t *testing.T) {
	e := echo.New()
	h := NewFactListHandler(&mockListFactsService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	// No middleware injecting profile ID.
	e.GET("/api/v1/facts", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rec.Code)
	}
}

// TestFactListHandler_CrossProfileIsolation verifies the handler passes only the
// resolved profileID to the service.
func TestFactListHandler_CrossProfileIsolation(t *testing.T) {
	e := echo.New()
	profileA := uuid.New()
	profileB := uuid.New()

	var capturedProfileID string
	svc := &mockListFactsService{
		listFunc: func(ctx context.Context, pid string, filters factservice.FactListFilters, limit int, cursor string) ([]*domain.Fact, string, error) {
			capturedProfileID = pid
			return nil, "", nil
		},
	}
	h := NewFactListHandler(svc)

	// Only profileA is injected via middleware.
	e.Use(injectProfileMiddleware(profileA))
	e.GET("/api/v1/facts", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
	}
	if capturedProfileID != profileA.String() {
		t.Errorf("service received profileID %q; want %q", capturedProfileID, profileA.String())
	}
	if capturedProfileID == profileB.String() {
		t.Error("service received profileB ID — cross-profile isolation violated")
	}
}

// TestFactListHandler_FiltersPassedToService verifies subject/predicate/status filters are forwarded.
func TestFactListHandler_FiltersPassedToService(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()

	var capturedFilters factservice.FactListFilters
	svc := &mockListFactsService{
		listFunc: func(ctx context.Context, pid string, filters factservice.FactListFilters, limit int, cursor string) ([]*domain.Fact, string, error) {
			capturedFilters = filters
			return nil, "", nil
		},
	}
	h := NewFactListHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/facts", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts?subject=sky&predicate=is&status=active", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
	}
	if capturedFilters.Subject != "sky" {
		t.Errorf("Subject = %q; want sky", capturedFilters.Subject)
	}
	if capturedFilters.Predicate != "is" {
		t.Errorf("Predicate = %q; want is", capturedFilters.Predicate)
	}
	if capturedFilters.Status != domain.FactStatusActive {
		t.Errorf("Status = %q; want active", capturedFilters.Status)
	}
}

// Compile-time companion interface check.
var _ FactListHandlerInterface = (*FactListHandler)(nil)

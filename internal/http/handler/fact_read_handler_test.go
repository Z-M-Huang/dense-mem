package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/factservice"
)

// mockGetFactService implements factservice.GetFactService for testing.
type mockGetFactService struct {
	getFunc func(ctx context.Context, profileID, factID string) (*domain.Fact, error)
}

func (m *mockGetFactService) Get(ctx context.Context, profileID, factID string) (*domain.Fact, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, profileID, factID)
	}
	return nil, factservice.ErrFactNotFound
}

// Ensure mockGetFactService satisfies the interface.
var _ factservice.GetFactService = (*mockGetFactService)(nil)

// TestFactReadHandler_Returns200InScope verifies a successful in-scope read returns 200.
func TestFactReadHandler_Returns200InScope(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockGetFactService{
		getFunc: func(ctx context.Context, pid, fid string) (*domain.Fact, error) {
			if pid != profileID.String() {
				t.Errorf("service got profileID %q; want %q", pid, profileID.String())
			}
			return &domain.Fact{
				FactID:    fid,
				ProfileID: pid,
				Subject:   "sky",
				Predicate: "is",
				Object:    "blue",
				Status:    domain.FactStatusActive,
			}, nil
		},
	}
	h := NewFactReadHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/facts/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts/fact-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
	}
}

// TestFactReadHandler_Returns404ForCrossProfile — profile isolation: cross-profile reads return 404.
func TestFactReadHandler_Returns404ForCrossProfile(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockGetFactService{
		getFunc: func(ctx context.Context, pid, fid string) (*domain.Fact, error) {
			// Service returns ErrFactNotFound for cross-profile reads (existence leakage prevention).
			return nil, factservice.ErrFactNotFound
		},
	}
	h := NewFactReadHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/facts/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts/other-profile-fact", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404. body=%s", rec.Code, rec.Body.String())
	}
}

// TestFactReadHandler_Returns404OnMissing verifies a missing fact returns 404.
func TestFactReadHandler_Returns404OnMissing(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockGetFactService{} // default returns ErrFactNotFound
	h := NewFactReadHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/facts/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts/does-not-exist", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", rec.Code)
	}
}

func TestFactReadHandler_TemporalFilterReturns404WhenFactOutsideWindow(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	validFrom := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	validTo := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	svc := &mockGetFactService{
		getFunc: func(ctx context.Context, pid, fid string) (*domain.Fact, error) {
			return &domain.Fact{
				FactID:    fid,
				ProfileID: pid,
				Status:    domain.FactStatusActive,
				ValidFrom: &validFrom,
				ValidTo:   &validTo,
			}, nil
		},
	}
	h := NewFactReadHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/facts/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts/fact-1?valid_at=2024-03-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404. body=%s", rec.Code, rec.Body.String())
	}
}

// TestFactReadHandler_Returns400WhenProfileMissing verifies missing profile ID returns 400.
func TestFactReadHandler_Returns400WhenProfileMissing(t *testing.T) {
	e := echo.New()
	h := NewFactReadHandler(&mockGetFactService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	// No middleware injecting profile ID.
	e.GET("/api/v1/facts/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts/fact-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rec.Code)
	}
}

// TestFactReadHandler_CrossProfileIsolation verifies the handler passes only the
// resolved profileID to the service — never any caller-supplied value.
func TestFactReadHandler_CrossProfileIsolation(t *testing.T) {
	e := echo.New()
	profileA := uuid.New()
	profileB := uuid.New()

	var capturedProfileID string
	svc := &mockGetFactService{
		getFunc: func(ctx context.Context, pid, fid string) (*domain.Fact, error) {
			capturedProfileID = pid
			return &domain.Fact{
				FactID:    fid,
				ProfileID: pid,
				Status:    domain.FactStatusActive,
			}, nil
		},
	}
	h := NewFactReadHandler(svc)

	// Only profileA is injected via middleware.
	e.Use(injectProfileMiddleware(profileA))
	e.GET("/api/v1/facts/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/facts/fact-1", nil)
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

// Compile-time companion interface check.
var _ FactReadHandlerInterface = (*FactReadHandler)(nil)

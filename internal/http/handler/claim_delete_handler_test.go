package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/claimservice"
)

// mockDeleteClaimService implements claimservice.DeleteClaimService for testing.
type mockDeleteClaimService struct {
	deleteFunc func(ctx context.Context, profileID string, claimID string) error
}

func (m *mockDeleteClaimService) Delete(ctx context.Context, profileID string, claimID string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, profileID, claimID)
	}
	return nil
}

// TestClaimDeleteHandler_Returns200OnSuccess verifies a successful delete returns 200 with status body.
func TestClaimDeleteHandler_Returns200OnSuccess(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockDeleteClaimService{}
	h := NewClaimDeleteHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.DELETE("/api/v1/claims/:id", h.Handle)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/claims/claim-abc", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v. body=%s", err, rec.Body.String())
	}
	if body["status"] != "deleted" {
		t.Errorf("body[status] = %q; want deleted", body["status"])
	}
}

// TestClaimDeleteHandler_Returns404OnNotFound verifies missing claim → 404.
func TestClaimDeleteHandler_Returns404OnNotFound(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockDeleteClaimService{
		deleteFunc: func(ctx context.Context, pid string, claimID string) error {
			return claimservice.ErrClaimNotFound
		},
	}
	h := NewClaimDeleteHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.DELETE("/api/v1/claims/:id", h.Handle)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/claims/nonexistent", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404. body=%s", rec.Code, rec.Body.String())
	}
}

// TestClaimDeleteHandler_Returns400WhenProfileMissing verifies missing profile → 400.
func TestClaimDeleteHandler_Returns400WhenProfileMissing(t *testing.T) {
	e := echo.New()
	h := NewClaimDeleteHandler(&mockDeleteClaimService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	// No middleware injecting profile ID.
	e.DELETE("/api/v1/claims/:id", h.Handle)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/claims/some-id", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400. body=%s", rec.Code, rec.Body.String())
	}
}

// TestClaimDeleteHandler_CrossProfileIsolation verifies the service receives only the resolved profileID.
// The service enforces isolation — the handler must pass the correct profile scope.
func TestClaimDeleteHandler_CrossProfileIsolation(t *testing.T) {
	e := echo.New()
	profileA := uuid.New()
	profileB := uuid.New()

	var capturedProfileID string
	svc := &mockDeleteClaimService{
		deleteFunc: func(ctx context.Context, pid string, claimID string) error {
			capturedProfileID = pid
			return nil
		},
	}
	h := NewClaimDeleteHandler(svc)

	// Only profileA is injected — profileB must not appear in the service call.
	e.Use(injectProfileMiddleware(profileA))
	e.DELETE("/api/v1/claims/:id", h.Handle)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/claims/claim-xyz", nil)
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
var _ ClaimDeleteHandlerInterface = (*ClaimDeleteHandler)(nil)

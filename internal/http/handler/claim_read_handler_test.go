package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/domain"
	dto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/claimservice"
)

// mockGetClaimService implements claimservice.GetClaimService for testing.
type mockGetClaimService struct {
	getFunc func(ctx context.Context, profileID string, claimID string) (*domain.Claim, error)
}

func (m *mockGetClaimService) Get(ctx context.Context, profileID string, claimID string) (*domain.Claim, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, profileID, claimID)
	}
	return &domain.Claim{
		ClaimID:   claimID,
		ProfileID: profileID,
	}, nil
}

// TestClaimReadHandler_Returns200OnFound verifies a found claim returns 200 with body.
func TestClaimReadHandler_Returns200OnFound(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	claimID := "claim-abc"
	svc := &mockGetClaimService{}
	h := NewClaimReadHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/claims/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/claims/"+claimID, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
	}
	var resp dto.ClaimResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v. body=%s", err, rec.Body.String())
	}
	if resp.ClaimID != claimID {
		t.Errorf("claim_id = %q; want %q", resp.ClaimID, claimID)
	}
}

// TestClaimReadHandler_Returns404OnNotFound verifies missing claim → 404.
func TestClaimReadHandler_Returns404OnNotFound(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockGetClaimService{
		getFunc: func(ctx context.Context, pid string, claimID string) (*domain.Claim, error) {
			return nil, claimservice.ErrClaimNotFound
		},
	}
	h := NewClaimReadHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/claims/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/claims/nonexistent", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404. body=%s", rec.Code, rec.Body.String())
	}
}

// TestClaimReadHandler_Returns400WhenProfileMissing verifies missing profile → 400.
func TestClaimReadHandler_Returns400WhenProfileMissing(t *testing.T) {
	e := echo.New()
	h := NewClaimReadHandler(&mockGetClaimService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	// No middleware injecting profile ID.
	e.GET("/api/v1/claims/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/claims/some-id", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400. body=%s", rec.Code, rec.Body.String())
	}
}

// TestClaimReadHandler_CrossProfileIsolation verifies that a claim belonging to profileB
// is not visible to profileA — the service returns ErrClaimNotFound for cross-profile reads,
// and the handler must surface that as 404 (not leak the existence of the resource).
func TestClaimReadHandler_CrossProfileIsolation(t *testing.T) {
	e := echo.New()
	profileA := uuid.New()
	profileB := uuid.New()

	// Service rejects cross-profile reads with ErrClaimNotFound.
	svc := &mockGetClaimService{
		getFunc: func(ctx context.Context, pid string, claimID string) (*domain.Claim, error) {
			if pid == profileB.String() {
				// This path should never be reached because profileA is injected.
				return &domain.Claim{ClaimID: claimID, ProfileID: pid}, nil
			}
			// profileA has no claim with this ID — simulate isolation.
			return nil, claimservice.ErrClaimNotFound
		},
	}
	h := NewClaimReadHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	// Only profileA is injected; profileB's claim must not be readable.
	e.Use(injectProfileMiddleware(profileA))
	e.GET("/api/v1/claims/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/claims/claim-owned-by-b", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("cross-profile read returned %d; want 404 (isolation must not leak existence)", rec.Code)
	}
}

// Compile-time companion interface check.
var _ ClaimReadHandlerInterface = (*ClaimReadHandler)(nil)

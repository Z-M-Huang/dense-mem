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

	"github.com/dense-mem/dense-mem/internal/domain"
	dto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/claimservice"
)

// mockCreateClaimService implements claimservice.CreateClaimService for testing.
type mockCreateClaimService struct {
	createFunc func(ctx context.Context, profileID string, claim *domain.Claim) (*claimservice.CreateResult, error)
}

func (m *mockCreateClaimService) Create(ctx context.Context, profileID string, claim *domain.Claim) (*claimservice.CreateResult, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, profileID, claim)
	}
	return &claimservice.CreateResult{
		Claim: &domain.Claim{
			ClaimID:     "claim-new",
			ProfileID:   profileID,
			SupportedBy: claim.SupportedBy,
		},
		Duplicate: false,
	}, nil
}

// TestClaimCreateHandler_Returns201OnCreate verifies a new claim returns 201.
func TestClaimCreateHandler_Returns201OnCreate(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockCreateClaimService{}
	h := NewClaimCreateHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.POST("/api/v1/claims", h.Handle)

	body := `{"supported_by":["` + uuid.New().String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/claims", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; want %d. body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp dto.ClaimResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v. body=%s", err, rec.Body.String())
	}
	if resp.ClaimID == "" {
		t.Error("response missing claim_id")
	}
}

// TestClaimCreateHandler_Returns200AndReplayHeaderOnDuplicate verifies idempotent replay.
func TestClaimCreateHandler_Returns200AndReplayHeaderOnDuplicate(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockCreateClaimService{
		createFunc: func(ctx context.Context, pid string, claim *domain.Claim) (*claimservice.CreateResult, error) {
			return &claimservice.CreateResult{
				Claim: &domain.Claim{
					ClaimID:   "claim-existing",
					ProfileID: pid,
				},
				Duplicate:   true,
				DuplicateOf: "claim-existing",
			}, nil
		},
	}
	h := NewClaimCreateHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.POST("/api/v1/claims", h.Handle)

	body := `{"supported_by":["` + uuid.New().String() + `"],"idempotency_key":"k1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/claims", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Idempotent-Replay"); got != "true" {
		t.Errorf("X-Idempotent-Replay = %q; want true", got)
	}
}

// TestClaimCreateHandler_Returns422OnMalformedJSON verifies malformed JSON → 422.
func TestClaimCreateHandler_Returns422OnMalformedJSON(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	h := NewClaimCreateHandler(&mockCreateClaimService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.POST("/api/v1/claims", h.Handle)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/claims", strings.NewReader(`{malformed`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want %d. body=%s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

// TestClaimCreateHandler_Returns422OnMissingSupportedBy verifies required field validation.
func TestClaimCreateHandler_Returns422OnMissingSupportedBy(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	h := NewClaimCreateHandler(&mockCreateClaimService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.POST("/api/v1/claims", h.Handle)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/claims", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want %d. body=%s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

// TestClaimCreateHandler_Returns400WhenProfileMissing verifies missing profile → 400.
func TestClaimCreateHandler_Returns400WhenProfileMissing(t *testing.T) {
	e := echo.New()
	h := NewClaimCreateHandler(&mockCreateClaimService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	// No middleware injecting profile ID.
	e.POST("/api/v1/claims", h.Handle)

	body := `{"supported_by":["` + uuid.New().String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/claims", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400 (PROFILE_ID_REQUIRED). body=%s", rec.Code, rec.Body.String())
	}
}

// TestClaimCreateHandler_Returns404OnMissingSupportingFragment verifies supporting_fragment_missing error → 404.
func TestClaimCreateHandler_Returns404OnMissingSupportingFragment(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockCreateClaimService{
		createFunc: func(ctx context.Context, pid string, claim *domain.Claim) (*claimservice.CreateResult, error) {
			return nil, claimservice.ErrSupportingFragmentMissing
		},
	}
	h := NewClaimCreateHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.POST("/api/v1/claims", h.Handle)

	body := `{"supported_by":["` + uuid.New().String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/claims", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404. body=%s", rec.Code, rec.Body.String())
	}
}

// TestClaimCreateHandler_ProfileIsolation verifies the handler forwards exactly the
// resolved profile ID to the service — cross-profile ID injection is not possible.
func TestClaimCreateHandler_ProfileIsolation(t *testing.T) {
	e := echo.New()
	profileA := uuid.New()
	profileB := uuid.New()

	var capturedProfileID string
	svc := &mockCreateClaimService{
		createFunc: func(ctx context.Context, pid string, claim *domain.Claim) (*claimservice.CreateResult, error) {
			capturedProfileID = pid
			return &claimservice.CreateResult{
				Claim: &domain.Claim{ClaimID: "c1", ProfileID: pid},
			}, nil
		},
	}
	h := NewClaimCreateHandler(svc)

	// Only profileA is resolved — profileB must not reach the service.
	e.Use(injectProfileMiddleware(profileA))
	e.POST("/api/v1/claims", h.Handle)

	body := `{"supported_by":["` + uuid.New().String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/claims", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; want 201. body=%s", rec.Code, rec.Body.String())
	}
	if capturedProfileID != profileA.String() {
		t.Errorf("service received profileID %q; want %q", capturedProfileID, profileA.String())
	}
	if capturedProfileID == profileB.String() {
		t.Error("service received profileB ID — cross-profile isolation violated")
	}
}

// TestClaimCreateHandler_Returns422WhenValidFromAfterValidTo verifies that the cross-field
// date validator registered in dto.init() is wired up through the handler's ValidateStruct
// call. ValidFrom after ValidTo must produce a 422 Unprocessable Entity.
func TestClaimCreateHandler_Returns422WhenValidFromAfterValidTo(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	h := NewClaimCreateHandler(&mockCreateClaimService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.POST("/api/v1/claims", h.Handle)

	// valid_to is before valid_from — should fail cross-field validation.
	body := `{
		"supported_by":["` + uuid.New().String() + `"],
		"valid_from":"2025-06-01T00:00:00Z",
		"valid_to":"2025-01-01T00:00:00Z"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/claims", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want 422 (ValidFrom after ValidTo). body=%s", rec.Code, rec.Body.String())
	}
}

// Compile-time companion interface check.
var _ ClaimCreateHandlerInterface = (*ClaimCreateHandler)(nil)

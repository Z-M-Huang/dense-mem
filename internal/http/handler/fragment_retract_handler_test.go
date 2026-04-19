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
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
)

// mockRetractFragmentService implements fragmentservice.RetractFragmentService for testing.
type mockRetractFragmentService struct {
	retractFunc func(ctx context.Context, profileID string, fragmentID string) error
}

func (m *mockRetractFragmentService) Retract(ctx context.Context, profileID string, fragmentID string) error {
	if m.retractFunc != nil {
		return m.retractFunc(ctx, profileID, fragmentID)
	}
	return nil
}

// TestFragmentRetractHandler covers AC-48: soft-tombstone of a fragment via
// POST /api/v1/fragments/:id/retract, including cross-profile isolation.
func TestFragmentRetractHandler(t *testing.T) {
	t.Run("Returns200WithStatusOnSuccess", func(t *testing.T) {
		e := echo.New()
		e.HTTPErrorHandler = httperr.ErrorHandler
		profileID := uuid.New()
		svc := &mockRetractFragmentService{}
		h := NewFragmentRetractHandler(svc)

		e.Use(injectProfileMiddleware(profileID))
		e.POST("/api/v1/fragments/:id/retract", h.Handle)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/fragments/frag-abc/retract", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
		}

		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v. body=%s", err, rec.Body.String())
		}
		if body["status"] != "retracted" {
			t.Errorf("body[status] = %q; want retracted", body["status"])
		}
	})

	t.Run("Returns404OnFragmentNotFound", func(t *testing.T) {
		e := echo.New()
		e.HTTPErrorHandler = httperr.ErrorHandler
		profileID := uuid.New()
		svc := &mockRetractFragmentService{
			retractFunc: func(ctx context.Context, pid string, fragmentID string) error {
				return fragmentservice.ErrFragmentNotFound
			},
		}
		h := NewFragmentRetractHandler(svc)

		e.Use(injectProfileMiddleware(profileID))
		e.POST("/api/v1/fragments/:id/retract", h.Handle)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/fragments/nonexistent/retract", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d; want 404. body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Returns400OnMissingProfileID", func(t *testing.T) {
		e := echo.New()
		e.HTTPErrorHandler = httperr.ErrorHandler
		svc := &mockRetractFragmentService{}
		h := NewFragmentRetractHandler(svc)

		// No injectProfileMiddleware — profile ID absent from context.
		e.POST("/api/v1/fragments/:id/retract", h.Handle)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/fragments/frag-abc/retract", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d; want 400. body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Returns500OnServiceError", func(t *testing.T) {
		e := echo.New()
		e.HTTPErrorHandler = httperr.ErrorHandler
		profileID := uuid.New()
		svc := &mockRetractFragmentService{
			retractFunc: func(ctx context.Context, pid string, fragmentID string) error {
				return context.DeadlineExceeded
			},
		}
		h := NewFragmentRetractHandler(svc)

		e.Use(injectProfileMiddleware(profileID))
		e.POST("/api/v1/fragments/:id/retract", h.Handle)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/fragments/frag-abc/retract", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d; want 500. body=%s", rec.Code, rec.Body.String())
		}
	})
}

// TestFragmentRetractHandler_CrossProfileIsolation verifies that a fragment
// belonging to profile A is not accessible (returns 404) when retracted under
// profile B. The handler must pass the profile ID it receives from context
// to the service so profile-scoped queries are enforced at the data layer.
func TestFragmentRetractHandler_CrossProfileIsolation(t *testing.T) {
	profileA := uuid.New()
	profileB := uuid.New()
	const fragmentID = "frag-owned-by-a"

	// Service simulates: fragment exists only for profileA.
	svc := &mockRetractFragmentService{
		retractFunc: func(ctx context.Context, pid string, id string) error {
			if pid == profileB.String() {
				return fragmentservice.ErrFragmentNotFound
			}
			return nil
		},
	}
	h := NewFragmentRetractHandler(svc)

	// Profile A can retract the fragment.
	{
		e := echo.New()
		e.HTTPErrorHandler = httperr.ErrorHandler
		e.Use(injectProfileMiddleware(profileA))
		e.POST("/api/v1/fragments/:id/retract", h.Handle)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/fragments/"+fragmentID+"/retract", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("profileA: status = %d; want 200. body=%s", rec.Code, rec.Body.String())
		}
	}

	// Profile B gets 404 for the same fragment — it is not visible across profiles.
	{
		e := echo.New()
		e.HTTPErrorHandler = httperr.ErrorHandler
		e.Use(injectProfileMiddleware(profileB))
		e.POST("/api/v1/fragments/:id/retract", h.Handle)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/fragments/"+fragmentID+"/retract", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("profileB: status = %d; want 404 (cross-profile isolation). body=%s", rec.Code, rec.Body.String())
		}

		// Confirm profile B's results do not contain profile A's fragment ID.
		bBody := rec.Body.String()
		if bBody != "" {
			var bResult map[string]any
			_ = json.Unmarshal([]byte(bBody), &bResult)
			if id, ok := bResult["id"]; ok {
				if id == fragmentID {
					t.Errorf("profileB response must not contain profileA's fragmentID %q", fragmentID)
				}
			}
		}
	}
}

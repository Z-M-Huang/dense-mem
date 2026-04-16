package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
)

type mockDeleteFragmentService struct {
	deleteFunc func(ctx context.Context, profileID, fragmentID string) error
	called     bool
	lastProfile string
	lastID     string
}

func (m *mockDeleteFragmentService) Delete(ctx context.Context, profileID, fragmentID string) error {
	m.called = true
	m.lastProfile = profileID
	m.lastID = fragmentID
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, profileID, fragmentID)
	}
	return nil
}

// TestFragmentDeleteHandler_EmitsAuditEvent — backpressure test + AC-31.
// The handler must invoke the delete service (which emits the audit event) with
// the caller's profile scope and return 204 on success.
func TestFragmentDeleteHandler_EmitsAuditEvent(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockDeleteFragmentService{
		deleteFunc: func(ctx context.Context, pid, fid string) error {
			if pid != profileID.String() {
				t.Errorf("service received profile %q; want %q", pid, profileID.String())
			}
			if fid != "frag-to-delete" {
				t.Errorf("service received fragment id %q; want frag-to-delete", fid)
			}
			return nil
		},
	}
	h := NewFragmentDeleteHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.DELETE("/api/v1/fragments/:id", h.Handle)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/fragments/frag-to-delete", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d; want 204. body=%s", rec.Code, rec.Body.String())
	}
	if !svc.called {
		t.Error("delete service was not invoked — audit event would not fire")
	}
	if svc.lastProfile != profileID.String() {
		t.Errorf("service profile = %q; want %q", svc.lastProfile, profileID.String())
	}
}

func TestFragmentDeleteHandler_Returns404ForMissing(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockDeleteFragmentService{
		deleteFunc: func(ctx context.Context, pid, fid string) error {
			return fragmentservice.ErrFragmentNotFound
		},
	}
	h := NewFragmentDeleteHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.DELETE("/api/v1/fragments/:id", h.Handle)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/fragments/missing", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", rec.Code)
	}
}

func TestFragmentDeleteHandler_Returns404ForCrossProfile(t *testing.T) {
	// Cross-profile deletes are indistinguishable from missing fragments (AC-31).
	e := echo.New()
	profileID := uuid.New()
	svc := &mockDeleteFragmentService{
		deleteFunc: func(ctx context.Context, pid, fid string) error {
			return fragmentservice.ErrFragmentNotFound
		},
	}
	h := NewFragmentDeleteHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.DELETE("/api/v1/fragments/:id", h.Handle)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/fragments/frag-owned-by-other", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404 (existence must not leak across profiles)", rec.Code)
	}
}

func TestFragmentDeleteHandler_Returns400WhenProfileMissing(t *testing.T) {
	e := echo.New()
	h := NewFragmentDeleteHandler(&mockDeleteFragmentService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.DELETE("/api/v1/fragments/:id", h.Handle)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/fragments/frag-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", rec.Code)
	}
}

func TestFragmentDeleteHandler_Returns500OnServiceError(t *testing.T) {
	e := echo.New()
	profileID := uuid.New()
	svc := &mockDeleteFragmentService{
		deleteFunc: func(ctx context.Context, pid, fid string) error {
			return errors.New("neo4j down")
		},
	}
	h := NewFragmentDeleteHandler(svc)
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.DELETE("/api/v1/fragments/:id", h.Handle)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/fragments/frag-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; want 500", rec.Code)
	}
}

var _ FragmentDeleteHandlerInterface = (*FragmentDeleteHandler)(nil)

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/communityservice"
)

// mockDetectCommunityService implements communityservice.DetectCommunityService for testing.
type mockDetectCommunityService struct {
	lastProfile string
	lastOptions communityservice.DetectOptions
	detectFunc  func(ctx context.Context, profileID string, opts communityservice.DetectOptions) error
}

func (m *mockDetectCommunityService) Detect(ctx context.Context, profileID string, opts communityservice.DetectOptions) error {
	m.lastProfile = profileID
	m.lastOptions = opts
	if m.detectFunc != nil {
		return m.detectFunc(ctx, profileID, opts)
	}
	return nil
}

type mockListCommunitiesService struct {
	listFunc func(ctx context.Context, profileID string, limit int) ([]*domain.Community, error)
}

func (m *mockListCommunitiesService) List(ctx context.Context, profileID string, limit int) ([]*domain.Community, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, profileID, limit)
	}
	return []*domain.Community{{CommunityID: "42", ProfileID: profileID, MemberCount: 3}}, nil
}

// injectAdminPrincipal returns a middleware that injects an admin principal into
// the request context, simulating the AuthMiddleware + AdminOnly chain for tests.
func injectAdminPrincipal() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()
			ctx = middleware.SetPrincipalForTest(ctx, &middleware.Principal{
				KeyID:     uuid.New(),
				ProfileID: nil,
				Role:      "admin",
				Scopes:    []string{"admin"},
			})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

// TestCommunityDetectHandler covers AC-49 and AC-50: admin community detect handler.
func TestCommunityDetectHandler(t *testing.T) {
	const routePattern = "/api/v1/admin/profiles/:profileId/community/detect"

	t.Run("Returns200OnSuccess", func(t *testing.T) {
		e := newTestEcho()
		svc := &mockDetectCommunityService{}
		h := NewCommunityDetectHandler(svc, &mockListCommunitiesService{})

		e.Use(injectAdminPrincipal())
		e.POST(routePattern, h.Handle)

		profileID := uuid.New()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/profiles/"+profileID.String()+"/community/detect", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

		var body struct {
			Data struct {
				Detected bool `json:"detected"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.True(t, body.Data.Detected, "response must include detected:true")
		require.Equal(t, profileID.String(), svc.lastProfile)
		require.Equal(t, communityservice.DetectOptions{}, svc.lastOptions)
	})

	t.Run("PassesTuningOptionsToService", func(t *testing.T) {
		e := newTestEcho()
		svc := &mockDetectCommunityService{}
		h := NewCommunityDetectHandler(svc, &mockListCommunitiesService{})

		e.Use(injectAdminPrincipal())
		e.POST(routePattern, h.Handle)

		profileID := uuid.New()
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/admin/profiles/"+profileID.String()+"/community/detect",
			strings.NewReader(`{"gamma":1.6,"tolerance":0.0002,"max_levels":7}`),
		)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
		require.Equal(t, profileID.String(), svc.lastProfile)
		require.Equal(t, communityservice.DetectOptions{
			Gamma:     1.6,
			Tolerance: 0.0002,
			MaxLevels: 7,
		}, svc.lastOptions)
	})

	t.Run("Returns401WhenNoPrincipal", func(t *testing.T) {
		e := newTestEcho()
		h := NewCommunityDetectHandler(&mockDetectCommunityService{}, &mockListCommunitiesService{})
		// No principal middleware — context has no principal.
		e.POST(routePattern, h.Handle)

		profileID := uuid.New()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/profiles/"+profileID.String()+"/community/detect", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code, "body=%s", rec.Body.String())

		var resp httperr.APIError
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, httperr.AUTH_MISSING, resp.Code)
	})

	t.Run("Returns403ForNonAdmin", func(t *testing.T) {
		e := newTestEcho()
		h := NewCommunityDetectHandler(&mockDetectCommunityService{}, &mockListCommunitiesService{})
		profileID := uuid.New()

		// Inject a standard (non-admin) principal.
		e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				ctx := c.Request().Context()
				ctx = middleware.SetPrincipalForTest(ctx, &middleware.Principal{
					KeyID:     uuid.New(),
					ProfileID: &profileID,
					Role:      "standard",
					Scopes:    []string{"read"},
				})
				c.SetRequest(c.Request().WithContext(ctx))
				return next(c)
			}
		})
		e.POST(routePattern, h.Handle)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/profiles/"+profileID.String()+"/community/detect", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusForbidden, rec.Code, "body=%s", rec.Body.String())

		var resp httperr.APIError
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, httperr.FORBIDDEN, resp.Code)
	})

	t.Run("Returns400OnInvalidUUID", func(t *testing.T) {
		e := newTestEcho()
		h := NewCommunityDetectHandler(&mockDetectCommunityService{}, &mockListCommunitiesService{})
		e.Use(injectAdminPrincipal())
		e.POST(routePattern, h.Handle)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/profiles/not-a-uuid/community/detect", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())

		var resp httperr.APIError
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, httperr.INVALID_UUID, resp.Code)
	})

	t.Run("Returns503WhenGDSUnavailable", func(t *testing.T) {
		e := newTestEcho()
		svc := &mockDetectCommunityService{
			detectFunc: func(ctx context.Context, pid string, opts communityservice.DetectOptions) error {
				return communityservice.ErrCommunityUnavailable
			},
		}
		h := NewCommunityDetectHandler(svc, &mockListCommunitiesService{})
		e.Use(injectAdminPrincipal())
		e.POST(routePattern, h.Handle)

		profileID := uuid.New()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/profiles/"+profileID.String()+"/community/detect", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusServiceUnavailable, rec.Code, "body=%s", rec.Body.String())

		var resp httperr.APIError
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, httperr.SERVICE_UNAVAILABLE, resp.Code)
	})

	t.Run("Returns422WhenGraphTooLarge", func(t *testing.T) {
		e := newTestEcho()
		svc := &mockDetectCommunityService{
			detectFunc: func(ctx context.Context, pid string, opts communityservice.DetectOptions) error {
				return communityservice.ErrCommunityGraphTooLarge
			},
		}
		h := NewCommunityDetectHandler(svc, &mockListCommunitiesService{})
		e.Use(injectAdminPrincipal())
		e.POST(routePattern, h.Handle)

		profileID := uuid.New()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/profiles/"+profileID.String()+"/community/detect", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "body=%s", rec.Body.String())

		var resp httperr.APIError
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, httperr.ErrCommunityGraphTooLarge, resp.Code)
	})

	t.Run("Returns500OnUnexpectedError", func(t *testing.T) {
		e := newTestEcho()
		svc := &mockDetectCommunityService{
			detectFunc: func(ctx context.Context, pid string, opts communityservice.DetectOptions) error {
				return context.DeadlineExceeded
			},
		}
		h := NewCommunityDetectHandler(svc, &mockListCommunitiesService{})
		e.Use(injectAdminPrincipal())
		e.POST(routePattern, h.Handle)

		profileID := uuid.New()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/profiles/"+profileID.String()+"/community/detect", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusInternalServerError, rec.Code, "body=%s", rec.Body.String())
	})

	t.Run("Returns400OnInvalidTuningParameters", func(t *testing.T) {
		e := newTestEcho()
		h := NewCommunityDetectHandler(&mockDetectCommunityService{}, &mockListCommunitiesService{})
		e.Use(injectAdminPrincipal())
		e.POST(routePattern, h.Handle)

		profileID := uuid.New()
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/admin/profiles/"+profileID.String()+"/community/detect",
			strings.NewReader(`{"gamma":-1}`),
		)
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "body=%s", rec.Body.String())
	})
}

// TestCommunityDetectHandler_CrossProfileIsolation verifies that the handler
// forwards exactly the profileId from the URL to the service, so community
// detection for profile B never touches profile A's graph nodes.
//
// This test satisfies the profile isolation invariant from
// .claude/rules/profile-isolation.md: service methods receive profileID as an
// explicit parameter and the handler must not substitute or leak the value.
func TestCommunityDetectHandler_CrossProfileIsolation(t *testing.T) {
	const routePattern = "/api/v1/admin/profiles/:profileId/community/detect"
	profileA := uuid.New()
	profileB := uuid.New()

	// capturedIDs records every profileID the service receives.
	var capturedIDs []string
	svc := &mockDetectCommunityService{
		detectFunc: func(ctx context.Context, pid string, opts communityservice.DetectOptions) error {
			capturedIDs = append(capturedIDs, pid)
			return nil
		},
	}
	h := NewCommunityDetectHandler(svc, &mockListCommunitiesService{})

	makeRequest := func(profileID uuid.UUID) *httptest.ResponseRecorder {
		e := newTestEcho()
		e.Use(injectAdminPrincipal())
		e.POST(routePattern, h.Handle)

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/admin/profiles/"+profileID.String()+"/community/detect",
			nil,
		)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	// Run detection for profile B; profile A's ID must never reach the service.
	recB := makeRequest(profileB)
	require.Equal(t, http.StatusOK, recB.Code, "profileB: body=%s", recB.Body.String())

	// The service must have been called exactly once with profile B's ID.
	bResults := capturedIDs
	aID := profileA.String()
	require.NotContains(t, bResults, aID,
		"service must not be called with profileA's ID when request is for profileB")
	require.Contains(t, bResults, profileB.String(),
		"service must be called with profileB's ID")
}

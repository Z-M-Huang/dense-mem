package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dense-mem/dense-mem/internal/domain"
	dto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/communityservice"
	"github.com/google/uuid"
)

type mockCommunityGetService struct {
	getFunc func(ctx context.Context, profileID string, communityID string) (*domain.Community, error)
}

func (m *mockCommunityGetService) Get(ctx context.Context, profileID string, communityID string) (*domain.Community, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, profileID, communityID)
	}
	return nil, communityservice.ErrCommunityNotFound
}

var _ communityservice.GetCommunitySummaryService = (*mockCommunityGetService)(nil)

func TestCommunityReadHandler_Returns200OnFound(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()
	svc := &mockCommunityGetService{
		getFunc: func(ctx context.Context, pid string, communityID string) (*domain.Community, error) {
			return &domain.Community{CommunityID: communityID, ProfileID: pid, MemberCount: 3}, nil
		},
	}
	h := NewCommunityReadHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/communities/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/communities/c-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
	}
	var body dto.CommunityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.CommunityID != "c-1" {
		t.Fatalf("community_id = %q; want c-1", body.CommunityID)
	}
}

func TestCommunityReadHandler_Returns404OnMissing(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()
	h := NewCommunityReadHandler(&mockCommunityGetService{})
	e.HTTPErrorHandler = httperr.ErrorHandler

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/communities/:id", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/communities/missing", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", rec.Code)
	}
}

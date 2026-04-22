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

type mockCommunityListService struct {
	listFunc func(ctx context.Context, profileID string, limit int) ([]*domain.Community, error)
}

func (m *mockCommunityListService) List(ctx context.Context, profileID string, limit int) ([]*domain.Community, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, profileID, limit)
	}
	return nil, nil
}

var _ communityservice.ListCommunitiesService = (*mockCommunityListService)(nil)

func TestCommunityListHandler_Returns200WithItems(t *testing.T) {
	e := newTestEcho()
	profileID := uuid.New()
	svc := &mockCommunityListService{
		listFunc: func(ctx context.Context, pid string, limit int) ([]*domain.Community, error) {
			return []*domain.Community{
				{CommunityID: "c1", ProfileID: pid, MemberCount: 3},
				{CommunityID: "c2", ProfileID: pid, MemberCount: 2},
			}, nil
		},
	}
	h := NewCommunityListHandler(svc)

	e.Use(injectProfileMiddleware(profileID))
	e.GET("/api/v1/communities", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/communities?limit=10", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200. body=%s", rec.Code, rec.Body.String())
	}
	var body dto.ListCommunitiesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items = %d; want 2", len(body.Items))
	}
	if body.Total != 2 {
		t.Fatalf("total = %d; want 2", body.Total)
	}
}

func TestCommunityListHandler_Returns400WhenProfileMissing(t *testing.T) {
	e := newTestEcho()
	h := NewCommunityListHandler(&mockCommunityListService{})
	e.HTTPErrorHandler = httperr.ErrorHandler
	e.GET("/api/v1/communities", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/communities", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

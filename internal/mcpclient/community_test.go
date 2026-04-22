package mcpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dense-mem/dense-mem/internal/service/communityservice"
	"github.com/stretchr/testify/require"
)

func TestCommunityAdapters(t *testing.T) {
	ctx := context.Background()
	const authToken = "test-token"
	const profileID = "profile-A"

	t.Run("GetCommunitySummary", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+authToken, r.Header.Get("Authorization"))
			require.Equal(t, profileID, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"community_id":"42","profile_id":"profile-A","level":0,"summary":"s","summary_version":"community-deterministic-v1","member_count":3,"top_entities":["alice"],"top_predicates":["knows"],"last_summarized_at":"2026-04-22T12:00:00Z"}`))
		}))
		defer srv.Close()

		client := NewClient(srv.URL, authToken, profileID)
		got, err := NewCommunityGet(client).Get(ctx, profileID, "42")
		require.NoError(t, err)
		require.Equal(t, "42", got.CommunityID)
		require.Equal(t, 3, got.MemberCount)
		require.Equal(t, time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC), got.LastSummarizedAt)
	})

	t.Run("GetCommunitySummaryNotFound", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		client := NewClient(srv.URL, authToken, profileID)
		_, err := NewCommunityGet(client).Get(ctx, profileID, "missing")
		require.True(t, errors.Is(err, communityservice.ErrCommunityNotFound))
	})

	t.Run("ListCommunities", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+authToken, r.Header.Get("Authorization"))
			require.Equal(t, profileID, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, "5", r.URL.Query().Get("limit"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"community_id":"42","profile_id":"profile-A","level":0,"summary":"s","summary_version":"community-deterministic-v1","member_count":3,"last_summarized_at":"2026-04-22T12:00:00Z"}],"total":1}`))
		}))
		defer srv.Close()

		client := NewClient(srv.URL, authToken, profileID)
		got, err := NewCommunityList(client).List(ctx, profileID, 5)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "42", got[0].CommunityID)
	})
}

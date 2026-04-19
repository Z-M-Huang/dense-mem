package mcpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dense-mem/dense-mem/internal/domain"
	httpDto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
	"github.com/dense-mem/dense-mem/internal/service/recallservice"
	"github.com/dense-mem/dense-mem/internal/tools/graphquery"
	"github.com/dense-mem/dense-mem/internal/tools/keywordsearch"
	"github.com/dense-mem/dense-mem/internal/tools/semanticsearch"
	"github.com/stretchr/testify/require"
)

const (
	// testAuthToken is a fixture value used only in unit tests; never a real credential.
	testAuthToken = "fixture-token-for-tests"
	testProfileA  = "profile-A"
)

// TestExistingToolAdapters verifies all 7 HTTP adapter constructors against
// AC-55 (adapters implement service interfaces), AC-59 (correct headers sent),
// and R4 (cross-profile isolation: X-Profile-ID is never mixed between profiles).
func TestExistingToolAdapters(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateFragment_sends_correct_headers_and_parses_response", func(t *testing.T) {
		fragResp := httpDto.FragmentResponse{
			ID:         "frag-001",
			FragmentID: "frag-001",
			Content:    "hello world",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, testAuthToken, r.Header.Get("X-API-Key"), "X-API-Key must be set")
			require.Equal(t, testProfileA, r.Header.Get("X-Profile-ID"), "X-Profile-ID must match caller")
			require.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if err := json.NewEncoder(w).Encode(fragResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, testProfileA)
		svc := NewFragmentCreate(c)
		result, err := svc.Create(ctx, testProfileA, &httpDto.CreateFragmentRequest{Content: "hello world"})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "frag-001", result.Fragment.FragmentID)
		require.Equal(t, testProfileA, result.Fragment.ProfileID, "ProfileID must be populated from caller")
		require.False(t, result.Duplicate)
	})

	t.Run("CreateFragment_detects_duplicate_via_X_Idempotent_Replay_header", func(t *testing.T) {
		fragResp := httpDto.FragmentResponse{
			ID: "frag-001", FragmentID: "frag-001", Content: "hello",
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Idempotent-Replay", "true")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(fragResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, testProfileA)
		svc := NewFragmentCreate(c)
		result, err := svc.Create(ctx, testProfileA, &httpDto.CreateFragmentRequest{Content: "hello"})
		require.NoError(t, err)
		require.True(t, result.Duplicate, "X-Idempotent-Replay: true must set Duplicate=true")
	})

	t.Run("GetFragment_sends_correct_headers_and_returns_fragment", func(t *testing.T) {
		fragResp := httpDto.FragmentResponse{
			ID: "frag-002", FragmentID: "frag-002", Content: "get me",
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, testAuthToken, r.Header.Get("X-API-Key"))
			require.Equal(t, testProfileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(fragResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, testProfileA)
		svc := NewFragmentGet(c)
		frag, err := svc.GetByID(ctx, testProfileA, "frag-002")
		require.NoError(t, err)
		require.NotNil(t, frag)
		require.Equal(t, "frag-002", frag.FragmentID)
		require.Equal(t, testProfileA, frag.ProfileID)
	})

	t.Run("GetFragment_returns_ErrFragmentNotFound_on_404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, testProfileA)
		svc := NewFragmentGet(c)
		_, err := svc.GetByID(ctx, testProfileA, "missing")
		require.Error(t, err)
		require.ErrorIs(t, err, fragmentservice.ErrFragmentNotFound,
			"404 from server must surface as ErrFragmentNotFound")
	})

	t.Run("ListFragments_sends_correct_headers_and_returns_fragments", func(t *testing.T) {
		listResp := httpDto.ListFragmentsResponse{
			Items: []httpDto.FragmentResponse{
				{ID: "frag-003", FragmentID: "frag-003", Content: "list me",
					CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
			NextCursor: "next-cursor",
			HasMore:    true,
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, testAuthToken, r.Header.Get("X-API-Key"))
			require.Equal(t, testProfileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(listResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, testProfileA)
		svc := NewFragmentList(c)
		frags, cursor, err := svc.List(ctx, testProfileA, fragmentservice.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, frags, 1)
		require.Equal(t, "frag-003", frags[0].FragmentID)
		require.Equal(t, testProfileA, frags[0].ProfileID)
		require.Equal(t, "next-cursor", cursor)
	})

	t.Run("Recall_sends_correct_headers_and_returns_hits", func(t *testing.T) {
		frag := &domain.Fragment{FragmentID: "frag-r1", Content: "recall content"}
		recallResp := recallHTTPResponse{
			Data: []recallservice.RecallHit{
				{Fragment: frag, SemanticRank: 1, FinalScore: 0.9},
			},
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, testAuthToken, r.Header.Get("X-API-Key"))
			require.Equal(t, testProfileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(recallResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, testProfileA)
		svc := NewRecall(c)
		hits, err := svc.Recall(ctx, testProfileA, recallservice.RecallRequest{Query: "recall me", Limit: 5})
		require.NoError(t, err)
		require.Len(t, hits, 1)
		require.NotNil(t, hits[0].Fragment)
		// domain.Fragment.FragmentID has json:"id" — verify it survives the JSON round-trip.
		require.Equal(t, "frag-r1", hits[0].Fragment.FragmentID)
		require.Equal(t, float64(0.9), hits[0].FinalScore)
	})

	t.Run("KeywordSearch_sends_correct_headers_and_returns_results", func(t *testing.T) {
		kwResult := keywordsearch.KeywordSearchResult{
			Data: []keywordsearch.SearchHit{{ID: "hit-1", Content: "kw hit"}},
			Meta: keywordsearch.KeywordSearchMeta{LimitApplied: 10},
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, testAuthToken, r.Header.Get("X-API-Key"))
			require.Equal(t, testProfileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(kwResult); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, testProfileA)
		svc := NewKeywordSearch(c)
		result, err := svc.Search(ctx, testProfileA, &keywordsearch.KeywordSearchRequest{Query: "hello", Limit: 10})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Data, 1)
		require.Equal(t, "hit-1", result.Data[0].ID)
	})

	t.Run("SemanticSearch_sends_correct_headers_and_returns_results", func(t *testing.T) {
		semResult := semanticsearch.SemanticSearchResult{
			Data: []semanticsearch.SearchHit{{ID: "sem-1", Content: "semantic hit"}},
			Meta: semanticsearch.SemanticSearchMeta{LimitApplied: 10},
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, testAuthToken, r.Header.Get("X-API-Key"))
			require.Equal(t, testProfileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(semResult); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, testProfileA)
		svc := NewSemanticSearch(c)
		result, err := svc.Search(ctx, testProfileA, &semanticsearch.SemanticSearchRequest{
			Embedding: []float32{0.1, 0.2, 0.3},
			Limit:     10,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Data, 1)
		require.Equal(t, "sem-1", result.Data[0].ID)
	})

	t.Run("GraphQuery_sends_correct_headers_and_returns_result", func(t *testing.T) {
		var gqResp graphQueryHTTPResponse
		gqResp.Data.Columns = []string{"n"}
		gqResp.Data.Rows = []map[string]any{{"n": "value"}}
		gqResp.Meta.RowCount = 1
		gqResp.Meta.RowCapApplied = false

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, testAuthToken, r.Header.Get("X-API-Key"))
			require.Equal(t, testProfileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(gqResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, testProfileA)
		svc := NewGraphQuery(c)
		result, err := svc.Execute(ctx, testProfileA, "MATCH (n {profile_id: $profileId}) RETURN n", nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, 1, result.RowCount)
		require.Equal(t, []string{"n"}, result.Columns)
	})

	// Cross-profile isolation (R4 / profile-isolation.md):
	// Two distinct clients bound to different profiles must never exchange profile IDs.
	// The adapter sends the caller-supplied profileID in X-Profile-ID on every request.
	t.Run("CrossProfileIsolation", func(t *testing.T) {
		const profileB = "profile-B"

		capturedProfiles := make([]string, 0, 2)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedProfiles = append(capturedProfiles, r.Header.Get("X-Profile-ID"))
			w.WriteHeader(http.StatusNotFound) // ErrFragmentNotFound returned to caller; fine here
		}))
		defer srv.Close()

		clientA := NewClient(srv.URL, testAuthToken, testProfileA)
		clientB := NewClient(srv.URL, testAuthToken, profileB)

		// Errors (ErrFragmentNotFound) are expected and intentionally discarded.
		NewFragmentGet(clientA).GetByID(ctx, testProfileA, "frag-x") //nolint:errcheck
		NewFragmentGet(clientB).GetByID(ctx, profileB, "frag-x")     //nolint:errcheck

		require.Len(t, capturedProfiles, 2)
		require.Equal(t, testProfileA, capturedProfiles[0],
			"client A must send its own profile ID in X-Profile-ID")
		require.Equal(t, profileB, capturedProfiles[1],
			"client B must send its own profile ID in X-Profile-ID")
		require.NotEqual(t, capturedProfiles[0], profileB,
			"profile-A request must not carry profile-B's ID")
		require.NotEqual(t, capturedProfiles[1], testProfileA,
			"profile-B request must not carry profile-A's ID")
	})
}

// Compile-time interface checks: ensure every adapter satisfies its service interface.
// These fail at build time (not just test time) if a signature drifts.
var (
	_ fragmentservice.CreateFragmentService = (*fragmentCreateAdapter)(nil)
	_ fragmentservice.GetFragmentService    = (*fragmentGetAdapter)(nil)
	_ fragmentservice.ListFragmentsService  = (*fragmentListAdapter)(nil)
	_ recallservice.RecallService           = (*recallAdapter)(nil)
	_ keywordsearch.KeywordSearchService    = (*keywordSearchAdapter)(nil)
	_ semanticsearch.SemanticSearchService  = (*semanticSearchAdapter)(nil)
	_ graphquery.GraphQueryService          = (*graphQueryAdapter)(nil)
)

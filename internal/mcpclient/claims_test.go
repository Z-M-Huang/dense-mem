package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dense-mem/dense-mem/internal/domain"
	httpDto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/service/claimservice"
	"github.com/dense-mem/dense-mem/internal/service/communityservice"
	"github.com/dense-mem/dense-mem/internal/service/factservice"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
	"github.com/stretchr/testify/require"
)

// testAuthToken is defined in client_test.go (same package mcpclient).
// All _test.go files in the same package share one compilation unit, so the
// const is visible here without re-declaration. Duplicating it would cause a
// "redeclared in this block" compile error.
//
// If you need to read this file in isolation, the value is:
//   "fixture-token-for-tests"

// TestKnowledgeToolAdapters verifies the 9 new HTTP adapter constructors against
// AC-55 (adapters implement service interfaces), AC-57 (correct headers sent),
// AC-58 (response parsing), and AC-60 (cross-profile isolation).
func TestKnowledgeToolAdapters(t *testing.T) {
	ctx := context.Background()
	const profileA = "profile-A"

	// -----------------------------------------------------------------------
	// CreateClaim
	// -----------------------------------------------------------------------
	t.Run("CreateClaim_sends_correct_headers_and_parses_response", func(t *testing.T) {
		claimResp := httpDto.ClaimResponse{
			ClaimID:           "claim-001",
			ProfileID:         profileA,
			Subject:           "sky",
			Predicate:         "is",
			Object:            "blue",
			Status:            "candidate",
			EntailmentVerdict: "unverified",
			RecordedAt:        time.Now(),
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+testAuthToken, r.Header.Get("Authorization"), "Authorization bearer token must be set")
			require.Equal(t, profileA, r.Header.Get("X-Profile-ID"), "X-Profile-ID must match caller")
			require.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if err := json.NewEncoder(w).Encode(claimResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewClaimCreate(c)
		result, err := svc.Create(ctx, profileA, &domain.Claim{
			Subject:     "sky",
			Predicate:   "is",
			Object:      "blue",
			SupportedBy: []string{"frag-001"},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "claim-001", result.Claim.ClaimID)
		require.Equal(t, profileA, result.Claim.ProfileID)
		require.False(t, result.Duplicate)
	})

	t.Run("CreateClaim_detects_duplicate_via_X_Idempotent_Replay_header", func(t *testing.T) {
		claimResp := httpDto.ClaimResponse{
			ClaimID:    "claim-001",
			ProfileID:  profileA,
			RecordedAt: time.Now(),
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Idempotent-Replay", "true")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(claimResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewClaimCreate(c)
		result, err := svc.Create(ctx, profileA, &domain.Claim{SupportedBy: []string{"frag-001"}})
		require.NoError(t, err)
		require.True(t, result.Duplicate, "X-Idempotent-Replay: true must set Duplicate=true")
	})

	// -----------------------------------------------------------------------
	// GetClaim
	// -----------------------------------------------------------------------
	t.Run("GetClaim_sends_correct_headers_and_returns_claim", func(t *testing.T) {
		claimResp := httpDto.ClaimResponse{
			ClaimID:    "claim-002",
			ProfileID:  profileA,
			Subject:    "water",
			Predicate:  "is",
			Object:     "wet",
			RecordedAt: time.Now(),
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+testAuthToken, r.Header.Get("Authorization"))
			require.Equal(t, profileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(claimResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewClaimGet(c)
		claim, err := svc.Get(ctx, profileA, "claim-002")
		require.NoError(t, err)
		require.NotNil(t, claim)
		require.Equal(t, "claim-002", claim.ClaimID)
		require.Equal(t, profileA, claim.ProfileID)
		require.Equal(t, "water", claim.Subject)
	})

	t.Run("GetClaim_returns_ErrClaimNotFound_on_404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewClaimGet(c)
		_, err := svc.Get(ctx, profileA, "missing")
		require.Error(t, err)
		require.True(t, errors.Is(err, claimservice.ErrClaimNotFound),
			"404 from server must surface as ErrClaimNotFound")
	})

	// -----------------------------------------------------------------------
	// ListClaims
	// -----------------------------------------------------------------------
	t.Run("ListClaims_sends_correct_headers_and_returns_claims", func(t *testing.T) {
		listResp := httpDto.ListClaimsResponse{
			Items: []httpDto.ClaimResponse{
				{ClaimID: "claim-003", ProfileID: profileA, Subject: "sun", RecordedAt: time.Now()},
			},
			NextCursor: "20",
			HasMore:    true,
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+testAuthToken, r.Header.Get("Authorization"))
			require.Equal(t, profileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(listResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewClaimList(c)
		claims, total, err := svc.List(ctx, profileA, 20, 0)
		require.NoError(t, err)
		require.Len(t, claims, 1)
		require.Equal(t, "claim-003", claims[0].ClaimID)
		require.Equal(t, profileA, claims[0].ProfileID)
		// HasMore=true → total > offset+len so callers detect a next page.
		require.Greater(t, total, len(claims),
			"total must be > len(items) when HasMore=true to signal a next page")
	})

	t.Run("ListClaims_no_next_page_when_HasMore_false", func(t *testing.T) {
		listResp := httpDto.ListClaimsResponse{
			Items: []httpDto.ClaimResponse{
				{ClaimID: "claim-last", ProfileID: profileA, RecordedAt: time.Now()},
			},
			HasMore: false,
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(listResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewClaimList(c)
		claims, total, err := svc.List(ctx, profileA, 20, 0)
		require.NoError(t, err)
		require.Len(t, claims, 1)
		// HasMore=false → total == offset+len; no further page.
		require.Equal(t, len(claims), total)
	})

	// -----------------------------------------------------------------------
	// VerifyClaim
	// -----------------------------------------------------------------------
	t.Run("VerifyClaim_sends_correct_headers_and_returns_updated_claim", func(t *testing.T) {
		now := time.Now()
		verifyResp := httpDto.VerifyClaimResponse{
			ClaimID:           "claim-004",
			EntailmentVerdict: "entailed",
			Status:            "validated",
			VerifiedAt:        &now,
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+testAuthToken, r.Header.Get("Authorization"))
			require.Equal(t, profileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(verifyResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewClaimVerify(c)
		claim, err := svc.Verify(ctx, profileA, "claim-004")
		require.NoError(t, err)
		require.NotNil(t, claim)
		require.Equal(t, "claim-004", claim.ClaimID)
		require.Equal(t, profileA, claim.ProfileID)
		require.Equal(t, domain.VerdictEntailed, claim.EntailmentVerdict)
		require.Equal(t, domain.StatusValidated, claim.Status)
	})

	t.Run("VerifyClaim_returns_ErrClaimNotFound_on_404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewClaimVerify(c)
		_, err := svc.Verify(ctx, profileA, "missing")
		require.True(t, errors.Is(err, claimservice.ErrClaimNotFound),
			"404 from server must surface as ErrClaimNotFound")
	})

	// -----------------------------------------------------------------------
	// PromoteClaim
	// -----------------------------------------------------------------------
	t.Run("PromoteClaim_sends_correct_headers_and_returns_fact", func(t *testing.T) {
		factResp := httpDto.FactResponse{
			FactID:              "fact-001",
			ProfileID:           profileA,
			Subject:             "sky",
			Predicate:           "is",
			Object:              "blue",
			Status:              "active",
			PromotedFromClaimID: "claim-005",
			RecordedAt:          time.Now(),
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+testAuthToken, r.Header.Get("Authorization"))
			require.Equal(t, profileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if err := json.NewEncoder(w).Encode(factResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewClaimPromote(c)
		fact, err := svc.Promote(ctx, profileA, "claim-005")
		require.NoError(t, err)
		require.NotNil(t, fact)
		require.Equal(t, "fact-001", fact.FactID)
		require.Equal(t, profileA, fact.ProfileID)
		require.Equal(t, "claim-005", fact.PromotedFromClaimID)
	})

	// -----------------------------------------------------------------------
	// GetFact
	// -----------------------------------------------------------------------
	t.Run("GetFact_sends_correct_headers_and_returns_fact", func(t *testing.T) {
		factResp := httpDto.FactResponse{
			FactID:     "fact-002",
			ProfileID:  profileA,
			Subject:    "fire",
			Predicate:  "is",
			Object:     "hot",
			Status:     "active",
			RecordedAt: time.Now(),
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+testAuthToken, r.Header.Get("Authorization"))
			require.Equal(t, profileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(factResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewFactGet(c)
		fact, err := svc.Get(ctx, profileA, "fact-002")
		require.NoError(t, err)
		require.NotNil(t, fact)
		require.Equal(t, "fact-002", fact.FactID)
		require.Equal(t, profileA, fact.ProfileID)
		require.Equal(t, "fire", fact.Subject)
	})

	t.Run("GetFact_returns_ErrFactNotFound_on_404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewFactGet(c)
		_, err := svc.Get(ctx, profileA, "missing")
		require.True(t, errors.Is(err, factservice.ErrFactNotFound),
			"404 from server must surface as ErrFactNotFound")
	})

	// -----------------------------------------------------------------------
	// ListFacts
	// -----------------------------------------------------------------------
	t.Run("ListFacts_sends_correct_headers_and_returns_facts", func(t *testing.T) {
		listResp := httpDto.ListFactsResponse{
			Items: []httpDto.FactResponse{
				{FactID: "fact-003", ProfileID: profileA, Subject: "ice", RecordedAt: time.Now()},
			},
			NextCursor: "next-page-token",
			HasMore:    true,
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+testAuthToken, r.Header.Get("Authorization"))
			require.Equal(t, profileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(listResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewFactList(c)
		facts, nextCursor, err := svc.List(ctx, profileA, factservice.FactListFilters{}, 10, "")
		require.NoError(t, err)
		require.Len(t, facts, 1)
		require.Equal(t, "fact-003", facts[0].FactID)
		require.Equal(t, profileA, facts[0].ProfileID)
		require.Equal(t, "next-page-token", nextCursor)
	})

	t.Run("ListFacts_passes_filters_as_query_params", func(t *testing.T) {
		listResp := httpDto.ListFactsResponse{
			Items: []httpDto.FactResponse{
				{FactID: "fact-004", ProfileID: profileA, Subject: "ocean", RecordedAt: time.Now()},
			},
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "ocean", r.URL.Query().Get("subject"))
			require.Equal(t, "is", r.URL.Query().Get("predicate"))
			require.Equal(t, "active", r.URL.Query().Get("status"))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(listResp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewFactList(c)
		facts, _, err := svc.List(ctx, profileA, factservice.FactListFilters{
			Subject:   "ocean",
			Predicate: "is",
			Status:    domain.FactStatusActive,
		}, 10, "")
		require.NoError(t, err)
		require.Len(t, facts, 1)
	})

	// -----------------------------------------------------------------------
	// RetractFragment
	// -----------------------------------------------------------------------
	t.Run("RetractFragment_sends_correct_headers_and_succeeds", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+testAuthToken, r.Header.Get("Authorization"))
			require.Equal(t, profileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodPost, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"status":"retracted"}`)); err != nil {
				t.Errorf("write: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewFragmentRetract(c)
		err := svc.Retract(ctx, profileA, "frag-retract-001")
		require.NoError(t, err)
	})

	t.Run("RetractFragment_returns_ErrFragmentNotFound_on_404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewFragmentRetract(c)
		err := svc.Retract(ctx, profileA, "missing-frag")
		require.True(t, errors.Is(err, fragmentservice.ErrFragmentNotFound),
			"404 from server must surface as ErrFragmentNotFound")
	})

	// -----------------------------------------------------------------------
	// DetectCommunity
	// -----------------------------------------------------------------------
	t.Run("DetectCommunity_sends_correct_headers_and_succeeds", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+testAuthToken, r.Header.Get("Authorization"))
			require.Equal(t, profileA, r.Header.Get("X-Profile-ID"))
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, 1.6, body["gamma"])
			require.Equal(t, 0.0002, body["tolerance"])
			require.Equal(t, 7.0, body["max_levels"])
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"detected":true}`)); err != nil {
				t.Errorf("write: %v", err)
			}
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewCommunityDetect(c)
		err := svc.Detect(ctx, profileA, communityservice.DetectOptions{
			Gamma:     1.6,
			Tolerance: 0.0002,
			MaxLevels: 7,
		})
		require.NoError(t, err)
	})

	t.Run("DetectCommunity_returns_ErrCommunityUnavailable_on_503", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewCommunityDetect(c)
		err := svc.Detect(ctx, profileA, communityservice.DetectOptions{})
		require.True(t, errors.Is(err, communityservice.ErrCommunityUnavailable),
			"503 from server must surface as ErrCommunityUnavailable")
	})

	t.Run("DetectCommunity_returns_ErrCommunityGraphTooLarge_on_422", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
		}))
		defer srv.Close()

		c := NewClient(srv.URL, testAuthToken, profileA)
		svc := NewCommunityDetect(c)
		err := svc.Detect(ctx, profileA, communityservice.DetectOptions{})
		require.True(t, errors.Is(err, communityservice.ErrCommunityGraphTooLarge),
			"422 from server must surface as ErrCommunityGraphTooLarge")
	})

	// -----------------------------------------------------------------------
	// Cross-profile isolation (AC-60 / profile-isolation.md)
	// Two distinct clients bound to different profiles must never exchange
	// profile IDs. The adapter sends the caller-supplied profileID in
	// X-Profile-ID on every request.
	// -----------------------------------------------------------------------
	t.Run("CrossProfileIsolation", func(t *testing.T) {
		const profileB = "profile-B"

		capturedProfiles := make([]string, 0, 2)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedProfiles = append(capturedProfiles, r.Header.Get("X-Profile-ID"))
			// Return 404 so ErrClaimNotFound is returned; the error is discarded.
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		clientA := NewClient(srv.URL, testAuthToken, profileA)
		clientB := NewClient(srv.URL, testAuthToken, profileB)

		// Errors (ErrClaimNotFound) are expected and intentionally discarded.
		NewClaimGet(clientA).Get(ctx, profileA, "claim-x") //nolint:errcheck
		NewClaimGet(clientB).Get(ctx, profileB, "claim-x") //nolint:errcheck

		require.Len(t, capturedProfiles, 2)
		require.Equal(t, profileA, capturedProfiles[0],
			"client A must send its own profile ID in X-Profile-ID")
		require.Equal(t, profileB, capturedProfiles[1],
			"client B must send its own profile ID in X-Profile-ID")
		require.NotEqual(t, capturedProfiles[0], profileB,
			"profile-A request must not carry profile-B's ID")
		require.NotEqual(t, capturedProfiles[1], profileA,
			"profile-B request must not carry profile-A's ID")
	})
}

// Compile-time interface checks: ensure every new adapter satisfies its service interface.
// These fail at build time (not just test time) if a signature drifts.
var (
	_ claimservice.CreateClaimService         = (*claimCreateAdapter)(nil)
	_ claimservice.GetClaimService            = (*claimGetAdapter)(nil)
	_ claimservice.ListClaimsService          = (*claimListAdapter)(nil)
	_ claimservice.VerifyClaimService         = (*claimVerifyAdapter)(nil)
	_ factservice.PromoteClaimService         = (*claimPromoteAdapter)(nil)
	_ factservice.GetFactService              = (*factGetAdapter)(nil)
	_ factservice.ListFactsService            = (*factListAdapter)(nil)
	_ fragmentservice.RetractFragmentService  = (*fragmentRetractAdapter)(nil)
	_ communityservice.DetectCommunityService = (*communityDetectAdapter)(nil)
)

package memoryservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/service/claimservice"
	"github.com/dense-mem/dense-mem/internal/service/factservice"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
	"github.com/stretchr/testify/require"
)

type stubFragmentCreate struct {
	called int
	req    *dto.CreateFragmentRequest
	res    *fragmentservice.CreateResult
	err    error
}

func (s *stubFragmentCreate) Create(_ context.Context, profileID string, req *dto.CreateFragmentRequest) (*fragmentservice.CreateResult, error) {
	s.called++
	s.req = req
	if s.err != nil {
		return nil, s.err
	}
	if s.res != nil {
		return s.res, nil
	}
	return &fragmentservice.CreateResult{
		Fragment: &domain.Fragment{
			FragmentID: "fragment-1",
			ProfileID:  profileID,
			Content:    req.Content,
			CreatedAt:  time.Now().UTC(),
		},
	}, nil
}

type stubClaimCreate struct {
	called int
	claim  *domain.Claim
	res    *claimservice.CreateResult
	err    error
}

func (s *stubClaimCreate) Create(_ context.Context, profileID string, claim *domain.Claim) (*claimservice.CreateResult, error) {
	s.called++
	s.claim = claim
	if s.err != nil {
		return nil, s.err
	}
	if s.res != nil {
		return s.res, nil
	}
	created := *claim
	created.ClaimID = "claim-1"
	created.ProfileID = profileID
	created.Status = domain.StatusCandidate
	return &claimservice.CreateResult{Claim: &created}, nil
}

type stubClaimVerify struct {
	called int
	claim  *domain.Claim
	err    error
}

func (s *stubClaimVerify) Verify(_ context.Context, profileID, claimID string) (*domain.Claim, error) {
	s.called++
	if s.err != nil {
		return nil, s.err
	}
	if s.claim != nil {
		return s.claim, nil
	}
	return &domain.Claim{
		ClaimID:           claimID,
		ProfileID:         profileID,
		Subject:           "user",
		Predicate:         "prefers",
		Object:            "vim",
		Status:            domain.StatusValidated,
		EntailmentVerdict: domain.VerdictEntailed,
	}, nil
}

type stubFactPromote struct {
	called int
	fact   *domain.Fact
	err    error
}

func (s *stubFactPromote) Promote(_ context.Context, profileID, claimID string) (*domain.Fact, error) {
	s.called++
	if s.err != nil {
		return nil, s.err
	}
	if s.fact != nil {
		return s.fact, nil
	}
	return &domain.Fact{FactID: "fact-1", ProfileID: profileID, PromotedFromClaimID: claimID}, nil
}

type stubFactList struct {
	facts []*domain.Fact
	err   error
}

func (s stubFactList) List(_ context.Context, _ string, _ factservice.FactListFilters, _ int, _ string) ([]*domain.Fact, string, error) {
	if s.err != nil {
		return nil, "", s.err
	}
	return s.facts, "", nil
}

type stubClaimList struct {
	claims []*domain.Claim
	err    error
}

func (s stubClaimList) List(_ context.Context, _ string, _ int, _ int) ([]*domain.Claim, int, error) {
	if s.err != nil {
		return nil, 0, s.err
	}
	return s.claims, len(s.claims), nil
}

type stubConfirm struct {
	called int
	res    *factservice.ConfirmMemoryResult
	err    error
}

func (s *stubConfirm) ConfirmMemory(_ context.Context, profileID string, req factservice.ConfirmMemoryRequest) (*factservice.ConfirmMemoryResult, error) {
	s.called++
	if s.err != nil {
		return nil, s.err
	}
	if s.res != nil {
		return s.res, nil
	}
	return &factservice.ConfirmMemoryResult{
		ClaimID:  req.ClaimID,
		Decision: req.Decision,
		Status:   "accepted",
		Fact:     &domain.Fact{FactID: "fact-confirmed", ProfileID: profileID},
	}, nil
}

func TestRememberPromotesValidatedNonConflictingClaim(t *testing.T) {
	create := &stubClaimCreate{}
	verify := &stubClaimVerify{}
	promote := &stubFactPromote{}
	svc := New(Dependencies{
		FragmentCreate: &stubFragmentCreate{},
		ClaimCreate:    create,
		ClaimVerify:    verify,
		FactPromote:    promote,
	})

	res, err := svc.Remember(context.Background(), "profile-1", RememberRequest{
		Content: "The user prefers vim.",
		Claims: []TypedClaimInput{{
			Subject:        "user",
			Predicate:      "prefers",
			Object:         "vim",
			ExtractConf:    0.9,
			ResolutionConf: 0.9,
		}},
	})

	require.NoError(t, err)
	require.Equal(t, "fragment-1", res.Fragment.ID)
	require.Len(t, res.Claims, 1)
	require.Equal(t, "promoted", res.Claims[0].Promotion)
	require.Equal(t, "fact-1", res.Claims[0].Fact.FactID)
	require.Equal(t, []string{"fragment-1"}, create.claim.SupportedBy)
	require.Equal(t, 1, verify.called)
	require.Equal(t, 1, promote.called)
	require.Empty(t, res.Clarifications)
}

func TestRememberRejectsUnsupportedHighLevelPredicateBeforeClaimCreate(t *testing.T) {
	create := &stubClaimCreate{}
	svc := New(Dependencies{
		FragmentCreate: &stubFragmentCreate{},
		ClaimCreate:    create,
	})

	res, err := svc.Remember(context.Background(), "profile-1", RememberRequest{
		Content: "The user lives at 123 Main St.",
		Claims: []TypedClaimInput{{
			Subject:        "user",
			Predicate:      "lives_at",
			Object:         "123 Main St",
			ExtractConf:    0.9,
			ResolutionConf: 0.9,
		}},
	})

	require.NoError(t, err)
	require.Len(t, res.Claims, 1)
	require.Equal(t, "predicate_not_supported", res.Claims[0].Status)
	require.Equal(t, 0, create.called)
}

func TestRememberReturnsStructuredPromotionOutcomes(t *testing.T) {
	t.Run("weaker conflict is rejected without clarification", func(t *testing.T) {
		svc := New(Dependencies{
			FragmentCreate: &stubFragmentCreate{},
			ClaimCreate:    &stubClaimCreate{},
			ClaimVerify:    &stubClaimVerify{},
			FactPromote:    &stubFactPromote{err: factservice.ErrPromotionRejected},
		})

		res, err := svc.Remember(context.Background(), "profile-1", RememberRequest{
			Content: "The user has a different profile fact.",
			Claims: []TypedClaimInput{{
				Subject:        "user",
				Predicate:      "profile_fact",
				Object:         "new value",
				ExtractConf:    0.9,
				ResolutionConf: 0.9,
			}},
		})

		require.NoError(t, err)
		require.Equal(t, "rejected_weaker", res.Claims[0].Promotion)
		require.Empty(t, res.Clarifications)
	})

	t.Run("comparable conflict returns clarification", func(t *testing.T) {
		conflict := &domain.Fact{
			FactID:    "fact-old",
			ProfileID: "profile-1",
			Subject:   "user",
			Predicate: "profile_fact",
			Object:    "old value",
			Status:    domain.FactStatusActive,
		}
		verify := &stubClaimVerify{claim: &domain.Claim{
			ClaimID:           "claim-1",
			ProfileID:         "profile-1",
			Subject:           "user",
			Predicate:         "profile_fact",
			Object:            "new value",
			Status:            domain.StatusValidated,
			EntailmentVerdict: domain.VerdictEntailed,
		}}
		svc := New(Dependencies{
			FragmentCreate: &stubFragmentCreate{},
			ClaimCreate:    &stubClaimCreate{},
			ClaimVerify:    verify,
			FactPromote:    &stubFactPromote{err: factservice.ErrPromotionDeferredDisputed},
			FactList:       stubFactList{facts: []*domain.Fact{conflict}},
		})

		res, err := svc.Remember(context.Background(), "profile-1", RememberRequest{
			Content: "The user has a conflicting profile fact.",
			Claims: []TypedClaimInput{{
				Subject:        "user",
				Predicate:      "profile_fact",
				Object:         "new value",
				ExtractConf:    0.9,
				ResolutionConf: 0.9,
			}},
		})

		require.NoError(t, err)
		require.Equal(t, "clarification_required", res.Claims[0].Promotion)
		require.Len(t, res.Clarifications, 1)
		require.Equal(t, "claim-1", res.Clarifications[0].ClaimID)
		require.Equal(t, "fact-old", res.Clarifications[0].ConflictingFacts[0].FactID)
	})
}

func TestImportMemoriesDoesNotAutoPromoteByDefault(t *testing.T) {
	promote := &stubFactPromote{}
	svc := New(Dependencies{
		FragmentCreate: &stubFragmentCreate{},
		ClaimCreate:    &stubClaimCreate{},
		ClaimVerify:    &stubClaimVerify{},
		FactPromote:    promote,
	})

	res, err := svc.ImportMemories(context.Background(), "profile-1", ImportRequest{
		Summary: "Historical summary: user uses Go.",
		Claims: []TypedClaimInput{{
			Subject:        "user",
			Predicate:      "uses",
			Object:         "Go",
			ExtractConf:    0.9,
			ResolutionConf: 0.9,
		}},
	})

	require.NoError(t, err)
	require.Len(t, res.Claims, 1)
	require.Empty(t, res.Claims[0].Promotion)
	require.Equal(t, 0, promote.called)
}

func TestReflectReturnsDisputedClarificationsAndStaleFacts(t *testing.T) {
	old := time.Now().Add(-60 * 24 * time.Hour)
	disputed := &domain.Claim{
		ClaimID:   "claim-disputed",
		Subject:   "user",
		Predicate: "profile_fact",
		Object:    "new value",
		Status:    domain.StatusDisputed,
	}
	svc := New(Dependencies{
		FactList: stubFactList{facts: []*domain.Fact{{
			FactID:     "fact-stale",
			Subject:    "user",
			Predicate:  "profile_fact",
			Object:     "old value",
			Status:     domain.FactStatusActive,
			RecordedAt: old,
		}}},
		ClaimList: stubClaimList{claims: []*domain.Claim{
			{ClaimID: "claim-candidate", Status: domain.StatusCandidate},
			disputed,
		}},
	})

	res, err := svc.Reflect(context.Background(), "profile-1", ReflectRequest{StaleAfterDays: 30})

	require.NoError(t, err)
	require.Len(t, res.StaleFacts, 1)
	require.Len(t, res.CandidateClaims, 1)
	require.Len(t, res.DisputedClaims, 1)
	require.Len(t, res.Clarifications, 1)
	require.Equal(t, "claim-disputed", res.Clarifications[0].ClaimID)
}

func TestConfirmMemoryDelegatesConfirmation(t *testing.T) {
	confirm := &stubConfirm{}
	svc := New(Dependencies{FactConfirm: confirm})

	res, err := svc.ConfirmMemory(context.Background(), "profile-1", ConfirmRequest{
		ClaimID:  "claim-1",
		Decision: "accept_claim",
	})

	require.NoError(t, err)
	require.Equal(t, 1, confirm.called)
	require.Equal(t, "accepted", res.Status)
	require.Equal(t, "fact-confirmed", res.Fact.FactID)
}

func TestRememberReturnsFragmentCreateError(t *testing.T) {
	svc := New(Dependencies{FragmentCreate: &stubFragmentCreate{err: errors.New("embed failed")}})

	_, err := svc.Remember(context.Background(), "profile-1", RememberRequest{Content: "x"})

	require.ErrorContains(t, err, "embed failed")
}

package communityservice

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildCommunitySummaries_PrefersCurrentFacts(t *testing.T) {
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

	summaries := buildCommunitySummaries("profile-1", []communitySummaryInput{
		{
			CommunityID: "7",
			MemberCount: 4,
			FactTriples: []communityTriple{
				{Subject: "alice", Predicate: "knows", Object: "bob"},
				{Subject: "alice", Predicate: "knows", Object: "carol"},
				{Subject: "bob", Predicate: "works_with", Object: "alice"},
			},
			ClaimTriples: []communityTriple{
				{Subject: "draft", Predicate: "mentions", Object: "topic"},
			},
		},
	}, now)

	require.Len(t, summaries, 1)
	got := summaries[0]
	require.Equal(t, "7", got.CommunityID)
	require.Equal(t, "profile-1", got.ProfileID)
	require.Equal(t, communitySummaryVersion, got.SummaryVersion)
	require.Equal(t, 4, got.MemberCount)
	require.Equal(t, now, got.LastSummarizedAt)
	require.Equal(t, []string{"alice", "bob", "carol"}, got.TopEntities)
	require.Equal(t, []string{"knows", "works_with"}, got.TopPredicates)
	require.Contains(t, got.Summary, "current facts")
	require.NotContains(t, got.Summary, "no current facts yet")
	require.True(t, strings.Contains(got.Summary, "alice knows bob") || strings.Contains(got.Summary, "alice knows carol"))
}

func TestBuildCommunitySummaries_FallsBackToClaims(t *testing.T) {
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

	summaries := buildCommunitySummaries("profile-2", []communitySummaryInput{
		{
			CommunityID: "11",
			MemberCount: 3,
			ClaimTriples: []communityTriple{
				{Subject: "project", Predicate: "needs", Object: "review"},
				{Subject: "review", Predicate: "blocks", Object: "launch"},
			},
		},
	}, now)

	require.Len(t, summaries, 1)
	got := summaries[0]
	require.Equal(t, []string{"review", "launch", "project"}, got.TopEntities)
	require.Equal(t, []string{"blocks", "needs"}, got.TopPredicates)
	require.Contains(t, got.Summary, "no current facts yet")
	require.Contains(t, got.Summary, "Claim themes center on")
}

func TestBuildCommunitySummaries_SortsBySizeThenID(t *testing.T) {
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

	summaries := buildCommunitySummaries("profile-3", []communitySummaryInput{
		{CommunityID: "b", MemberCount: 2},
		{CommunityID: "a", MemberCount: 2},
		{CommunityID: "z", MemberCount: 5},
	}, now)

	require.Len(t, summaries, 3)
	require.Equal(t, "z", summaries[0].CommunityID)
	require.Equal(t, "a", summaries[1].CommunityID)
	require.Equal(t, "b", summaries[2].CommunityID)
}

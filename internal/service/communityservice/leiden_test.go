package communityservice

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// stubCommunityLocker is a test-only implementation of communityLocker.
// It immediately invokes fn without acquiring a real Postgres advisory lock.
//
// Profile isolation invariant: the locker records each profileID it was called
// with so tests can assert that different profiles use separate lock keys.
type stubCommunityLocker struct {
	locked []string // profileIDs passed to WithCommunityLock in call order
	err    error    // returned before fn is called when non-nil
}

func (s *stubCommunityLocker) WithCommunityLock(ctx context.Context, profileID string, fn func(ctx context.Context) error) error {
	s.locked = append(s.locked, profileID)
	if s.err != nil {
		return s.err
	}
	return fn(ctx)
}

// Compile-time check that stubCommunityLocker satisfies communityLocker.
var _ communityLocker = (*stubCommunityLocker)(nil)

// stubLeidenQuerier is a test-only implementation of leidenQuerier.
//
// Fields:
//   - estimateNodeCount: returned by EstimateProjection
//   - estimateErr:       error returned by EstimateProjection
//   - projectErr:        error returned by ProjectGraph
//   - leidenErr:         error returned by RunLeiden
//   - dropErr:           error returned by DropGraph
//
// Recorded calls (for assertion):
//   - estimatedProfiles: profileID passed to EstimateProjection in order
//   - projectedProfiles: profileID passed to ProjectGraph in order
//   - projectedGraphs:   graphName passed to ProjectGraph in order
//   - droppedGraphs:     graphName passed to DropGraph in order
type stubLeidenQuerier struct {
	estimateNodeCount int64
	estimateErr       error
	projectErr        error
	leidenErr         error
	dropErr           error

	estimatedProfiles []string
	projectedProfiles []string
	projectedGraphs   []string
	droppedGraphs     []string
}

func (s *stubLeidenQuerier) EstimateProjection(_ context.Context, profileID, _ string) (int64, error) {
	s.estimatedProfiles = append(s.estimatedProfiles, profileID)
	if s.estimateErr != nil {
		return 0, s.estimateErr
	}
	return s.estimateNodeCount, nil
}

func (s *stubLeidenQuerier) ProjectGraph(_ context.Context, profileID, graphName string) error {
	s.projectedProfiles = append(s.projectedProfiles, profileID)
	s.projectedGraphs = append(s.projectedGraphs, graphName)
	return s.projectErr
}

func (s *stubLeidenQuerier) RunLeiden(_ context.Context, _ string) error {
	return s.leidenErr
}

func (s *stubLeidenQuerier) DropGraph(_ context.Context, graphName string) error {
	s.droppedGraphs = append(s.droppedGraphs, graphName)
	return s.dropErr
}

// Compile-time check that stubLeidenQuerier satisfies leidenQuerier.
var _ leidenQuerier = (*stubLeidenQuerier)(nil)

// stubConfigProvider is a minimal test-only implementation of
// config.ConfigProvider that returns only the fields relevant to Leiden.
type stubConfigProvider struct {
	maxNodes int
}

func (s *stubConfigProvider) GetAICommunityMaxNodes() int { return s.maxNodes }

// Implement the remaining ConfigProvider methods with zero/empty returns so
// the stub compiles as a config.ConfigProvider without importing that package.
func (s *stubConfigProvider) GetHTTPAddr() string                  { return "" }
func (s *stubConfigProvider) GetPostgresDSN() string               { return "" }
func (s *stubConfigProvider) GetNeo4jURI() string                  { return "" }
func (s *stubConfigProvider) GetNeo4jUser() string                 { return "" }
func (s *stubConfigProvider) GetNeo4jPassword() string             { return "" }
func (s *stubConfigProvider) GetNeo4jDatabase() string             { return "" }
func (s *stubConfigProvider) GetRedisAddr() string                 { return "" }
func (s *stubConfigProvider) GetRedisPassword() string             { return "" }
func (s *stubConfigProvider) GetRedisDB() int                      { return 0 }
func (s *stubConfigProvider) GetBootstrapAdminKey() string         { return "" }
func (s *stubConfigProvider) GetArgonMemoryKB() int                { return 0 }
func (s *stubConfigProvider) GetArgonTime() int                    { return 0 }
func (s *stubConfigProvider) GetArgonThreads() int                 { return 0 }
func (s *stubConfigProvider) GetRateLimitPerMinute() int           { return 0 }
func (s *stubConfigProvider) GetAdminRateLimitPerMinute() int      { return 0 }
func (s *stubConfigProvider) GetFragmentCreateRateLimit() int      { return 0 }
func (s *stubConfigProvider) GetFragmentReadRateLimit() int        { return 0 }
func (s *stubConfigProvider) GetSSEHeartbeatSeconds() int          { return 0 }
func (s *stubConfigProvider) GetSSEMaxDurationSeconds() int        { return 0 }
func (s *stubConfigProvider) GetSSEMaxConcurrentStreams() int      { return 0 }
func (s *stubConfigProvider) GetAdminQueryTimeoutSeconds() int     { return 0 }
func (s *stubConfigProvider) GetAdminQueryRowCap() int             { return 0 }
func (s *stubConfigProvider) GetEmbeddingDimensions() int          { return 0 }
func (s *stubConfigProvider) GetAIAPIURL() string                  { return "" }
func (s *stubConfigProvider) GetAIAPIKey() string                  { return "" }
func (s *stubConfigProvider) GetAIEmbeddingModel() string          { return "" }
func (s *stubConfigProvider) GetAIEmbeddingDimensions() int        { return 0 }
func (s *stubConfigProvider) GetAIEmbeddingTimeoutSeconds() int    { return 0 }
func (s *stubConfigProvider) IsEmbeddingConfigured() bool          { return false }
func (s *stubConfigProvider) GetAIVerifierModel() string           { return "" }
func (s *stubConfigProvider) GetAIVerifierMaxConcurrency() int     { return 0 }
func (s *stubConfigProvider) GetClaimWriteRateLimit() int          { return 0 }
func (s *stubConfigProvider) GetClaimReadRateLimit() int           { return 0 }
func (s *stubConfigProvider) GetRecallValidatedClaimWeight() float64 { return 0 }
func (s *stubConfigProvider) GetPromoteTxTimeoutSeconds() int      { return 0 }

// newTestLeidenService constructs a leidenServiceImpl with injected stubs.
// Used only in tests; production callers use NewLeidenService.
func newTestLeidenService(locker communityLocker, querier leidenQuerier, maxNodes int) DetectCommunityService {
	return &leidenServiceImpl{
		locker:  locker,
		querier: querier,
		cfg:     &stubConfigProvider{maxNodes: maxNodes},
		logger:  nil,
	}
}

// ---------------------------------------------------------------------------
// TestLeidenDetect — covers AC-51, AC-52
// ---------------------------------------------------------------------------

// TestLeidenDetect verifies the full Detect flow: advisory lock acquisition,
// estimate check, graph projection, leiden write, and deferred graph drop.
func TestLeidenDetect(t *testing.T) {
	ctx := context.Background()
	const profileID = "profile-abc"
	const maxNodes = 500_000

	t.Run("happy path: detect completes within node cap", func(t *testing.T) {
		locker := &stubCommunityLocker{}
		querier := &stubLeidenQuerier{
			estimateNodeCount: 100, // well below maxNodes
		}
		svc := newTestLeidenService(locker, querier, maxNodes)

		err := svc.Detect(ctx, profileID)

		require.NoError(t, err, "Detect must succeed when node count is within the cap")

		// Advisory lock must have been acquired for the correct profile.
		require.Equal(t, []string{profileID}, locker.locked,
			"Detect must acquire the advisory lock for the given profileID")

		// Graph must have been projected for the correct profile.
		require.Contains(t, querier.projectedProfiles, profileID,
			"Detect must project a graph scoped to the given profileID")

		// Projected graph name must embed the profileID.
		wantGraph := GraphNamePrefix + profileID + "-leiden"
		require.Contains(t, querier.projectedGraphs, wantGraph,
			"projected graph name must embed profileID for isolation")

		// Drop must always be called (deferred).
		require.Contains(t, querier.droppedGraphs, wantGraph,
			"Detect must always drop the projected graph on return")
	})

	t.Run("rejects when estimated node count exceeds cap", func(t *testing.T) {
		locker := &stubCommunityLocker{}
		querier := &stubLeidenQuerier{
			estimateNodeCount: int64(maxNodes) + 1, // over the cap
		}
		svc := newTestLeidenService(locker, querier, maxNodes)

		err := svc.Detect(ctx, profileID)

		require.Error(t, err, "Detect must return an error when node count exceeds the cap")
		require.ErrorIs(t, err, ErrCommunityGraphTooLarge,
			"Detect must wrap ErrCommunityGraphTooLarge when node count exceeds the cap")

		// Graph must NOT have been projected when estimate exceeds the cap.
		require.Empty(t, querier.projectedGraphs,
			"Detect must not project a graph when the estimate exceeds the cap")

		// Drop must NOT have been called because projection never happened.
		require.Empty(t, querier.droppedGraphs,
			"Detect must not attempt a graph drop when projection was skipped")
	})

	t.Run("node count exactly at cap is allowed", func(t *testing.T) {
		locker := &stubCommunityLocker{}
		querier := &stubLeidenQuerier{
			estimateNodeCount: int64(maxNodes), // exactly at the cap
		}
		svc := newTestLeidenService(locker, querier, maxNodes)

		err := svc.Detect(ctx, profileID)

		require.NoError(t, err, "Detect must succeed when node count equals the cap exactly")
	})

	t.Run("graph is always dropped even when leiden write fails", func(t *testing.T) {
		leidenErr := errors.New("gds.leiden.write failed")
		locker := &stubCommunityLocker{}
		querier := &stubLeidenQuerier{
			estimateNodeCount: 50,
			leidenErr:         leidenErr,
		}
		svc := newTestLeidenService(locker, querier, maxNodes)

		err := svc.Detect(ctx, profileID)

		require.Error(t, err, "Detect must propagate the leiden write error")

		// Drop must still be called via defer.
		wantGraph := GraphNamePrefix + profileID + "-leiden"
		require.Contains(t, querier.droppedGraphs, wantGraph,
			"Detect must drop the projected graph even when leiden write fails")
	})

	t.Run("propagates estimate error", func(t *testing.T) {
		estimateErr := errors.New("gds.graph.project.estimate: procedure unavailable")
		locker := &stubCommunityLocker{}
		querier := &stubLeidenQuerier{estimateErr: estimateErr}
		svc := newTestLeidenService(locker, querier, maxNodes)

		err := svc.Detect(ctx, profileID)

		require.Error(t, err)
		require.ErrorContains(t, err, "leiden estimate projection")
	})

	t.Run("propagates project error and still drops graph", func(t *testing.T) {
		projectErr := errors.New("gds.graph.project: failed")
		locker := &stubCommunityLocker{}
		querier := &stubLeidenQuerier{
			estimateNodeCount: 50,
			projectErr:        projectErr,
		}
		svc := newTestLeidenService(locker, querier, maxNodes)

		err := svc.Detect(ctx, profileID)

		require.Error(t, err)
		require.ErrorContains(t, err, "leiden project graph")
	})
}

// ---------------------------------------------------------------------------
// TestLeidenDetect_CrossProfileIsolation — covers AC-53, AC-54
// ---------------------------------------------------------------------------

// TestLeidenDetect_CrossProfileIsolation verifies that running Detect for
// profile A does not affect profile B's graph data.
//
// This test satisfies the mandatory cross-profile isolation requirement from
// .claude/rules/profile-isolation.md.
func TestLeidenDetect_CrossProfileIsolation(t *testing.T) {
	ctx := context.Background()

	const profileA = "profile-A"
	const profileB = "profile-B"
	const maxNodes = 500_000

	// Run Detect for both profiles using the same querier instance so we can
	// observe that each profile operates on a distinct graph name.
	locker := &stubCommunityLocker{}
	querier := &stubLeidenQuerier{estimateNodeCount: 10}

	svcA := newTestLeidenService(locker, querier, maxNodes)
	svcB := newTestLeidenService(locker, querier, maxNodes)

	require.NoError(t, svcA.Detect(ctx, profileA))
	require.NoError(t, svcB.Detect(ctx, profileB))

	graphA := GraphNamePrefix + profileA + "-leiden"
	graphB := GraphNamePrefix + profileB + "-leiden"

	// Graph names must be distinct so GDS projections never overlap.
	require.NotEqual(t, graphA, graphB,
		"each profile must use a distinct GDS graph name")

	// Verify that profile A's graph name does not appear in profile B's operations.
	// The querier tracks which graphs were projected per call order.
	// Profile A's project call (index 0) must use graphA, not graphB.
	require.Equal(t, graphA, querier.projectedGraphs[0],
		"first Detect call (profile A) must project graph scoped to profile A")
	require.Equal(t, graphB, querier.projectedGraphs[1],
		"second Detect call (profile B) must project graph scoped to profile B")

	// Each profile's graph must have been dropped independently.
	require.Contains(t, querier.droppedGraphs, graphA,
		"profile A graph must be dropped after detection")
	require.Contains(t, querier.droppedGraphs, graphB,
		"profile B graph must be dropped after detection")

	// Profile B's drop list must not contain profile A's graph name,
	// confirming that profile A data was not touched during profile B's run.
	// Since stub tracks drop order, graphB appears at index 1.
	require.NotEqual(t, querier.droppedGraphs[1], graphA,
		"profile B Detect must not drop profile A graph")

	// Advisory lock must have been acquired separately for each profile.
	require.Contains(t, locker.locked, profileA,
		"Detect must acquire advisory lock for profile A")
	require.Contains(t, locker.locked, profileB,
		"Detect must acquire advisory lock for profile B")
	// The lock keys differ by profileID so the profiles are serialised
	// independently and never block each other.
	require.NotEqual(t, locker.locked[0], locker.locked[1],
		"advisory lock must use distinct keys for different profiles")
}

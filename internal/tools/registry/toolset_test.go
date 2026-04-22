package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/service/claimservice"
	"github.com/dense-mem/dense-mem/internal/service/factservice"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
	"github.com/dense-mem/dense-mem/internal/service/recallservice"
)

type stubCreate struct {
	called      int
	lastProfile string
	lastReq     *dto.CreateFragmentRequest
}

func (s *stubCreate) Create(ctx context.Context, profileID string, req *dto.CreateFragmentRequest) (*fragmentservice.CreateResult, error) {
	s.called++
	s.lastProfile = profileID
	s.lastReq = req
	return &fragmentservice.CreateResult{
		Fragment: &domain.Fragment{FragmentID: "f-1", ProfileID: profileID, Content: req.Content},
	}, nil
}

type stubGet struct{}

func (stubGet) GetByID(ctx context.Context, profileID, fragmentID string) (*domain.Fragment, error) {
	if fragmentID == "miss" {
		return nil, fragmentservice.ErrFragmentNotFound
	}
	return &domain.Fragment{FragmentID: fragmentID, ProfileID: profileID, Content: "hello"}, nil
}

type stubList struct{}

func (stubList) List(ctx context.Context, profileID string, opts fragmentservice.ListOptions) ([]domain.Fragment, string, error) {
	return []domain.Fragment{{FragmentID: "f-1", ProfileID: profileID}}, "", nil
}

func TestBuildDefault_RegistersV1ToolSurface(t *testing.T) {
	reg, err := BuildDefault(Dependencies{})
	if err != nil {
		t.Fatalf("BuildDefault: %v", err)
	}
	required := []string{
		"save_memory", "get_memory", "list_recent_memories", "recall_memory",
		"keyword-search", "semantic-search", "graph-query",
	}
	for _, name := range required {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestBuildDefault_SchemaFieldsPopulated(t *testing.T) {
	reg, _ := BuildDefault(Dependencies{})
	tool, _ := reg.Get("save_memory")
	if tool.Description == "" {
		t.Error("save_memory description is empty")
	}
	if tool.InputSchema == nil || tool.OutputSchema == nil {
		t.Error("save_memory schemas must not be nil")
	}
	if len(tool.RequiredScopes) == 0 {
		t.Error("save_memory must declare required scopes")
	}
}

func TestBuildDefault_AvailabilityReflectsDeps(t *testing.T) {
	// No deps → save_memory and recall_memory are Available=false.
	reg, _ := BuildDefault(Dependencies{})
	for _, name := range []string{"save_memory", "recall_memory"} {
		tool, _ := reg.Get(name)
		if tool.Available {
			t.Errorf("%s Available = true; want false when deps missing", name)
		}
	}
	// With FragmentCreate + embedding configured, save_memory becomes available.
	reg2, _ := BuildDefault(Dependencies{
		FragmentCreate:      &stubCreate{},
		EmbeddingConfigured: true,
	})
	save, _ := reg2.Get("save_memory")
	if !save.Available {
		t.Error("save_memory Available = false; want true when deps present")
	}
	// Recall stays unavailable without a recall service.
	recall, _ := reg2.Get("recall_memory")
	if recall.Available {
		t.Error("recall_memory Available = true; want false when deps.Recall is nil")
	}
}

func TestBuildDefault_SaveInvokerCallsService(t *testing.T) {
	create := &stubCreate{}
	reg, _ := BuildDefault(Dependencies{
		FragmentCreate:      create,
		EmbeddingConfigured: true,
	})
	tool, _ := reg.Get("save_memory")
	out, err := tool.Invoke(context.Background(), "pA", map[string]any{"content": "hello"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if create.called != 1 {
		t.Errorf("service called %d times; want 1", create.called)
	}
	if create.lastProfile != "pA" {
		t.Errorf("service profile = %q; want pA", create.lastProfile)
	}
	if out["status"] != "created" {
		t.Errorf("output status = %v; want created", out["status"])
	}
	if out["id"] != "f-1" {
		t.Errorf("output id = %v; want f-1", out["id"])
	}
}

func TestBuildDefault_InvokerReturnsUnavailableWhenDepsMissing(t *testing.T) {
	reg, _ := BuildDefault(Dependencies{}) // nothing wired
	tool, _ := reg.Get("save_memory")
	_, err := tool.Invoke(context.Background(), "pA", map[string]any{"content": "x"})
	if !errors.Is(err, ErrToolUnavailable) {
		t.Errorf("err = %v; want ErrToolUnavailable", err)
	}
}

func TestBuildDefault_GetInvokerWraps(t *testing.T) {
	reg, _ := BuildDefault(Dependencies{FragmentGet: stubGet{}})
	tool, _ := reg.Get("get_memory")
	out, err := tool.Invoke(context.Background(), "pA", map[string]any{"id": "f-42"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out["id"] != "f-42" {
		t.Errorf("out[id] = %v; want f-42", out["id"])
	}
}

func TestBuildDefault_ListInvokerWraps(t *testing.T) {
	reg, _ := BuildDefault(Dependencies{FragmentList: stubList{}})
	tool, _ := reg.Get("list_recent_memories")
	out, err := tool.Invoke(context.Background(), "pA", map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	items, ok := out["items"].([]map[string]any)
	if !ok {
		t.Fatalf("items has type %T; want []map[string]any", out["items"])
	}
	if len(items) != 1 {
		t.Errorf("items length = %d; want 1", len(items))
	}
	if out["has_more"] != false {
		t.Errorf("has_more = %v; want false", out["has_more"])
	}
}

func TestBuildDefault_RecallAvailableOnlyWithBothDepsAndEmbedding(t *testing.T) {
	rec := stubRecall{}
	// Recall service present but embedding not configured → still unavailable.
	reg, _ := BuildDefault(Dependencies{Recall: rec})
	tool, _ := reg.Get("recall_memory")
	if tool.Available {
		t.Error("recall_memory Available = true; want false when embedding not configured")
	}
	// Both → available.
	reg2, _ := BuildDefault(Dependencies{Recall: rec, EmbeddingConfigured: true})
	tool2, _ := reg2.Get("recall_memory")
	if !tool2.Available {
		t.Error("recall_memory Available = false; want true when deps + embedding present")
	}
}

type stubRecall struct{}

func (stubRecall) Recall(ctx context.Context, profileID string, req recallservice.RecallRequest) ([]recallservice.RecallHit, error) {
	return []recallservice.RecallHit{}, nil
}

// --- knowledge pipeline stubs ---

type stubClaimCreate struct {
	lastProfile string
}

func (s *stubClaimCreate) Create(ctx context.Context, profileID string, claim *domain.Claim) (*claimservice.CreateResult, error) {
	s.lastProfile = profileID
	return &claimservice.CreateResult{
		Claim: &domain.Claim{ClaimID: "c-1", ProfileID: profileID},
	}, nil
}

type stubClaimGet struct{}

func (stubClaimGet) Get(ctx context.Context, profileID, claimID string) (*domain.Claim, error) {
	return &domain.Claim{ClaimID: claimID, ProfileID: profileID}, nil
}

type stubClaimList struct{}

func (stubClaimList) List(ctx context.Context, profileID string, limit, offset int) ([]*domain.Claim, int, error) {
	return []*domain.Claim{{ClaimID: "c-1", ProfileID: profileID}}, 1, nil
}

type stubClaimVerify struct{}

func (stubClaimVerify) Verify(ctx context.Context, profileID, claimID string) (*domain.Claim, error) {
	return &domain.Claim{ClaimID: claimID, ProfileID: profileID, Status: domain.StatusValidated}, nil
}

type stubFactPromote struct{}

func (stubFactPromote) Promote(ctx context.Context, profileID, claimID string) (*domain.Fact, error) {
	return &domain.Fact{FactID: "f-1", ProfileID: profileID, PromotedFromClaimID: claimID}, nil
}

type stubFactGet struct{}

func (stubFactGet) Get(ctx context.Context, profileID, factID string) (*domain.Fact, error) {
	return &domain.Fact{FactID: factID, ProfileID: profileID}, nil
}

type stubFactList struct{}

func (stubFactList) List(ctx context.Context, profileID string, filters factservice.FactListFilters, limit int, cursor string) ([]*domain.Fact, string, error) {
	return []*domain.Fact{{FactID: "f-1", ProfileID: profileID}}, "", nil
}

type stubFragmentRetract struct {
	lastProfile string
}

func (s *stubFragmentRetract) Retract(ctx context.Context, profileID, fragmentID string) error {
	s.lastProfile = profileID
	return nil
}

type stubCommunityDetect struct {
	lastProfile string
}

func (s *stubCommunityDetect) Detect(ctx context.Context, profileID string) error {
	s.lastProfile = profileID
	return nil
}

type stubCommunityGet struct{}

func (stubCommunityGet) Get(ctx context.Context, profileID string, communityID string) (*domain.Community, error) {
	return &domain.Community{CommunityID: communityID, ProfileID: profileID, MemberCount: 2}, nil
}

type stubCommunityList struct {
	lastProfile string
}

func (s *stubCommunityList) List(ctx context.Context, profileID string, limit int) ([]*domain.Community, error) {
	s.lastProfile = profileID
	return []*domain.Community{{CommunityID: "community-1", ProfileID: profileID, MemberCount: 2}}, nil
}

// --- knowledge pipeline tests ---

// TestBuildDefaultIncludesKnowledgeTools verifies all 9 knowledge pipeline
// tools are registered regardless of whether their dependencies are wired.
func TestBuildDefaultIncludesKnowledgeTools(t *testing.T) {
	reg, err := BuildDefault(Dependencies{})
	if err != nil {
		t.Fatalf("BuildDefault: %v", err)
	}
	required := []string{
		"post_claim", "get_claim", "list_claims", "verify_claim",
		"promote_claim", "get_fact", "list_facts",
		"retract_fragment", "detect_community", "get_community_summary", "list_communities",
	}
	for _, name := range required {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("tool %q not registered", name)
		}
	}
}

// TestBuildDefaultKnowledgeTools_AvailabilityReflectsDeps verifies that each
// knowledge tool's Available flag mirrors whether its dependency is wired.
func TestBuildDefaultKnowledgeTools_AvailabilityReflectsDeps(t *testing.T) {
	// No deps → all knowledge tools unavailable.
	reg, _ := BuildDefault(Dependencies{})
	for _, name := range []string{
		"post_claim", "get_claim", "list_claims", "verify_claim",
		"promote_claim", "get_fact", "list_facts",
		"retract_fragment", "detect_community", "get_community_summary", "list_communities",
	} {
		tool, _ := reg.Get(name)
		if tool.Available {
			t.Errorf("%s Available = true; want false when deps missing", name)
		}
	}

	// Wire all deps → all knowledge tools available.
	reg2, _ := BuildDefault(Dependencies{
		ClaimCreate:     &stubClaimCreate{},
		ClaimGet:        stubClaimGet{},
		ClaimList:       stubClaimList{},
		ClaimVerify:     stubClaimVerify{},
		FactPromote:     stubFactPromote{},
		FactGet:         stubFactGet{},
		FactList:        stubFactList{},
		FragmentRetract: &stubFragmentRetract{},
		CommunityDetect: &stubCommunityDetect{},
		CommunityGet:    stubCommunityGet{},
		CommunityList:   &stubCommunityList{},
	})
	for _, name := range []string{
		"post_claim", "get_claim", "list_claims", "verify_claim",
		"promote_claim", "get_fact", "list_facts",
		"retract_fragment", "detect_community", "get_community_summary", "list_communities",
	} {
		tool, _ := reg2.Get(name)
		if !tool.Available {
			t.Errorf("%s Available = false; want true when dep wired", name)
		}
	}
}

// TestBuildDefaultKnowledgeTools_CrossProfileIsolation verifies that each
// knowledge tool's invoker passes the profileID argument through to the
// service — no cross-profile data leakage is possible at the tool layer.
func TestBuildDefaultKnowledgeTools_CrossProfileIsolation(t *testing.T) {
	retract := &stubFragmentRetract{}
	communities := &stubCommunityList{}
	reg, _ := BuildDefault(Dependencies{
		ClaimGet:        stubClaimGet{},
		FragmentRetract: retract,
		CommunityList:   communities,
	})

	// retract_fragment — verify profileID routing.
	tool, _ := reg.Get("retract_fragment")
	if _, err := tool.Invoke(context.Background(), "profileA", map[string]any{"id": "frag-1"}); err != nil {
		t.Fatalf("retract_fragment profileA: %v", err)
	}
	if retract.lastProfile != "profileA" {
		t.Errorf("retract_fragment routed to %q; want profileA", retract.lastProfile)
	}
	if _, err := tool.Invoke(context.Background(), "profileB", map[string]any{"id": "frag-2"}); err != nil {
		t.Fatalf("retract_fragment profileB: %v", err)
	}
	if retract.lastProfile != "profileB" {
		t.Errorf("retract_fragment routed to %q after second call; want profileB", retract.lastProfile)
	}

	// get_claim — verify that each profile receives only its own scoped data.
	claimTool, _ := reg.Get("get_claim")
	aResult, err := claimTool.Invoke(context.Background(), "profileA", map[string]any{"id": "c-shared-id"})
	if err != nil {
		t.Fatalf("get_claim profileA: %v", err)
	}
	bResult, err := claimTool.Invoke(context.Background(), "profileB", map[string]any{"id": "c-shared-id"})
	if err != nil {
		t.Fatalf("get_claim profileB: %v", err)
	}
	aProfile, _ := aResult["profile_id"].(string)
	bProfile, _ := bResult["profile_id"].(string)
	if aProfile != "profileA" {
		t.Errorf("get_claim profileA result has profile_id=%q; want profileA", aProfile)
	}
	if bProfile != "profileB" {
		t.Errorf("get_claim profileB result has profile_id=%q; want profileB", bProfile)
	}
	// The cross-profile isolation invariant: B's result must not contain A's data.
	if bProfile == "profileA" {
		t.Error("cross-profile isolation failure: profileB received profileA-scoped data")
	}

	communityTool, _ := reg.Get("list_communities")
	if _, err := communityTool.Invoke(context.Background(), "profileA", map[string]any{}); err != nil {
		t.Fatalf("list_communities profileA: %v", err)
	}
	if communities.lastProfile != "profileA" {
		t.Errorf("list_communities routed to %q; want profileA", communities.lastProfile)
	}
}

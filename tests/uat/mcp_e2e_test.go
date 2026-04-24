//go:build uat

package uat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpserver "github.com/dense-mem/dense-mem/internal/http"
	"github.com/dense-mem/dense-mem/internal/http/handler"
	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/dense-mem/dense-mem/internal/service"
	"github.com/dense-mem/dense-mem/internal/service/fragmentdedupe"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
	neo4jstorage "github.com/dense-mem/dense-mem/internal/storage/neo4j"
	pgclient "github.com/dense-mem/dense-mem/internal/storage/postgres"
	"github.com/dense-mem/dense-mem/internal/tools/registry"
	"github.com/stretchr/testify/require"
)

const testEmbeddingDimensions = 1536

type fixedEmbeddingProvider struct {
	model string
	dims  int
}

func (p *fixedEmbeddingProvider) Embed(_ context.Context, _ string) ([]float32, string, error) {
	return make([]float32, p.dims), p.model, nil
}

func (p *fixedEmbeddingProvider) EmbedBatch(_ context.Context, texts []string) ([][]float32, string, error) {
	out := make([][]float32, 0, len(texts))
	for range texts {
		out = append(out, make([]float32, p.dims))
	}
	return out, p.model, nil
}

func (p *fixedEmbeddingProvider) ModelName() string { return p.model }
func (p *fixedEmbeddingProvider) Dimensions() int   { return p.dims }
func (p *fixedEmbeddingProvider) IsAvailable() bool { return true }

type mcpHTTPClient struct {
	baseURL string
	apiKey  string
	nextID  int
}

func (c *mcpHTTPClient) call(t *testing.T, method string, params any) map[string]any {
	t.Helper()

	c.nextID++
	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      c.nextID,
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}

	data, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/mcp", bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	payload, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var out map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(payload), &out); err != nil {
		t.Fatalf("decode mcp response: %v\nbody=%s", err, string(payload))
	}
	return out
}

func toolResult(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()

	if errPayload, ok := resp["error"]; ok {
		t.Fatalf("unexpected mcp error: %#v", errPayload)
	}

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, "result should be an object")

	content, ok := result["content"].([]any)
	require.True(t, ok, "result.content should be present")
	require.NotEmpty(t, content, "result.content should contain one text payload")

	first, ok := content[0].(map[string]any)
	require.True(t, ok, "content item should be an object")

	text, ok := first["text"].(string)
	require.True(t, ok, "content[0].text should be a string")

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &payload))
	return payload
}

func startWritableMemoryServer(t *testing.T, env *TestEnv) (*httptest.Server, fragmentservice.GetFragmentService) {
	t.Helper()

	cfgProvider := env.buildConfig()
	cfgProvider.aiAPIURL = "http://stub-embedding.local"
	cfgProvider.aiAPIKey = "stub-key"
	cfgProvider.aiEmbeddingModel = "test-embedding"
	cfgProvider.aiEmbeddingDimensions = testEmbeddingDimensions

	cfgConcrete := env.buildConfigConcrete()
	cfgConcrete.AIAPIURL = cfgProvider.aiAPIURL
	cfgConcrete.AIAPIKey = cfgProvider.aiAPIKey
	cfgConcrete.AIEmbeddingModel = cfgProvider.aiEmbeddingModel
	cfgConcrete.AIEmbeddingDimensions = cfgProvider.aiEmbeddingDimensions

	logger := observability.New(slog.LevelError)
	server := httpserver.NewServer(cfgConcrete, logger, httpserver.HealthConfig{})

	profileScopeEnforcer := neo4jstorage.NewProfileScopeEnforcer(env.neo4jClient)
	readerAdapter := &scopedReaderAdapter{inner: profileScopeEnforcer}
	fragmentAuditor := &fragmentAuditAdapter{inner: env.auditService}
	lookup := fragmentdedupe.NewNeo4jDedupeLookup(readerAdapter)
	consistency := service.NewEmbeddingConsistencyService(pgclient.NewEmbeddingConfigRepository(env.db), cfgProvider)

	embedder := &fixedEmbeddingProvider{
		model: cfgProvider.aiEmbeddingModel,
		dims:  cfgProvider.aiEmbeddingDimensions,
	}

	fragmentCreateSvc := fragmentservice.NewCreateFragmentService(
		embedder,
		profileScopeEnforcer,
		lookup,
		fragmentAuditor,
		consistency,
		slog.Default(),
		nil,
	)
	fragmentGetSvc := fragmentservice.NewGetFragmentService(readerAdapter)
	fragmentListSvc := fragmentservice.NewListFragmentsService(readerAdapter)

	reg, err := registry.BuildDefault(registry.Dependencies{
		FragmentCreate: fragmentCreateSvc,
		FragmentGet:    fragmentGetSvc,
		FragmentList:   fragmentListSvc,
	})
	require.NoError(t, err)

	deps := httpserver.ProtectedDeps{
		APIKeyRepo:       env.apiKeyRepo,
		ProfileService:   env.profileSvc,
		ProfileSvc:       env.profileSvc,
		RateLimitService: env.rateLimitSvc,
		AuditService:     env.auditService,
		Config:           cfgProvider,
		Logger:           logger,
	}

	handlers := httpserver.ProtectedHandlers{
		APIKeySvc:      env.apiKeySvc,
		FragmentCreate: handler.NewFragmentCreateHandler(fragmentCreateSvc).Handle,
		FragmentRead:   handler.NewFragmentReadHandler(fragmentGetSvc).Handle,
		FragmentList:   handler.NewFragmentListHandler(fragmentListSvc).Handle,
		ToolCatalog:    handler.NewToolCatalogHandler(reg).Handle,
		MCPPost:        handler.NewMCPHandler(reg, logger).HandlePost,
		MCPGet:         handler.NewMCPHandler(reg, logger).HandleGet,
	}

	httpserver.RegisterProtectedRoutesWithHandlers(server, deps, handlers)
	httptestServer := httptest.NewServer(server)
	t.Cleanup(httptestServer.Close)

	return httptestServer, fragmentGetSvc
}

func createProfileAndKey(t *testing.T, ctx context.Context, env *TestEnv) (string, string) {
	t.Helper()

	profile, err := env.profileSvc.Create(ctx, service.CreateProfileRequest{
		Name:        fmt.Sprintf("uat-mcp-%d", time.Now().UnixNano()),
		Description: "MCP runtime verification profile",
	}, nil, "operator", "", "uat-mcp-runtime")
	require.NoError(t, err)

	_, rawKey, err := env.apiKeySvc.CreateStandardKey(ctx, profile.ID, service.CreateAPIKeyRequest{
		Label:     "uat-mcp-runtime",
		Scopes:    []string{"read", "write"},
		RateLimit: 0,
	}, nil, "operator", "", "uat-mcp-runtime")
	require.NoError(t, err)

	return profile.ID.String(), rawKey
}

func TestUATMCPRuntime_SaveMemoryPersistsAndReadsBack(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	profileID, rawAPIKey := createProfileAndKey(t, ctx, env)
	serverURL, fragmentGetSvc := startWritableMemoryServer(t, env)
	mcp := &mcpHTTPClient{baseURL: serverURL.URL, apiKey: rawAPIKey}

	initResp := mcp.call(t, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "uat-mcp-runtime",
			"version": "1.0.0",
		},
	})
	require.NotNil(t, initResp["result"], "initialize must succeed")

	saveResp := toolResult(t, mcp.call(t, "tools/call", map[string]any{
		"name": "save_memory",
		"arguments": map[string]any{
			"content":         "MCP persisted memory for runtime verification.",
			"source_type":     "manual",
			"authority":       "primary",
			"idempotency_key": "uat-mcp-runtime-save",
			"labels":          []string{"uat", "mcp"},
			"metadata": map[string]any{
				"origin": "uat",
			},
		},
	}))
	require.Equal(t, "created", saveResp["status"])

	fragmentID, ok := saveResp["id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, fragmentID)

	stored, err := fragmentGetSvc.GetByID(ctx, profileID, fragmentID)
	require.NoError(t, err)
	require.Equal(t, "MCP persisted memory for runtime verification.", stored.Content)
	require.Equal(t, "primary", string(stored.Authority))

	getResp := toolResult(t, mcp.call(t, "tools/call", map[string]any{
		"name": "get_memory",
		"arguments": map[string]any{
			"id":         fragmentID,
			"profile_id": "foreign-profile-should-be-ignored",
		},
	}))
	require.Equal(t, fragmentID, getResp["id"])
	require.Equal(t, "MCP persisted memory for runtime verification.", getResp["content"])
	require.Equal(t, "primary", getResp["authority"])

	listResp := toolResult(t, mcp.call(t, "tools/call", map[string]any{
		"name": "list_recent_memories",
		"arguments": map[string]any{
			"limit": 5,
		},
	}))
	items, ok := listResp["items"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, items)

	first, ok := items[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, fragmentID, first["id"])

	dupResp := toolResult(t, mcp.call(t, "tools/call", map[string]any{
		"name": "save_memory",
		"arguments": map[string]any{
			"content":         "MCP persisted memory for runtime verification.",
			"source_type":     "manual",
			"authority":       "primary",
			"idempotency_key": "uat-mcp-runtime-save",
		},
	}))
	require.Equal(t, "duplicate", dupResp["status"])
	require.Equal(t, fragmentID, dupResp["duplicate_of"])
}

package main

import (
	"context"
	"fmt"

	httpDto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/mcpclient"
	"github.com/dense-mem/dense-mem/internal/tools/registry"
)

var requiredMCPTools = []string{
	"save_memory",
	"get_memory",
	"list_recent_memories",
	"recall_memory",
	"keyword-search",
	"semantic-search",
	"graph-query",
	"post_claim",
	"get_claim",
	"list_claims",
	"verify_claim",
	"promote_claim",
	"get_fact",
	"list_facts",
	"retract_fragment",
	"detect_community",
	"get_community_summary",
	"list_communities",
}

func buildRemoteRegistry(ctx context.Context, client *mcpclient.Client, profileID string) (registry.Registry, error) {
	remoteCatalog, err := client.ListTools(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("fetch tool catalog: %w", err)
	}

	localRegistry, err := registry.BuildDefault(registry.Dependencies{
		FragmentCreate:      mcpclient.NewFragmentCreate(client),
		FragmentGet:         mcpclient.NewFragmentGet(client),
		FragmentList:        mcpclient.NewFragmentList(client),
		Recall:              mcpclient.NewRecall(client),
		KeywordSearch:       mcpclient.NewKeywordSearch(client),
		SemanticSearch:      mcpclient.NewSemanticSearch(client),
		GraphQuery:          mcpclient.NewGraphQuery(client),
		ClaimCreate:         mcpclient.NewClaimCreate(client),
		ClaimGet:            mcpclient.NewClaimGet(client),
		ClaimList:           mcpclient.NewClaimList(client),
		ClaimVerify:         mcpclient.NewClaimVerify(client),
		FactPromote:         mcpclient.NewClaimPromote(client),
		FactGet:             mcpclient.NewFactGet(client),
		FactList:            mcpclient.NewFactList(client),
		FragmentRetract:     mcpclient.NewFragmentRetract(client),
		CommunityDetect:     mcpclient.NewCommunityDetect(client),
		CommunityGet:        mcpclient.NewCommunityGet(client),
		CommunityList:       mcpclient.NewCommunityList(client),
		EmbeddingConfigured: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build local tool adapters: %w", err)
	}

	localByName := make(map[string]registry.Tool, len(localRegistry.List()))
	for _, tool := range localRegistry.List() {
		localByName[tool.Name] = tool
	}

	remoteByName := make(map[string]httpDto.ToolCatalogEntry, len(remoteCatalog.Tools))
	for _, tool := range remoteCatalog.Tools {
		remoteByName[tool.Name] = tool
	}
	for _, name := range requiredMCPTools {
		remoteTool, ok := remoteByName[name]
		if !ok {
			return nil, fmt.Errorf("required MCP tool missing from remote catalog: %s", name)
		}
		localTool, ok := localByName[name]
		if !ok || localTool.Invoke == nil {
			return nil, fmt.Errorf("required MCP tool has no local invoker: %s", name)
		}
		if remoteTool.Available && !localTool.Available {
			return nil, fmt.Errorf("required MCP tool unavailable locally: %s", name)
		}
	}

	reg := registry.New()
	for _, remoteTool := range remoteCatalog.Tools {
		localTool, ok := localByName[remoteTool.Name]
		if !ok || localTool.Invoke == nil {
			continue
		}
		if err := reg.Register(registry.Tool{
			Name:           remoteTool.Name,
			Description:    remoteTool.Description,
			InputSchema:    remoteTool.InputSchema,
			OutputSchema:   remoteTool.OutputSchema,
			RequiredScopes: remoteTool.RequiredScopes,
			Available:      remoteTool.Available,
			Invoke:         localTool.Invoke,
		}); err != nil {
			return nil, fmt.Errorf("register tool %s: %w", remoteTool.Name, err)
		}
	}

	return reg, nil
}

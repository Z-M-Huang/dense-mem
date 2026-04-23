package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	dto "github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/mcpclient"
)

func TestBuildRemoteRegistry_MirrorsRemoteMetadata(t *testing.T) {
	catalog := dto.ToolCatalogResponse{Tools: make([]dto.ToolCatalogEntry, 0, len(requiredMCPTools))}
	for _, name := range requiredMCPTools {
		entry := dto.ToolCatalogEntry{
			Name:           name,
			Description:    "remote " + name,
			InputSchema:    map[string]any{"type": "object", "title": name},
			OutputSchema:   map[string]any{"type": "object"},
			RequiredScopes: []string{"read"},
		}
		catalog.Tools = append(catalog.Tools, entry)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tools" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(catalog); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}))
	defer srv.Close()

	client := mcpclient.NewClient(srv.URL, "test-key", "profile-1")
	reg, err := buildRemoteRegistry(context.Background(), client, "profile-1")
	if err != nil {
		t.Fatalf("buildRemoteRegistry: %v", err)
	}

	saveTool, ok := reg.Get("save_memory")
	if !ok {
		t.Fatalf("save_memory not registered")
	}
	if saveTool.Description != "remote save_memory" {
		t.Fatalf("save_memory description = %q", saveTool.Description)
	}
	if saveTool.InputSchema["title"] != "save_memory" {
		t.Fatalf("save_memory input schema did not mirror remote catalog")
	}
}

func TestBuildRemoteRegistry_FailsWhenRequiredToolMissing(t *testing.T) {
	catalog := dto.ToolCatalogResponse{
		Tools: []dto.ToolCatalogEntry{
			{
				Name:         "get_memory",
				Description:  "remote get_memory",
				InputSchema:  map[string]any{"type": "object"},
				OutputSchema: map[string]any{"type": "object"},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(catalog); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}))
	defer srv.Close()

	client := mcpclient.NewClient(srv.URL, "test-key", "profile-1")
	_, err := buildRemoteRegistry(context.Background(), client, "profile-1")
	if err == nil {
		t.Fatal("expected missing required tool error")
	}
}

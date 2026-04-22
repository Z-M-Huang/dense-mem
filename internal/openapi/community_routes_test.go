package openapi

import "testing"

func TestGenerateIncludesCommunityReadRoutes(t *testing.T) {
	g := New(testRegistry(t), DefaultRoutes())

	spec, err := g.Generate(SpecVariantAISafe)
	if err != nil {
		t.Fatalf("Generate(AISafe): %v", err)
	}

	paths := spec["paths"].(map[string]any)
	listPath, ok := paths["/api/v1/communities"]
	if !ok {
		t.Fatalf("community list route missing")
	}
	getPath, ok := paths["/api/v1/communities/{id}"]
	if !ok {
		t.Fatalf("community read route missing")
	}

	listGet := listPath.(map[string]any)["get"].(map[string]any)
	readGet := getPath.(map[string]any)["get"].(map[string]any)

	if listGet["operationId"] != "listCommunities" {
		t.Fatalf("list operationId = %v; want listCommunities", listGet["operationId"])
	}
	if readGet["operationId"] != "getCommunitySummary" {
		t.Fatalf("read operationId = %v; want getCommunitySummary", readGet["operationId"])
	}

	components := spec["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	if _, ok := schemas["CommunityResponse"]; !ok {
		t.Fatalf("CommunityResponse schema missing")
	}
	if _, ok := schemas["ListCommunitiesResponse"]; !ok {
		t.Fatalf("ListCommunitiesResponse schema missing")
	}
}

package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dense-mem/dense-mem/internal/tools/registry"
)

func testRegistry(t *testing.T) registry.Registry {
	t.Helper()
	reg, err := registry.BuildDefault(registry.Dependencies{})
	if err != nil {
		t.Fatalf("BuildDefault: %v", err)
	}
	return reg
}

// TestGenerator_AISafeExcludesAdmin — backpressure case (AC-34).
func TestGenerator_AISafeExcludesAdmin(t *testing.T) {
	g := New(testRegistry(t), DefaultRoutes())
	spec, err := g.Generate(SpecVariantAISafe)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths missing or wrong type")
	}
	if _, present := paths["/api/v1/admin/graph/query"]; present {
		t.Errorf("admin path must NOT appear in ai-safe spec")
	}
	if _, present := paths["/api/v1/fragments"]; !present {
		t.Errorf("ai-safe spec must include /api/v1/fragments")
	}
}

func TestGenerator_FullIncludesAdmin(t *testing.T) {
	g := New(testRegistry(t), DefaultRoutes())
	spec, err := g.Generate(SpecVariantFull)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	paths := spec["paths"].(map[string]any)
	if _, present := paths["/api/v1/admin/graph/query"]; !present {
		t.Errorf("full spec must include /api/v1/admin/graph/query")
	}
	if _, present := paths["/api/v1/fragments"]; !present {
		t.Errorf("full spec must include /api/v1/fragments")
	}
}

func TestGenerator_ValidOpenAPIVersion(t *testing.T) {
	g := New(testRegistry(t), DefaultRoutes())
	spec, err := g.Generate(SpecVariantFull)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if spec["openapi"] != "3.0.3" {
		t.Errorf("openapi = %v; want 3.0.3", spec["openapi"])
	}
}

func TestGenerator_SecuritySchemesPresent(t *testing.T) {
	g := New(testRegistry(t), DefaultRoutes())
	spec, err := g.Generate(SpecVariantFull)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	components, ok := spec["components"].(map[string]any)
	if !ok {
		t.Fatalf("components missing")
	}
	ss, ok := components["securitySchemes"].(map[string]any)
	if !ok {
		t.Fatalf("securitySchemes missing")
	}
	if _, has := ss["ApiKeyAuth"]; !has {
		t.Errorf("ApiKeyAuth scheme missing")
	}
	if _, has := ss["ProfileHeader"]; !has {
		t.Errorf("ProfileHeader scheme missing")
	}
}

func TestGenerator_SchemasDerivedFromRegistry(t *testing.T) {
	reg, err := registry.BuildDefault(registry.Dependencies{})
	if err != nil {
		t.Fatalf("BuildDefault: %v", err)
	}
	g := New(reg, DefaultRoutes())
	spec, err := g.Generate(SpecVariantFull)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	components := spec["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)

	// save_memory -> SavememoryInput/Output per schemaNameFor naming
	if _, has := schemas["SavememoryInput"]; !has {
		t.Errorf("registry-derived schema SavememoryInput missing; have keys: %v", keysOf(schemas))
	}
	if _, has := schemas["SavememoryOutput"]; !has {
		t.Errorf("registry-derived schema SavememoryOutput missing")
	}
	if _, has := schemas["ErrorResponse"]; !has {
		t.Errorf("ErrorResponse schema missing")
	}
}

func TestGenerator_UnknownVariantErrors(t *testing.T) {
	g := New(testRegistry(t), DefaultRoutes())
	if _, err := g.Generate(SpecVariant("bogus")); err == nil {
		t.Errorf("expected error for unknown variant")
	}
}

func TestGenerator_JSONRoundTrips(t *testing.T) {
	g := New(testRegistry(t), DefaultRoutes())
	spec, err := g.Generate(SpecVariantAISafe)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), "\"openapi\":\"3.0.3\"") {
		t.Errorf("serialized spec missing version: %s", string(b)[:120])
	}
}

func TestGenerator_PathParamsDeclared(t *testing.T) {
	g := New(testRegistry(t), DefaultRoutes())
	spec, err := g.Generate(SpecVariantAISafe)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	paths := spec["paths"].(map[string]any)
	path := paths["/api/v1/fragments/{id}"].(map[string]any)
	get := path["get"].(map[string]any)
	params, ok := get["parameters"].([]map[string]any)
	if !ok || len(params) == 0 {
		t.Fatalf("GET /api/v1/fragments/{id} missing parameters block")
	}
	found := false
	for _, p := range params {
		if p["name"] == "id" {
			found = true
		}
	}
	if !found {
		t.Errorf("path param 'id' not declared")
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

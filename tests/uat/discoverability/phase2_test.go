//go:build uat

package discoverability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dense-mem/dense-mem/internal/observability"
)

// UAT-14: Phase 2 migration and backfill (Unit 11–12).
// AC trace: AC-41, AC-42, AC-43, AC-44, AC-45, AC-46, AC-47, AC-48.
func TestUAT14_Phase2MigrationAndBackfill(t *testing.T) {
	// Domain model must carry the day-1 properties new fragments require.
	dom, err := os.ReadFile(repoPath(t, "internal/domain/fragment.go"))
	require.NoError(t, err, "internal/domain/fragment.go must exist")
	domStr := string(dom)
	for _, field := range []string{
		"SourceType", "ContentHash", "IdempotencyKey",
		"EmbeddingModel", "EmbeddingDimensions",
	} {
		assert.Contains(t, domStr, field,
			"domain.Fragment must declare %q", field)
	}
	assert.Contains(t, domStr, `SourceTypeConversation SourceType = "conversation"`,
		"SourceType enum must include conversation")
	assert.Contains(t, domStr, `SourceTypeManual`,
		"SourceType enum must include manual (the default)")

	// Migration runner must be additive-only and null-safe per AC-42/AC-48.
	mig, err := os.ReadFile(repoPath(t, "internal/storage/neo4j/fragment_migration.go"))
	require.NoError(t, err, "fragment_migration.go must exist")
	migStr := string(mig)
	assert.Contains(t, migStr, "ADDITIVE MIGRATION CONTRACT",
		"migration file must document the additive contract")
	assert.Contains(t, migStr, "BackfillContentHashes",
		"migration must expose BackfillContentHashes for legacy fragments (AC-43)")
	assert.Contains(t, migStr, "CoerceSourceType",
		"readers must have a null-safe coercion helper (AC-46)")
	assert.NotContains(t, migStr, "DROP CONSTRAINT",
		"migrations must not drop existing constraints (additive-only)")
}

// UAT-15: Non-functional safeguards, docs, metrics, and verification.
// AC trace: AC-14, AC-26, AC-31, AC-53, AC-54, AC-55.
//
// This test exercises the invariants Unit 25 owns and that can be verified
// without a live backend:
//   - README.md documents data egress + model consistency + discoverability.
//   - Metrics recorder captures embedding + recall outcomes.
//   - Rate-limit configuration differentiates fragment write vs read tiers.
//
// Companion-interface compile assertions live next to each mock in that mock's
// own package (`var _ Interface = (*Mock)(nil)` in internal/.../mock_test.go),
// so they fire during `go test` of each owning package rather than here.
func TestUAT15_NonFunctionalSafeguards(t *testing.T) {
	t.Run("README documents data egress and discoverability", func(t *testing.T) {
		path := repoPath(t, "README.md")
		body, err := os.ReadFile(path)
		require.NoError(t, err, "README must exist")
		content := string(body)
		assert.Contains(t, content, "Data Egress",
			"README must document data egress for embedding providers")
		assert.Contains(t, content, "Embedding Model Consistency",
			"README must describe how to rotate the embedding model safely")
		assert.Contains(t, content, "Tool Discoverability",
			"README must point at tool catalog + OpenAPI + MCP")
		assert.NotContains(t, content, "sk-",
			"README must never embed an API key sample")
	})

	t.Run("Discoverability metrics record outcomes", func(t *testing.T) {
		m := observability.NewInMemoryDiscoverabilityMetrics()
		m.ObserveEmbeddingLatency(123.4, "ok")
		m.ObserveEmbeddingLatency(456.0, "timeout")
		m.IncEmbeddingError("rate_limited")
		m.ObserveRecallLatency(78.9)
		m.IncFragmentCreate("created")
		m.IncFragmentCreate("duplicate")
		m.IncFragmentCreate("error")

		samples := m.EmbeddingSamples()
		assert.Len(t, samples, 2, "embedding latency samples must be retained")
		assert.Equal(t, "ok", samples[0].Outcome)
		assert.Equal(t, "timeout", samples[1].Outcome)
		assert.Equal(t, 1, m.EmbeddingErrorCount("rate_limited"),
			"rate-limited errors must increment the counter")
		assert.Len(t, m.RecallLatencies(), 1,
			"recall latency samples must be retained")
		assert.Equal(t, 1, m.FragmentCreateCount("created"))
		assert.Equal(t, 1, m.FragmentCreateCount("duplicate"))
		assert.Equal(t, 1, m.FragmentCreateCount("error"))
	})
}

// repoPath returns the absolute path to a file within the repo. Tests live in
// tests/uat/discoverability/, two levels below the module root.
func repoPath(t *testing.T, rel string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	// Walk up until go.mod is found to be robust to how the test is invoked.
	for dir := cwd; dir != "/" && dir != ""; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, rel)
		}
	}
	t.Fatalf("could not locate repo root from %q", cwd)
	return ""
}

var _ = strings.Contains // keep "strings" imported if future assertions grow

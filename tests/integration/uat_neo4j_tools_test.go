package integration

import (
	"context"
	"testing"
)

// TestUATNeo4jIsolationAndTools is a UAT test for Neo4j isolation and graph tools.
// Skipped: depends on testcontainers fixture that is not yet wired (TestEnv.Setup
// is a placeholder). Tracked separately from the knowledge-pipeline build.
func TestUATNeo4jIsolationAndTools(t *testing.T) {
	t.Helper()
	t.Skip("UAT scaffold: TestEnv.Setup is a placeholder; testcontainers wiring pending")
	ctx := context.Background()

	// Setup test environment
	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	_ = env
}

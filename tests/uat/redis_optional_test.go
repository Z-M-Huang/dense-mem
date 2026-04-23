//go:build uat

package uat

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUATHealth_NoRedis_Degraded200(t *testing.T) {
	ctx := context.Background()
	env, cleanup := SetupTestEnv(t, ctx, TestEnvOptions{NoRedisMode: true})
	defer cleanup()

	resp, err := http.Get(env.GetServerURL() + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, true, body["degraded"])
	assert.Contains(t, body["reason"], "in-memory")
}

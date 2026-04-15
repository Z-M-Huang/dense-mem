package compose_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerComposeExample_RedisBehindProfile(t *testing.T) {
	b, err := os.ReadFile("../../docker-compose.example.yml")
	require.NoError(t, err)

	text := string(b)
	assert.Contains(t, text, `profiles: ["redis"]`)
	assert.NotContains(t, text, "redis:\n        condition: service_healthy")
}

func TestDockerCompose_DefaultCompose_MarksRedisOptional(t *testing.T) {
	b, err := os.ReadFile("../../docker-compose.yml")
	require.NoError(t, err)

	text := string(b)
	assert.Contains(t, text, "optional for single-node")
	assert.NotContains(t, text, "redis:\n        condition: service_healthy")
}

//go:build uat

package uat

import (
	"bytes"
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

func TestUATProfileDelete_NoRedis_NoPanic(t *testing.T) {
	ctx := context.Background()
	env, cleanup := SetupTestEnv(t, ctx, TestEnvOptions{NoRedisMode: true})
	defer cleanup()

	assert.True(t, env.IsNoRedisMode())
	assert.Empty(t, env.GetRedisAddr())

	_, adminRawKey := env.AdminKey()

	// Create a profile
	createProfileReq := map[string]interface{}{
		"name":        "uat-delete-noredis",
		"description": "Profile for no-redis delete test",
	}
	createProfileBody, _ := json.Marshal(createProfileReq)

	req, _ := http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles", bytes.NewReader(createProfileBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var profileResp envelope
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&profileResp))
	profileData, ok := profileResp.Data.(map[string]interface{})
	require.True(t, ok)
	profileIDStr, ok := profileData["id"].(string)
	require.True(t, ok)

	// Delete the profile — exercises the cleanup call path without Redis; must not panic
	req, _ = http.NewRequest("DELETE", env.GetServerURL()+"/api/v1/profiles/"+profileIDStr, nil)
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp2, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode, "profile delete should succeed without Redis")

	var deleteResp envelope
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&deleteResp))
	deleteData, ok := deleteResp.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "deleted", deleteData["status"])
}

func TestUATRateLimit_NoRedis(t *testing.T) {
	ctx := context.Background()
	env, cleanup := SetupTestEnv(t, ctx, TestEnvOptions{NoRedisMode: true, RateLimitPerMinute: 2})
	defer cleanup()

	_, adminRawKey := env.AdminKey()

	// Create a profile
	createProfileReq := map[string]interface{}{
		"name":        "uat-ratelimit-noredis",
		"description": "Profile for no-redis rate limit test",
	}
	createProfileBody, _ := json.Marshal(createProfileReq)

	req, _ := http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles", bytes.NewReader(createProfileBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var profileResp envelope
	err = json.NewDecoder(resp.Body).Decode(&profileResp)
	require.NoError(t, err)
	profileData, ok := profileResp.Data.(map[string]interface{})
	require.True(t, ok)
	profileIDStr, ok := profileData["id"].(string)
	require.True(t, ok)
	profileID := profileIDStr

	// Create a standard key with very low rate limit
	createKeyReq := map[string]interface{}{
		"label":      "uat-ratelimit-noredis-key",
		"scopes":     []string{"read"},
		"rate_limit": 2,
	}
	createKeyBody, _ := json.Marshal(createKeyReq)

	req, _ = http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles/"+profileID+"/api-keys", bytes.NewReader(createKeyBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var keyEnvelope envelope
	err = json.NewDecoder(resp.Body).Decode(&keyEnvelope)
	require.NoError(t, err)
	keyRespData, ok := keyEnvelope.Data.(map[string]interface{})
	require.True(t, ok)
	rawAPIKey, ok := keyRespData["api_key"].(string)
	require.True(t, ok)

	// Make requests until rate limited
	got429 := false
	for i := 0; i < 10; i++ {
		req, _ = http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles/"+profileID, nil)
		req.Header.Set("Authorization", "Bearer "+rawAPIKey)

		resp, err = http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			got429 = true
			assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "rate limit should return 429")
			assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Limit"), "X-RateLimit-Limit header must be present")
			assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Remaining"), "X-RateLimit-Remaining header must be present")
			assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Reset"), "X-RateLimit-Reset header must be present")
			assert.NotEmpty(t, resp.Header.Get("Retry-After"), "Retry-After header must be present on 429")
			break
		}
	}
	require.True(t, got429, "should have gotten 429 after exceeding rate limit of 2")
}

func TestUATAPIKeyRevoke_NoRedis_NoPanic(t *testing.T) {
	ctx := context.Background()
	env, cleanup := SetupTestEnv(t, ctx, TestEnvOptions{NoRedisMode: true})
	defer cleanup()

	_, adminRawKey := env.AdminKey()

	// Create a profile
	createProfileReq := map[string]interface{}{
		"name":        "uat-revoke-noredis",
		"description": "Profile for no-redis revoke test",
	}
	createProfileBody, _ := json.Marshal(createProfileReq)

	req, _ := http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles", bytes.NewReader(createProfileBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var profileResp envelope
	err = json.NewDecoder(resp.Body).Decode(&profileResp)
	require.NoError(t, err)
	profileData, ok := profileResp.Data.(map[string]interface{})
	require.True(t, ok)
	profileIDStr, ok := profileData["id"].(string)
	require.True(t, ok)
	profileID := profileIDStr

	// Create a key
	createKeyReq := map[string]interface{}{
		"label":  "uat-revoke-noredis-key",
		"scopes": []string{"read"},
	}
	createKeyBody, _ := json.Marshal(createKeyReq)

	req, _ = http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles/"+profileID+"/api-keys", bytes.NewReader(createKeyBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var keyEnv envelope
	err = json.NewDecoder(resp.Body).Decode(&keyEnv)
	require.NoError(t, err)
	keyData, ok := keyEnv.Data.(map[string]interface{})
	require.True(t, ok)
	keyMap, ok := keyData["key"].(map[string]interface{})
	require.True(t, ok)
	keyIDStr, ok := keyMap["id"].(string)
	require.True(t, ok)
	keyID := keyIDStr

	// Revoke the key - should not panic without Redis
	req, _ = http.NewRequest("DELETE", env.GetServerURL()+"/api/v1/profiles/"+profileID+"/api-keys/"+keyID, nil)
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "revoke should succeed without Redis")
}

//go:build uat

package uat

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UAT-6: API key lifecycle with profile association
// Tests: Create standard key, list keys, get key, revoke key
func TestUATAPIKeyLifecycle(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	_, adminRawKey := env.AdminKey()

	// First, create a profile for API key association
	createProfileReq := map[string]interface{}{
		"name":        "uat-apikey-profile",
		"description": "Profile for API key lifecycle test",
	}
	createProfileBody, _ := json.Marshal(createProfileReq)

	req, _ := http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles", bytes.NewReader(createProfileBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "profile create should succeed")
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "profile create should return 201")

	var profileResp envelope
	err = json.NewDecoder(resp.Body).Decode(&profileResp)
	require.NoError(t, err, "profile response should be valid JSON")
	profileData, ok := profileResp.Data.(map[string]interface{})
	require.True(t, ok, "profile data should be a map")
	profileIDStr, ok := profileData["id"].(string)
	require.True(t, ok, "profile ID should be a string")
	profileID, err := uuid.Parse(profileIDStr)
	require.NoError(t, err, "profile ID should be valid UUID")

	// UAT-6: Create standard API key for the profile
	createKeyReq := map[string]interface{}{
		"label":      "uat-test-key",
		"scopes":     []string{"read", "write"},
		"rate_limit": 100,
	}
	createKeyBody, _ := json.Marshal(createKeyReq)

	req, _ = http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles/"+profileID.String()+"/api-keys", bytes.NewReader(createKeyBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err, "API key create should succeed")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "API key create should return 201")

	var keyEnvelope envelope
	err = json.NewDecoder(resp.Body).Decode(&keyEnvelope)
	require.NoError(t, err, "API key response should be valid JSON")

	keyRespData, ok := keyEnvelope.Data.(map[string]interface{})
	require.True(t, ok, "key response data should be a map")

	// The raw key should be returned exactly once
	rawAPIKey, ok := keyRespData["api_key"].(string)
	require.True(t, ok, "api_key should be a string")
	assert.NotEmpty(t, rawAPIKey, "raw API key should be returned")

	keyData, ok := keyRespData["key"].(map[string]interface{})
	require.True(t, ok, "key data should be a map")
	keyIDStr, ok := keyData["id"].(string)
	require.True(t, ok, "key ID should be a string")
	keyID, err := uuid.Parse(keyIDStr)
	require.NoError(t, err, "key ID should be valid UUID")

	// Verify scopes are set
	scopes, ok := keyData["scopes"].([]interface{})
	require.True(t, ok, "scopes should be an array")
	assert.Contains(t, scopes, "read", "should have read scope")
	assert.Contains(t, scopes, "write", "should have write scope")

	// UAT-6: List API keys for the profile
	req, _ = http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles/"+profileID.String()+"/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err, "API key list should succeed")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "API key list should return 200")

	var listResp paginationEnvelope
	err = json.NewDecoder(resp.Body).Decode(&listResp)
	require.NoError(t, err, "API key list response should be valid JSON")
	
	dataArray, ok := listResp.Data.([]interface{})
	require.True(t, ok, "data should be an array")
	assert.GreaterOrEqual(t, len(dataArray), 1, "should have at least 1 key")

	// UAT-6: Authenticate with the new key and verify it works
	req, _ = http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles/"+profileID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+rawAPIKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err, "authenticated request should succeed")
	defer resp.Body.Close()

	// Standard key should be able to read own profile
	assert.Equal(t, http.StatusOK, resp.StatusCode, "standard key should be able to read own profile")

	// UAT-6: Revoke the API key
	req, _ = http.NewRequest("DELETE", env.GetServerURL()+"/api/v1/profiles/"+profileID.String()+"/api-keys/"+keyID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err, "API key revoke should succeed")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "API key revoke should return 200")

	var revokeResp envelope
	err = json.NewDecoder(resp.Body).Decode(&revokeResp)
	require.NoError(t, err, "revoke response should be valid JSON")
	revokeData, ok := revokeResp.Data.(map[string]interface{})
	require.True(t, ok, "revoke response should be a map")
	assert.Equal(t, "revoked", revokeData["status"], "revoke should return status=revoked")

	// UAT-6: Verify revoked key cannot authenticate
	req, _ = http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles/"+profileID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+rawAPIKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err, "revoked key request should return error")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "revoked key should return 401")
}

// UAT-7: Admin key bypasses all authorization checks
func TestUATAdminKeyBypass(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	_, adminRawKey := env.AdminKey()

	// Create a profile
	createProfileReq := map[string]interface{}{
		"name":        "uat-admin-bypass-profile",
		"description": "Profile for admin bypass test",
	}
	createProfileBody, _ := json.Marshal(createProfileReq)

	req, _ := http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles", bytes.NewReader(createProfileBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "profile create should succeed")
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var profileResp envelope
	err = json.NewDecoder(resp.Body).Decode(&profileResp)
	require.NoError(t, err)
	profileData, ok := profileResp.Data.(map[string]interface{})
	require.True(t, ok)
	profileIDStr, ok := profileData["id"].(string)
	require.True(t, ok)
	profileID, err := uuid.Parse(profileIDStr)
	require.NoError(t, err)

	// Create a standard key for another profile
	createKeyReq := map[string]interface{}{
		"label":  "uat-cross-profile-key",
		"scopes": []string{"read"},
	}
	createKeyBody, _ := json.Marshal(createKeyReq)

	req, _ = http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles/"+profileID.String()+"/api-keys", bytes.NewReader(createKeyBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Create another profile
	createProfileReq2 := map[string]interface{}{
		"name":        "uat-admin-bypass-profile-2",
		"description": "Second profile for cross-profile test",
	}
	createProfileBody2, _ := json.Marshal(createProfileReq2)

	req, _ = http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles", bytes.NewReader(createProfileBody2))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var profileResp2 envelope
	err = json.NewDecoder(resp.Body).Decode(&profileResp2)
	require.NoError(t, err)
	profileData2, ok := profileResp2.Data.(map[string]interface{})
	require.True(t, ok)
	profileIDStr2, ok := profileData2["id"].(string)
	require.True(t, ok)
	profileID2, err := uuid.Parse(profileIDStr2)
	require.NoError(t, err)

	// UAT-7: Admin can access any profile (bypasses cross-profile check)
	req, _ = http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles/"+profileID2.String(), nil)
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "admin should be able to access any profile")
}

// UAT-8: Cross-profile access is denied for standard keys (AC-30)
func TestUATCrossProfileDenial(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	_, adminRawKey := env.AdminKey()

	// Create profile 1
	createProfileReq1 := map[string]interface{}{
		"name":        "uat-cross-denial-1",
		"description": "First profile",
	}
	createProfileBody1, _ := json.Marshal(createProfileReq1)

	req, _ := http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles", bytes.NewReader(createProfileBody1))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var profileResp1 envelope
	err = json.NewDecoder(resp.Body).Decode(&profileResp1)
	require.NoError(t, err)
	profileData1, ok := profileResp1.Data.(map[string]interface{})
	require.True(t, ok)
	profileIDStr1, ok := profileData1["id"].(string)
	require.True(t, ok)
	profileID1, err := uuid.Parse(profileIDStr1)
	require.NoError(t, err)

	// Create profile 2
	createProfileReq2 := map[string]interface{}{
		"name":        "uat-cross-denial-2",
		"description": "Second profile",
	}
	createProfileBody2, _ := json.Marshal(createProfileReq2)

	req, _ = http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles", bytes.NewReader(createProfileBody2))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var profileResp2 envelope
	err = json.NewDecoder(resp.Body).Decode(&profileResp2)
	require.NoError(t, err)
	profileData2, ok := profileResp2.Data.(map[string]interface{})
	require.True(t, ok)
	profileIDStr2, ok := profileData2["id"].(string)
	require.True(t, ok)
	profileID2, err := uuid.Parse(profileIDStr2)
	require.NoError(t, err)

	// Create a standard key for profile 1
	createKeyReq := map[string]interface{}{
		"label":  "uat-cross-denial-key",
		"scopes": []string{"read", "write"},
	}
	createKeyBody, _ := json.Marshal(createKeyReq)

	req, _ = http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles/"+profileID1.String()+"/api-keys", bytes.NewReader(createKeyBody))
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
	require.NotEmpty(t, rawAPIKey)

	// UAT-8: Standard key from profile 1 cannot access profile 2
	req, _ = http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles/"+profileID2.String(), nil)
	req.Header.Set("Authorization", "Bearer "+rawAPIKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return 403 Forbidden (cross-profile denied)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "cross-profile access should be denied")

	var errResp errorEnvelope
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)
	assert.Contains(t, errResp.Error.Code, "FORBIDDEN", "error code should indicate forbidden")
}
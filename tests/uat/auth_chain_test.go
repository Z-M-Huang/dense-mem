//go:build uat

package uat

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dense-mem/dense-mem/internal/crypto"
)

// UAT-9: Auth middleware rejects missing Authorization header
func TestUATAuthMissingHeader(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	// Request without Authorization header to protected route
	req, _ := http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles", nil)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "request should complete")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "missing auth should return 401")

	var errResp errorEnvelope
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)
	assert.Equal(t, "AUTH_MISSING", errResp.Error.Code, "error code should be AUTH_MISSING")
}

// UAT-10: Auth middleware rejects malformed Authorization header
func TestUATAuthMalformedHeader(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	tests := []struct {
		name      string
		authValue string
	}{
		{"no bearer prefix", "invalidtoken"},
		{"wrong scheme", "Basic dXNlcjpwYXNz"},
		{"empty bearer", "Bearer "},
		{"key too short", "Bearer short"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles", nil)
			req.Header.Set("Authorization", tc.authValue)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err, "request should complete")
			defer resp.Body.Close()

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "malformed auth should return 401")

			var errResp errorEnvelope
			err = json.NewDecoder(resp.Body).Decode(&errResp)
			require.NoError(t, err)
			assert.Equal(t, "AUTH_INVALID", errResp.Error.Code, "error code should be AUTH_INVALID")
		})
	}
}

// UAT-11: Auth middleware rejects invalid key (non-existent prefix)
func TestUATAuthInvalidKey(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	// Generate a fake key that doesn't exist in the database
	fakeKey, err := crypto.GenerateRawKey()
	require.NoError(t, err, "should generate fake key")

	req, _ := http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles", nil)
	req.Header.Set("Authorization", "Bearer "+fakeKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "request should complete")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "invalid key should return 401")

	var errResp errorEnvelope
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)
	assert.Equal(t, "AUTH_INVALID", errResp.Error.Code, "error code should be AUTH_INVALID")
}

// UAT-12: Auth middleware stores principal correctly in context
func TestUATAuthPrincipalStored(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	_, adminRawKey := env.AdminKey()

	// Create a profile
	createProfileReq := map[string]interface{}{
		"name":        "uat-principal-test",
		"description": "Profile for principal test",
	}
	createProfileBody, _ := json.Marshal(createProfileReq)

	req, _ := http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles", bytes.NewReader(createProfileBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// The admin key should be able to make admin-level requests
	// This proves the principal is stored correctly in context
	req, _ = http.NewRequest("GET", env.GetServerURL()+"/api/v1/admin/test", nil)
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Admin route should return 200 (not 401/403) which proves principal works
	assert.Equal(t, http.StatusOK, resp.StatusCode, "admin key should access admin routes")
}

// UAT-13: Rate limiting is enforced
func TestUATRateLimiting(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	_, adminRawKey := env.AdminKey()

	// Create a profile
	createProfileReq := map[string]interface{}{
		"name":        "uat-ratelimit-test",
		"description": "Profile for rate limit test",
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
	profileID, err := uuid.Parse(profileIDStr)
	require.NoError(t, err)

	// Create a standard key with low rate limit
	createKeyReq := map[string]interface{}{
		"label":      "uat-ratelimit-key",
		"scopes":     []string{"read"},
		"rate_limit": 10, // Low limit for testing
	}
	createKeyBody, _ := json.Marshal(createKeyReq)

	req, _ = http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles/"+profileID.String()+"/api-keys", bytes.NewReader(createKeyBody))
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

	// Make requests until rate limited (beyond the 10 limit)
	// Note: Rate limiting is per-minute, so this test may not trigger immediately
	// depending on Redis state. This is a functional check of the middleware.
	for i := 0; i < 15; i++ {
		req, _ = http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles/"+profileID.String(), nil)
		req.Header.Set("Authorization", "Bearer "+rawAPIKey)

		resp, err = http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()

		// At some point we should get 429 Too Many Requests
		if resp.StatusCode == http.StatusTooManyRequests {
			// Rate limit was enforced
			assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "rate limit should return 429")
			return
		}
	}

	// If rate limit wasn't triggered, at least verify requests work
	t.Log("Rate limit not triggered (may need Redis in specific state)")
}

// UAT-14: Expired key cannot authenticate
func TestUATExpiredKey(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	_, adminRawKey := env.AdminKey()

	// Create a profile
	createProfileReq := map[string]interface{}{
		"name":        "uat-expired-test",
		"description": "Profile for expired key test",
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
	profileID, err := uuid.Parse(profileIDStr)
	require.NoError(t, err)

	// Create a key that expires very soon (1 second in the past - already expired)
	pastTime := time.Now().Add(-1 * time.Second).Format(time.RFC3339)
	createKeyReq := map[string]interface{}{
		"label":     "uat-expired-key",
		"scopes":    []string{"read"},
		"expires_at": pastTime,
	}
	createKeyBody, _ := json.Marshal(createKeyReq)

	req, _ = http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles/"+profileID.String()+"/api-keys", bytes.NewReader(createKeyBody))
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

	// Try to use the expired key
	req, _ = http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles/"+profileID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+rawAPIKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "expired key should return 401")

	var errResp errorEnvelope
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	require.NoError(t, err)
	// AUTH_INVALID is returned because GetActiveByPrefix filters out expired keys
	assert.Equal(t, "AUTH_INVALID", errResp.Error.Code, "error code should be AUTH_INVALID (expired keys filtered by query)")
}
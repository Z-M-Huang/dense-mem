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

// UAT-4: Profile CRUD lifecycle
// Tests: Create, List, Get, Patch, Delete profiles with admin key
func TestUATProfileCRUDLifecycle(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	_, adminRawKey := env.AdminKey()

	// UAT-4: Admin creates a profile
	createReq := map[string]interface{}{
		"name":        "uat-profile-test",
		"description": "UAT test profile for CRUD lifecycle",
	}
	createBody, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "profile create request should succeed")
	defer resp.Body.Close()

	// Should return 201 Created
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "profile create should return 201")

	var createResp envelope
	err = json.NewDecoder(resp.Body).Decode(&createResp)
	require.NoError(t, err, "profile create response should be valid JSON")

	profileData, ok := createResp.Data.(map[string]interface{})
	require.True(t, ok, "profile data should be a map")
	profileIDStr, ok := profileData["id"].(string)
	require.True(t, ok, "profile ID should be a string")
	profileID, err := uuid.Parse(profileIDStr)
	require.NoError(t, err, "profile ID should be valid UUID")

	// UAT-4: Admin lists profiles
	req, _ = http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles", nil)
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err, "profile list request should succeed")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "profile list should return 200")

	var listResp paginationEnvelope
	err = json.NewDecoder(resp.Body).Decode(&listResp)
	require.NoError(t, err, "profile list response should be valid JSON")
	assert.GreaterOrEqual(t, int(listResp.Pagination.Total), 1, "should have at least 1 profile")

	// UAT-4: Admin gets the profile
	req, _ = http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles/"+profileID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err, "profile get request should succeed")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "profile get should return 200")

	var getResp envelope
	err = json.NewDecoder(resp.Body).Decode(&getResp)
	require.NoError(t, err, "profile get response should be valid JSON")
	getData, ok := getResp.Data.(map[string]interface{})
	require.True(t, ok, "profile data should be a map")
	assert.Equal(t, profileID.String(), getData["id"], "profile ID should match")

	// UAT-4: Admin updates the profile
	updateReq := map[string]interface{}{
		"description": "Updated description for UAT test",
	}
	updateBody, _ := json.Marshal(updateReq)

	req, _ = http.NewRequest("PATCH", env.GetServerURL()+"/api/v1/profiles/"+profileID.String(), bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err, "profile update request should succeed")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "profile update should return 200")

	var updateResp envelope
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	require.NoError(t, err, "profile update response should be valid JSON")
	updateData, ok := updateResp.Data.(map[string]interface{})
	require.True(t, ok, "profile data should be a map")
	assert.Equal(t, "Updated description for UAT test", updateData["description"], "description should be updated")

	// UAT-4: Admin deletes the profile
	req, _ = http.NewRequest("DELETE", env.GetServerURL()+"/api/v1/profiles/"+profileID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err, "profile delete request should succeed")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "profile delete should return 200")

	var deleteResp envelope
	err = json.NewDecoder(resp.Body).Decode(&deleteResp)
	require.NoError(t, err, "profile delete response should be valid JSON")
	deleteData, ok := deleteResp.Data.(map[string]interface{})
	require.True(t, ok, "delete response should be a map")
	assert.Equal(t, "deleted", deleteData["status"], "delete should return status=deleted")
}

// UAT-5: Profile pagination
// Tests: limit, offset, total count in pagination envelope
func TestUATProfilePagination(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	env, cleanup := SetupTestEnv(t, ctx)
	defer cleanup()

	_, adminRawKey := env.AdminKey()

	// Create multiple profiles for pagination testing
	for i := 0; i < 5; i++ {
		createReq := map[string]interface{}{
			"name":        "uat-pagination-" + string(rune('a'+i)),
			"description": "Pagination test profile",
		}
		createBody, _ := json.Marshal(createReq)

		req, _ := http.NewRequest("POST", env.GetServerURL()+"/api/v1/profiles", bytes.NewReader(createBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminRawKey)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, "profile create should succeed")
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode, "profile create should return 201")
	}

	// UAT-5: List with pagination
	req, _ := http.NewRequest("GET", env.GetServerURL()+"/api/v1/profiles?limit=2&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+adminRawKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "profile list with pagination should succeed")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "paginated list should return 200")

	var pagResp paginationEnvelope
	err = json.NewDecoder(resp.Body).Decode(&pagResp)
	require.NoError(t, err, "pagination response should be valid JSON")
	assert.Equal(t, 2, pagResp.Pagination.Limit, "limit should be 2")
	assert.Equal(t, 0, pagResp.Pagination.Offset, "offset should be 0")
	assert.GreaterOrEqual(t, int(pagResp.Pagination.Total), 5, "total should be at least 5")

	// Verify data array length matches limit
	dataArray, ok := pagResp.Data.([]interface{})
	require.True(t, ok, "data should be an array")
	assert.LessOrEqual(t, len(dataArray), 2, "data array length should not exceed limit")
}

// Helper types for JSON responses
type envelope struct {
	Data interface{} `json:"data"`
}

type paginationEnvelope struct {
	Data       interface{} `json:"data"`
	Pagination pagination  `json:"pagination"`
}

type pagination struct {
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
	Total  int64 `json:"total"`
}

type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuccessEnvelope(t *testing.T) {
	e := echo.New()

	t.Run("writes data wrapper with correct status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		testData := map[string]interface{}{
			"id":   "123",
			"name": "test",
		}

		err := Success(c, http.StatusOK, testData)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, rec.Code)

		var result map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &result)
		require.NoError(t, err)

		// Verify top-level "data" key exists
		data, ok := result["data"]
		require.True(t, ok, "expected 'data' key at top level")

		dataMap, ok := data.(map[string]interface{})
		require.True(t, ok, "expected data to be an object")
		assert.Equal(t, "123", dataMap["id"])
		assert.Equal(t, "test", dataMap["name"])
	})

	t.Run("SuccessOK writes 200 status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := SuccessOK(c, "hello")
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"data":"hello"`)
	})

	t.Run("SuccessCreated writes 201 status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := SuccessCreated(c, map[string]string{"id": "new-id"})
		require.NoError(t, err)

		assert.Equal(t, http.StatusCreated, rec.Code)
		assert.Contains(t, rec.Body.String(), `"data"`)
		assert.Contains(t, rec.Body.String(), `"id":"new-id"`)
	})

	t.Run("SuccessNoContent writes 204 status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := SuccessNoContent(c)
		require.NoError(t, err)

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("handles nil data", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := Success(c, http.StatusOK, nil)
		require.NoError(t, err)

		assert.Contains(t, rec.Body.String(), `"data":null`)
	})

	t.Run("handles array data", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		testData := []string{"item1", "item2", "item3"}

		err := Success(c, http.StatusOK, testData)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(rec.Body.Bytes(), &result)
		require.NoError(t, err)

		data, ok := result["data"].([]interface{})
		require.True(t, ok)
		assert.Len(t, data, 3)
		assert.Equal(t, "item1", data[0])
		assert.Equal(t, "item2", data[1])
		assert.Equal(t, "item3", data[2])
	})
}
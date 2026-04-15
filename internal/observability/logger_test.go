package observability

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggerNeverEmitsSecrets(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := NewWithHandler(handler)

	t.Run("never logs key_hash", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", String("key_hash", "sensitive-hash-value"))
		output := buf.String()
		assert.NotContains(t, output, "key_hash")
		assert.NotContains(t, output, "sensitive-hash-value")
	})

	t.Run("never logs encrypted_secret", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", String("encrypted_secret", "sensitive-secret"))
		output := buf.String()
		assert.NotContains(t, output, "encrypted_secret")
		assert.NotContains(t, output, "sensitive-secret")
	})

	t.Run("never logs api_key", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", String("api_key", "sk-1234567890"))
		output := buf.String()
		assert.NotContains(t, output, "api_key")
		assert.NotContains(t, output, "sk-1234567890")
	})

	t.Run("never logs raw_key", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", String("raw_key", "raw-key-value"))
		output := buf.String()
		assert.NotContains(t, output, "raw_key")
		assert.NotContains(t, output, "raw-key-value")
	})

	t.Run("never logs secret", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", String("secret", "my-secret"))
		output := buf.String()
		assert.NotContains(t, output, "secret")
		assert.NotContains(t, output, "my-secret")
	})

	t.Run("never logs password", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", String("password", "my-password"))
		output := buf.String()
		assert.NotContains(t, output, "password")
		assert.NotContains(t, output, "my-password")
	})

	t.Run("never logs token", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", String("token", "bearer-token-123"))
		output := buf.String()
		assert.NotContains(t, output, "token")
		assert.NotContains(t, output, "bearer-token-123")
	})

	t.Run("never logs vector", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", String("vector", "[0.1, 0.2, 0.3]"))
		output := buf.String()
		assert.NotContains(t, output, "vector")
		assert.NotContains(t, output, "[0.1, 0.2, 0.3]")
	})

	t.Run("never logs embedding", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", String("embedding", "[0.1, 0.2]"))
		output := buf.String()
		assert.NotContains(t, output, "embedding")
	})

	t.Run("never logs embeddings", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", String("embeddings", "[[0.1, 0.2]]"))
		output := buf.String()
		assert.NotContains(t, output, "embeddings")
	})

	t.Run("logs safe fields normally", func(t *testing.T) {
		buf.Reset()
		logger.Info("test",
			String("correlation_id", "corr-123"),
			String("client_ip", "192.168.1.1"),
			String("profile_id", "profile-456"),
			String("key_id", "key-789"),
			String("key_prefix", "dm_"),
		)
		output := buf.String()
		assert.Contains(t, output, "correlation_id")
		assert.Contains(t, output, "corr-123")
		assert.Contains(t, output, "client_ip")
		assert.Contains(t, output, "192.168.1.1")
		assert.Contains(t, output, "profile_id")
		assert.Contains(t, output, "profile-456")
		assert.Contains(t, output, "key_id")
		assert.Contains(t, output, "key-789")
		assert.Contains(t, output, "key_prefix")
		assert.Contains(t, output, "dm_")
	})

	t.Run("Error method filters secrets", func(t *testing.T) {
		buf.Reset()
		logger.Error("error occurred", assert.AnError,
			String("key_hash", "secret-hash"),
			String("safe_field", "safe-value"),
		)
		output := buf.String()
		assert.NotContains(t, output, "key_hash")
		assert.NotContains(t, output, "secret-hash")
		assert.Contains(t, output, "safe_field")
		assert.Contains(t, output, "safe-value")
	})
}

func TestLoggerConvenienceFunctions(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := NewWithHandler(handler)

	t.Run("CorrelationID helper", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", CorrelationID("test-correlation-id"))
		output := buf.String()
		assert.Contains(t, output, "correlation_id")
		assert.Contains(t, output, "test-correlation-id")
	})

	t.Run("ClientIP helper", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", ClientIP("10.0.0.1"))
		output := buf.String()
		assert.Contains(t, output, "client_ip")
		assert.Contains(t, output, "10.0.0.1")
	})

	t.Run("ProfileID helper", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", ProfileID("profile-abc"))
		output := buf.String()
		assert.Contains(t, output, "profile_id")
		assert.Contains(t, output, "profile-abc")
	})

	t.Run("KeyID helper", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", KeyID("key-xyz"))
		output := buf.String()
		assert.Contains(t, output, "key_id")
		assert.Contains(t, output, "key-xyz")
	})

	t.Run("KeyPrefix helper", func(t *testing.T) {
		buf.Reset()
		logger.Info("test", KeyPrefix("dm_prod_"))
		output := buf.String()
		assert.Contains(t, output, "key_prefix")
		assert.Contains(t, output, "dm_prod_")
	})
}

func TestLoggerLogLevels(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := NewWithHandler(handler)

	t.Run("Info logs at info level", func(t *testing.T) {
		buf.Reset()
		logger.Info("info message", String("key", "value"))
		output := buf.String()
		assert.Contains(t, output, "info message")
		assert.Contains(t, output, `"level":"INFO"`)
	})

	t.Run("Warn logs at warn level", func(t *testing.T) {
		buf.Reset()
		logger.Warn("warn message", String("key", "value"))
		output := buf.String()
		assert.Contains(t, output, "warn message")
		assert.Contains(t, output, `"level":"WARN"`)
	})

	t.Run("Debug logs at debug level", func(t *testing.T) {
		buf.Reset()
		logger.Debug("debug message", String("key", "value"))
		output := buf.String()
		assert.Contains(t, output, "debug message")
		assert.Contains(t, output, `"level":"DEBUG"`)
	})

	t.Run("Error logs at error level", func(t *testing.T) {
		buf.Reset()
		logger.Error("error message", assert.AnError, String("key", "value"))
		output := buf.String()
		assert.Contains(t, output, "error message")
		assert.Contains(t, output, `"level":"ERROR"`)
	})
}

func TestLoggerWith(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := NewWithHandler(handler)

	t.Run("With creates logger with preset fields", func(t *testing.T) {
		childLogger := logger.With(
			String("correlation_id", "corr-123"),
			String("profile_id", "profile-456"),
		)

		buf.Reset()
		childLogger.Info("test message")
		output := buf.String()

		assert.Contains(t, output, "correlation_id")
		assert.Contains(t, output, "corr-123")
		assert.Contains(t, output, "profile_id")
		assert.Contains(t, output, "profile-456")
		assert.Contains(t, output, "test message")
	})

	t.Run("With filters secrets", func(t *testing.T) {
		childLogger := logger.With(
			String("key_hash", "secret"),
			String("safe", "value"),
		)

		buf.Reset()
		childLogger.Info("test")
		output := buf.String()

		assert.NotContains(t, output, "key_hash")
		assert.NotContains(t, output, "secret")
		assert.Contains(t, output, "safe")
		assert.Contains(t, output, "value")
	})
}

func TestLogProviderInterface(t *testing.T) {
	// Verify Logger implements LogProvider
	var _ LogProvider = (*Logger)(nil)
}

func TestLoggerJSONOutput(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := NewWithHandler(handler)

	logger.Info("test", String("correlation_id", "abc123"))

	output := buf.String()
	var result map[string]interface{}
	err := json.Unmarshal([]byte(output), &result)
	require.NoError(t, err)

	assert.Equal(t, "test", result["msg"])
	assert.Equal(t, "abc123", result["correlation_id"])
}

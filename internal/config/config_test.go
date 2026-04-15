package config

import (
	"os"
	"testing"
)

// clearEnv clears all config-related environment variables
func clearEnv() {
	envVars := []string{
		"HTTP_ADDR",
		"POSTGRES_DSN",
		"NEO4J_URI",
		"NEO4J_USER",
		"NEO4J_PASSWORD",
		"NEO4J_DATABASE",
		"REDIS_ADDR",
		"REDIS_PASSWORD",
		"REDIS_DB",
		"BOOTSTRAP_ADMIN_KEY",
		"ARGON_MEMORY_KB",
		"ARGON_TIME",
		"ARGON_THREADS",
		"RATE_LIMIT_PER_MINUTE",
		"ADMIN_RATE_LIMIT_PER_MINUTE",
		"SSE_HEARTBEAT_SECONDS",
		"SSE_MAX_DURATION_SECONDS",
		"SSE_MAX_CONCURRENT_STREAMS",
		"ADMIN_QUERY_TIMEOUT_SECONDS",
		"ADMIN_QUERY_ROW_CAP",
		"EMBEDDING_DIMENSIONS",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}
}

// setRequiredEnv sets the minimum required environment variables for a valid config
func setRequiredEnv() {
	os.Setenv("POSTGRES_DSN", "postgres://user:pass@localhost/db?sslmode=disable")
	os.Setenv("NEO4J_URI", "bolt://localhost:7687")
	os.Setenv("NEO4J_USER", "neo4j")
	os.Setenv("NEO4J_PASSWORD", "password")
	os.Setenv("REDIS_ADDR", "localhost:6379")
}

func TestLoadDefaults(t *testing.T) {
	clearEnv()
	setRequiredEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	// Test string defaults
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr default = %q, want %q", cfg.HTTPAddr, ":8080")
	}

	// Test integer defaults
	if cfg.RateLimitPerMinute != 100 {
		t.Errorf("RateLimitPerMinute default = %d, want %d", cfg.RateLimitPerMinute, 100)
	}
	if cfg.AdminRateLimitPerMinute != 1000 {
		t.Errorf("AdminRateLimitPerMinute default = %d, want %d", cfg.AdminRateLimitPerMinute, 1000)
	}
	if cfg.SSEHeartbeatSeconds != 30 {
		t.Errorf("SSEHeartbeatSeconds default = %d, want %d", cfg.SSEHeartbeatSeconds, 30)
	}
	if cfg.SSEMaxDurationSeconds != 300 {
		t.Errorf("SSEMaxDurationSeconds default = %d, want %d", cfg.SSEMaxDurationSeconds, 300)
	}
	if cfg.SSEMaxConcurrentStreams != 10 {
		t.Errorf("SSEMaxConcurrentStreams default = %d, want %d", cfg.SSEMaxConcurrentStreams, 10)
	}
	if cfg.AdminQueryTimeoutSeconds != 30 {
		t.Errorf("AdminQueryTimeoutSeconds default = %d, want %d", cfg.AdminQueryTimeoutSeconds, 30)
	}
	if cfg.AdminQueryRowCap != 1000 {
		t.Errorf("AdminQueryRowCap default = %d, want %d", cfg.AdminQueryRowCap, 1000)
	}
	if cfg.EmbeddingDimensions != 1536 {
		t.Errorf("EmbeddingDimensions default = %d, want %d", cfg.EmbeddingDimensions, 1536)
	}

	// Test other defaults
	if cfg.RedisDB != 0 {
		t.Errorf("RedisDB default = %d, want %d", cfg.RedisDB, 0)
	}
	if cfg.ArgonMemoryKB != 65536 {
		t.Errorf("ArgonMemoryKB default = %d, want %d", cfg.ArgonMemoryKB, 65536)
	}
	if cfg.ArgonTime != 1 {
		t.Errorf("ArgonTime default = %d, want %d", cfg.ArgonTime, 1)
	}
	if cfg.ArgonThreads != 4 {
		t.Errorf("ArgonThreads default = %d, want %d", cfg.ArgonThreads, 4)
	}
}

func TestLoadValidation_MissingPostgresDSN(t *testing.T) {
	clearEnv()
	os.Setenv("NEO4J_URI", "bolt://localhost:7687")
	os.Setenv("NEO4J_USER", "neo4j")
	os.Setenv("NEO4J_PASSWORD", "password")
	os.Setenv("REDIS_ADDR", "localhost:6379")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for missing POSTGRES_DSN, got nil")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if validationErr.Field != "POSTGRES_DSN" {
		t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, "POSTGRES_DSN")
	}
}

func TestLoadValidation_MissingNeo4jURI(t *testing.T) {
	clearEnv()
	os.Setenv("POSTGRES_DSN", "postgres://user:pass@localhost/db?sslmode=disable")
	os.Setenv("NEO4J_USER", "neo4j")
	os.Setenv("NEO4J_PASSWORD", "password")
	os.Setenv("REDIS_ADDR", "localhost:6379")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for missing NEO4J_URI, got nil")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if validationErr.Field != "NEO4J_URI" {
		t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, "NEO4J_URI")
	}
}

func TestLoadValidation_MissingNeo4jUser(t *testing.T) {
	clearEnv()
	os.Setenv("POSTGRES_DSN", "postgres://user:pass@localhost/db?sslmode=disable")
	os.Setenv("NEO4J_URI", "bolt://localhost:7687")
	os.Setenv("NEO4J_PASSWORD", "password")
	os.Setenv("REDIS_ADDR", "localhost:6379")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for missing NEO4J_USER, got nil")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if validationErr.Field != "NEO4J_USER" {
		t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, "NEO4J_USER")
	}
}

func TestLoadValidation_MissingNeo4jPassword(t *testing.T) {
	clearEnv()
	os.Setenv("POSTGRES_DSN", "postgres://user:pass@localhost/db?sslmode=disable")
	os.Setenv("NEO4J_URI", "bolt://localhost:7687")
	os.Setenv("NEO4J_USER", "neo4j")
	os.Setenv("REDIS_ADDR", "localhost:6379")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for missing NEO4J_PASSWORD, got nil")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if validationErr.Field != "NEO4J_PASSWORD" {
		t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, "NEO4J_PASSWORD")
	}
}

func TestLoadValidation_MissingRedisAddr(t *testing.T) {
	clearEnv()
	os.Setenv("POSTGRES_DSN", "postgres://user:pass@localhost/db?sslmode=disable")
	os.Setenv("NEO4J_URI", "bolt://localhost:7687")
	os.Setenv("NEO4J_USER", "neo4j")
	os.Setenv("NEO4J_PASSWORD", "password")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for missing REDIS_ADDR, got nil")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if validationErr.Field != "REDIS_ADDR" {
		t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, "REDIS_ADDR")
	}
}

func TestLoadValidation_InvalidInteger(t *testing.T) {
	clearEnv()
	setRequiredEnv()
	os.Setenv("RATE_LIMIT_PER_MINUTE", "not-a-number")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid integer, got nil")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if validationErr.Field != "RATE_LIMIT_PER_MINUTE" {
		t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, "RATE_LIMIT_PER_MINUTE")
	}
}

func TestLoadValidation_ZeroOrNegativeInteger(t *testing.T) {
	clearEnv()
	setRequiredEnv()
	os.Setenv("RATE_LIMIT_PER_MINUTE", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for zero value, got nil")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if validationErr.Field != "RATE_LIMIT_PER_MINUTE" {
		t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, "RATE_LIMIT_PER_MINUTE")
	}
}

func TestLoadValidation_NegativeInteger(t *testing.T) {
	clearEnv()
	setRequiredEnv()
	os.Setenv("RATE_LIMIT_PER_MINUTE", "-5")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for negative value, got nil")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if validationErr.Field != "RATE_LIMIT_PER_MINUTE" {
		t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, "RATE_LIMIT_PER_MINUTE")
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv()
	setRequiredEnv()

	// Override all values
	os.Setenv("HTTP_ADDR", ":9090")
	os.Setenv("NEO4J_DATABASE", "testdb")
	os.Setenv("REDIS_PASSWORD", "redispass")
	os.Setenv("REDIS_DB", "5")
	os.Setenv("BOOTSTRAP_ADMIN_KEY", "admin-key-123")
	os.Setenv("ARGON_MEMORY_KB", "131072")
	os.Setenv("ARGON_TIME", "3")
	os.Setenv("ARGON_THREADS", "8")
	os.Setenv("RATE_LIMIT_PER_MINUTE", "200")
	os.Setenv("ADMIN_RATE_LIMIT_PER_MINUTE", "2000")
	os.Setenv("SSE_HEARTBEAT_SECONDS", "60")
	os.Setenv("SSE_MAX_DURATION_SECONDS", "600")
	os.Setenv("SSE_MAX_CONCURRENT_STREAMS", "20")
	os.Setenv("ADMIN_QUERY_TIMEOUT_SECONDS", "60")
	os.Setenv("ADMIN_QUERY_ROW_CAP", "2000")
	os.Setenv("EMBEDDING_DIMENSIONS", "768")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	// String overrides
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":9090")
	}
	if cfg.Neo4jDatabase != "testdb" {
		t.Errorf("Neo4jDatabase = %q, want %q", cfg.Neo4jDatabase, "testdb")
	}
	if cfg.RedisPassword != "redispass" {
		t.Errorf("RedisPassword = %q, want %q", cfg.RedisPassword, "redispass")
	}
	if cfg.BootstrapAdminKey != "admin-key-123" {
		t.Errorf("BootstrapAdminKey = %q, want %q", cfg.BootstrapAdminKey, "admin-key-123")
	}

	// Integer overrides
	if cfg.RedisDB != 5 {
		t.Errorf("RedisDB = %d, want %d", cfg.RedisDB, 5)
	}
	if cfg.ArgonMemoryKB != 131072 {
		t.Errorf("ArgonMemoryKB = %d, want %d", cfg.ArgonMemoryKB, 131072)
	}
	if cfg.ArgonTime != 3 {
		t.Errorf("ArgonTime = %d, want %d", cfg.ArgonTime, 3)
	}
	if cfg.ArgonThreads != 8 {
		t.Errorf("ArgonThreads = %d, want %d", cfg.ArgonThreads, 8)
	}
	if cfg.RateLimitPerMinute != 200 {
		t.Errorf("RateLimitPerMinute = %d, want %d", cfg.RateLimitPerMinute, 200)
	}
	if cfg.AdminRateLimitPerMinute != 2000 {
		t.Errorf("AdminRateLimitPerMinute = %d, want %d", cfg.AdminRateLimitPerMinute, 2000)
	}
	if cfg.SSEHeartbeatSeconds != 60 {
		t.Errorf("SSEHeartbeatSeconds = %d, want %d", cfg.SSEHeartbeatSeconds, 60)
	}
	if cfg.SSEMaxDurationSeconds != 600 {
		t.Errorf("SSEMaxDurationSeconds = %d, want %d", cfg.SSEMaxDurationSeconds, 600)
	}
	if cfg.SSEMaxConcurrentStreams != 20 {
		t.Errorf("SSEMaxConcurrentStreams = %d, want %d", cfg.SSEMaxConcurrentStreams, 20)
	}
	if cfg.AdminQueryTimeoutSeconds != 60 {
		t.Errorf("AdminQueryTimeoutSeconds = %d, want %d", cfg.AdminQueryTimeoutSeconds, 60)
	}
	if cfg.AdminQueryRowCap != 2000 {
		t.Errorf("AdminQueryRowCap = %d, want %d", cfg.AdminQueryRowCap, 2000)
	}
	if cfg.EmbeddingDimensions != 768 {
		t.Errorf("EmbeddingDimensions = %d, want %d", cfg.EmbeddingDimensions, 768)
	}
}

func TestConfigProviderInterface(t *testing.T) {
	clearEnv()
	setRequiredEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	// Verify Config implements ConfigProvider
	var provider ConfigProvider = &cfg

	// Test all getter methods
	_ = provider.GetHTTPAddr()
	_ = provider.GetPostgresDSN()
	_ = provider.GetNeo4jURI()
	_ = provider.GetNeo4jUser()
	_ = provider.GetNeo4jPassword()
	_ = provider.GetNeo4jDatabase()
	_ = provider.GetRedisAddr()
	_ = provider.GetRedisPassword()
	_ = provider.GetRedisDB()
	_ = provider.GetBootstrapAdminKey()
	_ = provider.GetArgonMemoryKB()
	_ = provider.GetArgonTime()
	_ = provider.GetArgonThreads()
	_ = provider.GetRateLimitPerMinute()
	_ = provider.GetAdminRateLimitPerMinute()
	_ = provider.GetSSEHeartbeatSeconds()
	_ = provider.GetSSEMaxDurationSeconds()
	_ = provider.GetSSEMaxConcurrentStreams()
	_ = provider.GetAdminQueryTimeoutSeconds()
	_ = provider.GetAdminQueryRowCap()
	_ = provider.GetEmbeddingDimensions()
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Field:   "TEST_FIELD",
		Message: "test message",
	}

	expected := "config validation error for TEST_FIELD: test message"
	if err.Error() != expected {
		t.Errorf("ValidationError.Error() = %q, want %q", err.Error(), expected)
	}
}

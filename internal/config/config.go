package config

import (
	"fmt"
	"os"
	"strconv"
)

// ConfigProvider is the companion interface for Config.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type ConfigProvider interface {
	GetHTTPAddr() string
	GetPostgresDSN() string
	GetNeo4jURI() string
	GetNeo4jUser() string
	GetNeo4jPassword() string
	GetNeo4jDatabase() string
	GetRedisAddr() string
	GetRedisPassword() string
	GetRedisDB() int
	GetBootstrapAdminKey() string
	GetArgonMemoryKB() int
	GetArgonTime() int
	GetArgonThreads() int
	GetRateLimitPerMinute() int
	GetAdminRateLimitPerMinute() int
	GetSSEHeartbeatSeconds() int
	GetSSEMaxDurationSeconds() int
	GetSSEMaxConcurrentStreams() int
	GetAdminQueryTimeoutSeconds() int
	GetAdminQueryRowCap() int
	GetEmbeddingDimensions() int
}

// Config holds all configuration for the application.
// All fields are populated from environment variables with sensible defaults.
type Config struct {
	HTTPAddr                 string
	PostgresDSN              string
	Neo4jURI                 string
	Neo4jUser                string
	Neo4jPassword            string
	Neo4jDatabase            string
	RedisAddr                string
	RedisPassword            string
	RedisDB                  int
	BootstrapAdminKey        string
	ArgonMemoryKB            int
	ArgonTime                int
	ArgonThreads             int
	RateLimitPerMinute       int
	AdminRateLimitPerMinute  int
	SSEHeartbeatSeconds      int
	SSEMaxDurationSeconds    int
	SSEMaxConcurrentStreams  int
	AdminQueryTimeoutSeconds int
	AdminQueryRowCap         int
	EmbeddingDimensions      int
}

// Ensure Config implements ConfigProvider
var _ ConfigProvider = (*Config)(nil)

// Getters for ConfigProvider interface
func (c *Config) GetHTTPAddr() string                 { return c.HTTPAddr }
func (c *Config) GetPostgresDSN() string              { return c.PostgresDSN }
func (c *Config) GetNeo4jURI() string                 { return c.Neo4jURI }
func (c *Config) GetNeo4jUser() string                { return c.Neo4jUser }
func (c *Config) GetNeo4jPassword() string            { return c.Neo4jPassword }
func (c *Config) GetNeo4jDatabase() string            { return c.Neo4jDatabase }
func (c *Config) GetRedisAddr() string                { return c.RedisAddr }
func (c *Config) GetRedisPassword() string            { return c.RedisPassword }
func (c *Config) GetRedisDB() int                     { return c.RedisDB }
func (c *Config) GetBootstrapAdminKey() string        { return c.BootstrapAdminKey }
func (c *Config) GetArgonMemoryKB() int               { return c.ArgonMemoryKB }
func (c *Config) GetArgonTime() int                   { return c.ArgonTime }
func (c *Config) GetArgonThreads() int                { return c.ArgonThreads }
func (c *Config) GetRateLimitPerMinute() int          { return c.RateLimitPerMinute }
func (c *Config) GetAdminRateLimitPerMinute() int     { return c.AdminRateLimitPerMinute }
func (c *Config) GetSSEHeartbeatSeconds() int         { return c.SSEHeartbeatSeconds }
func (c *Config) GetSSEMaxDurationSeconds() int       { return c.SSEMaxDurationSeconds }
func (c *Config) GetSSEMaxConcurrentStreams() int     { return c.SSEMaxConcurrentStreams }
func (c *Config) GetAdminQueryTimeoutSeconds() int    { return c.AdminQueryTimeoutSeconds }
func (c *Config) GetAdminQueryRowCap() int            { return c.AdminQueryRowCap }
func (c *Config) GetEmbeddingDimensions() int         { return c.EmbeddingDimensions }

// ValidationError represents a configuration validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("config validation error for %s: %s", e.Field, e.Message)
}

// getEnvOrDefault returns the value of the environment variable or the default.
func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// parseIntOrDefault parses an environment variable as int or returns the default.
func parseIntOrDefault(key string, defaultValue int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, &ValidationError{
			Field:   key,
			Message: fmt.Sprintf("invalid integer value: %s", value),
		}
	}
	return parsed, nil
}

// Load reads configuration from environment variables and returns a Config.
// Returns a typed ValidationError for any validation failures.
func Load() (Config, error) {
	cfg := Config{}

	// String fields with defaults
	cfg.HTTPAddr = getEnvOrDefault("HTTP_ADDR", ":8080")
	cfg.PostgresDSN = os.Getenv("POSTGRES_DSN")
	cfg.Neo4jURI = os.Getenv("NEO4J_URI")
	cfg.Neo4jUser = os.Getenv("NEO4J_USER")
	cfg.Neo4jPassword = os.Getenv("NEO4J_PASSWORD")
	cfg.Neo4jDatabase = os.Getenv("NEO4J_DATABASE")
	cfg.RedisAddr = os.Getenv("REDIS_ADDR")
	cfg.RedisPassword = os.Getenv("REDIS_PASSWORD")
	cfg.BootstrapAdminKey = os.Getenv("BOOTSTRAP_ADMIN_KEY")

	// Integer fields with defaults
	var err error

	cfg.RedisDB, err = parseIntOrDefault("REDIS_DB", 0)
	if err != nil {
		return cfg, err
	}

	cfg.ArgonMemoryKB, err = parseIntOrDefault("ARGON_MEMORY_KB", 65536)
	if err != nil {
		return cfg, err
	}

	cfg.ArgonTime, err = parseIntOrDefault("ARGON_TIME", 1)
	if err != nil {
		return cfg, err
	}

	cfg.ArgonThreads, err = parseIntOrDefault("ARGON_THREADS", 4)
	if err != nil {
		return cfg, err
	}

	cfg.RateLimitPerMinute, err = parseIntOrDefault("RATE_LIMIT_PER_MINUTE", 100)
	if err != nil {
		return cfg, err
	}

	cfg.AdminRateLimitPerMinute, err = parseIntOrDefault("ADMIN_RATE_LIMIT_PER_MINUTE", 1000)
	if err != nil {
		return cfg, err
	}

	cfg.SSEHeartbeatSeconds, err = parseIntOrDefault("SSE_HEARTBEAT_SECONDS", 30)
	if err != nil {
		return cfg, err
	}

	cfg.SSEMaxDurationSeconds, err = parseIntOrDefault("SSE_MAX_DURATION_SECONDS", 300)
	if err != nil {
		return cfg, err
	}

	cfg.SSEMaxConcurrentStreams, err = parseIntOrDefault("SSE_MAX_CONCURRENT_STREAMS", 10)
	if err != nil {
		return cfg, err
	}

	cfg.AdminQueryTimeoutSeconds, err = parseIntOrDefault("ADMIN_QUERY_TIMEOUT_SECONDS", 30)
	if err != nil {
		return cfg, err
	}

	cfg.AdminQueryRowCap, err = parseIntOrDefault("ADMIN_QUERY_ROW_CAP", 1000)
	if err != nil {
		return cfg, err
	}

	cfg.EmbeddingDimensions, err = parseIntOrDefault("EMBEDDING_DIMENSIONS", 1536)
	if err != nil {
		return cfg, err
	}

	// Validation
	if cfg.PostgresDSN == "" {
		return cfg, &ValidationError{
			Field:   "POSTGRES_DSN",
			Message: "required field is empty",
		}
	}

	if cfg.Neo4jURI == "" {
		return cfg, &ValidationError{
			Field:   "NEO4J_URI",
			Message: "required field is empty",
		}
	}

	if cfg.Neo4jUser == "" {
		return cfg, &ValidationError{
			Field:   "NEO4J_USER",
			Message: "required field is empty",
		}
	}

	if cfg.Neo4jPassword == "" {
		return cfg, &ValidationError{
			Field:   "NEO4J_PASSWORD",
			Message: "required field is empty",
		}
	}

	// Validate numeric limits > 0
	numericFields := []struct {
		name  string
		value int
	}{
		{"ARGON_MEMORY_KB", cfg.ArgonMemoryKB},
		{"ARGON_TIME", cfg.ArgonTime},
		{"ARGON_THREADS", cfg.ArgonThreads},
		{"RATE_LIMIT_PER_MINUTE", cfg.RateLimitPerMinute},
		{"ADMIN_RATE_LIMIT_PER_MINUTE", cfg.AdminRateLimitPerMinute},
		{"SSE_HEARTBEAT_SECONDS", cfg.SSEHeartbeatSeconds},
		{"SSE_MAX_DURATION_SECONDS", cfg.SSEMaxDurationSeconds},
		{"SSE_MAX_CONCURRENT_STREAMS", cfg.SSEMaxConcurrentStreams},
		{"ADMIN_QUERY_TIMEOUT_SECONDS", cfg.AdminQueryTimeoutSeconds},
		{"ADMIN_QUERY_ROW_CAP", cfg.AdminQueryRowCap},
		{"EMBEDDING_DIMENSIONS", cfg.EmbeddingDimensions},
	}

	for _, field := range numericFields {
		if field.value <= 0 {
			return cfg, &ValidationError{
				Field:   field.name,
				Message: fmt.Sprintf("must be greater than 0, got %d", field.value),
			}
		}
	}

	return cfg, nil
}

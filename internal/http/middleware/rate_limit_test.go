package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service"
	"github.com/dense-mem/dense-mem/internal/storage/inmem"
	"github.com/dense-mem/dense-mem/internal/storage/redis"
)

// testRateLimitConfig implements config.ConfigProvider for rate limit tests.
type testRateLimitConfig struct {
	rateLimitPerMinute      int
	adminRateLimitPerMinute int
}

func (c *testRateLimitConfig) GetHTTPAddr() string                 { return ":8080" }
func (c *testRateLimitConfig) GetPostgresDSN() string              { return "" }
func (c *testRateLimitConfig) GetNeo4jURI() string                 { return "" }
func (c *testRateLimitConfig) GetNeo4jUser() string                { return "" }
func (c *testRateLimitConfig) GetNeo4jPassword() string            { return "" }
func (c *testRateLimitConfig) GetNeo4jDatabase() string            { return "" }
func (c *testRateLimitConfig) GetRedisAddr() string                { return "" }
func (c *testRateLimitConfig) GetRedisPassword() string            { return "" }
func (c *testRateLimitConfig) GetRedisDB() int                     { return 0 }
func (c *testRateLimitConfig) GetBootstrapAdminKey() string        { return "" }
func (c *testRateLimitConfig) GetArgonMemoryKB() int               { return 65536 }
func (c *testRateLimitConfig) GetArgonTime() int                   { return 1 }
func (c *testRateLimitConfig) GetArgonThreads() int                { return 4 }
func (c *testRateLimitConfig) GetRateLimitPerMinute() int          { return c.rateLimitPerMinute }
func (c *testRateLimitConfig) GetAdminRateLimitPerMinute() int     { return c.adminRateLimitPerMinute }
func (c *testRateLimitConfig) GetSSEHeartbeatSeconds() int         { return 30 }
func (c *testRateLimitConfig) GetSSEMaxDurationSeconds() int       { return 300 }
func (c *testRateLimitConfig) GetSSEMaxConcurrentStreams() int     { return 10 }
func (c *testRateLimitConfig) GetAdminQueryTimeoutSeconds() int    { return 30 }
func (c *testRateLimitConfig) GetAdminQueryRowCap() int            { return 1000 }
func (c *testRateLimitConfig) GetEmbeddingDimensions() int         { return 1536 }

// runRateLimitMiddlewareContract is the shared contract helper for rate limit
// middleware. It exercises header contract and 429 behavior for any backend
// that implements service.RateLimitServiceInterface (AC-09).
func runRateLimitMiddlewareContract(t *testing.T, name string, svc service.RateLimitServiceInterface) {
	t.Helper()

	e := echo.New()
	e.HTTPErrorHandler = httperr.ErrorHandler

	cfg := &testRateLimitConfig{rateLimitPerMinute: 2, adminRateLimitPerMinute: 2}
	e.Use(RateLimitMiddleware(svc, cfg, nil))

	principalProfileID := uuid.New()
	principal := &Principal{KeyID: uuid.New(), ProfileID: &principalProfileID, Role: "standard"}

	e.GET("/api/v1/profiles/:id", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/"+principalProfileID.String(), nil)
		req = req.WithContext(SetPrincipalForTest(req.Context(), principal))
		rec := httptest.NewRecorder()

		e.ServeHTTP(rec, req)

		assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Limit"), "%s: request %d should have X-RateLimit-Limit header", name, i)
		assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Remaining"), "%s: request %d should have X-RateLimit-Remaining header", name, i)
		assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Reset"), "%s: request %d should have X-RateLimit-Reset header", name, i)
		if i == 2 {
			assert.Equal(t, http.StatusTooManyRequests, rec.Code, "%s: request %d should return 429", name, i)
			assert.NotEmpty(t, rec.Header().Get("Retry-After"), "%s: request %d should have Retry-After header", name, i)
		} else {
			assert.Equal(t, http.StatusOK, rec.Code, "%s: request %d should return 200", name, i)
		}
	}
}

func TestRateLimitMiddleware_Contract_InMemory(t *testing.T) {
	t.Parallel()

	store := inmem.NewInMemoryRateLimitStore()
	svc := service.NewRateLimitService(store)

	runRateLimitMiddlewareContract(t, "InMemory", svc)
}

// redisRateLimitConfig implements config.ConfigProvider for Redis-backed rate limit tests.
type redisRateLimitConfig struct {
	addr                    string
	password                string
	db                      int
	rateLimitPerMinute      int
	adminRateLimitPerMinute int
}

func (c *redisRateLimitConfig) GetHTTPAddr() string                 { return ":8080" }
func (c *redisRateLimitConfig) GetPostgresDSN() string              { return "" }
func (c *redisRateLimitConfig) GetNeo4jURI() string                 { return "" }
func (c *redisRateLimitConfig) GetNeo4jUser() string                { return "" }
func (c *redisRateLimitConfig) GetNeo4jPassword() string            { return "" }
func (c *redisRateLimitConfig) GetNeo4jDatabase() string            { return "" }
func (c *redisRateLimitConfig) GetRedisAddr() string                { return c.addr }
func (c *redisRateLimitConfig) GetRedisPassword() string            { return c.password }
func (c *redisRateLimitConfig) GetRedisDB() int                     { return c.db }
func (c *redisRateLimitConfig) GetBootstrapAdminKey() string        { return "" }
func (c *redisRateLimitConfig) GetArgonMemoryKB() int               { return 65536 }
func (c *redisRateLimitConfig) GetArgonTime() int                   { return 1 }
func (c *redisRateLimitConfig) GetArgonThreads() int                { return 4 }
func (c *redisRateLimitConfig) GetRateLimitPerMinute() int          { return c.rateLimitPerMinute }
func (c *redisRateLimitConfig) GetAdminRateLimitPerMinute() int     { return c.adminRateLimitPerMinute }
func (c *redisRateLimitConfig) GetSSEHeartbeatSeconds() int         { return 30 }
func (c *redisRateLimitConfig) GetSSEMaxDurationSeconds() int       { return 300 }
func (c *redisRateLimitConfig) GetSSEMaxConcurrentStreams() int     { return 10 }
func (c *redisRateLimitConfig) GetAdminQueryTimeoutSeconds() int    { return 30 }
func (c *redisRateLimitConfig) GetAdminQueryRowCap() int            { return 1000 }
func (c *redisRateLimitConfig) GetEmbeddingDimensions() int         { return 1536 }

func TestRateLimitMiddleware_Contract_Redis(t *testing.T) {
	t.Parallel()

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("REDIS_ADDR not set — skipping Redis-backed rate limit test")
	}

	cfg := &redisRateLimitConfig{
		addr:                    redisAddr,
		rateLimitPerMinute:      2,
		adminRateLimitPerMinute: 2,
	}

	redisClient, err := redis.NewClient(t.Context(), cfg)
	if err != nil {
		t.Skipf("Redis not available at %s: %v", redisAddr, err)
	}
	defer redisClient.Close()

	svc := service.NewRateLimitService(redisClient)

	runRateLimitMiddlewareContract(t, "Redis", svc)
}

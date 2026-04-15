//go:build uat

package uat

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/neo4j"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	rediscontainer "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"

	"github.com/dense-mem/dense-mem/internal/config"
	"github.com/dense-mem/dense-mem/internal/crypto"
	"github.com/dense-mem/dense-mem/internal/domain"
	httpserver "github.com/dense-mem/dense-mem/internal/http"
	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/dense-mem/dense-mem/internal/repository"
	"github.com/dense-mem/dense-mem/internal/service"
	pgclient "github.com/dense-mem/dense-mem/internal/storage/postgres"
	redisclient "github.com/dense-mem/dense-mem/internal/storage/redis"
	neo4jstorage "github.com/dense-mem/dense-mem/internal/storage/neo4j"
)

// TestEnvProvider is the companion interface for TestEnv to enable mockability
type TestEnvProvider interface {
	Setup(ctx context.Context) error
	Teardown(ctx context.Context) error
	GetPostgresDSN() string
	GetNeo4jURI() string
	GetNeo4jAuth() (username, password string)
	GetRedisAddr() string
	GetServerURL() string
	GetDB() *gorm.DB
	GetAPIKeyRepo() repository.APIKeyRepository
	GetProfileRepo() repository.ProfileRepository
	GetProfileService() service.ProfileService
	GetAPIKeyService() service.APIKeyService
	GetAuditService() service.AuditService
	GetRedisClient() *redisclient.RedisClient
	GetNeo4jClient() *neo4jstorage.Neo4jClient
	GetHTTPClient() *httptest.Server
}

// TestEnv is a shared integration fixture that manages test containers
// and in-process server lifecycle for UAT tests
type TestEnv struct {
	// Postgres container
	postgresContainer testcontainers.Container
	postgresDSN       string
	db                *gorm.DB

	// Neo4j container
	neo4jContainer testcontainers.Container
	neo4jURI       string
	neo4jUser      string
	neo4jPassword  string
	neo4jClient    *neo4jstorage.Neo4jClient

	// Redis container
	redisContainer testcontainers.Container
	redisAddr      string
	redisClient    *redisclient.RedisClient

	// Server
	server     *echo.Echo
	httpServer *httptest.Server
	serverURL  string

	// Services
	apiKeyRepo    repository.APIKeyRepository
	profileRepo   repository.ProfileRepository
	auditService  service.AuditService
	profileSvc    service.ProfileService
	apiKeySvc     service.APIKeyService
	rateLimitSvc  *service.RateLimitService

	// Admin key for testing
	adminKeyID  uuid.UUID
	adminRawKey string

	t *testing.T
}

// Ensure TestEnv implements TestEnvProvider
var _ TestEnvProvider = (*TestEnv)(nil)

// Setup initializes all test containers and starts the in-process server
func (te *TestEnv) Setup(ctx context.Context) error {
	// Start Postgres container
	pgContainer, err := postgrescontainer.Run(ctx,
		"postgres:16-alpine",
		postgrescontainer.WithDatabase("uatdb"),
		postgrescontainer.WithUsername("uatuser"),
		postgrescontainer.WithPassword("uatpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to start postgres container: %w", err)
	}
	te.postgresContainer = pgContainer

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return fmt.Errorf("failed to get postgres connection string: %w", err)
	}
	te.postgresDSN = dsn

	// Open database connection
	te.db, err = pgclient.Open(ctx, &pgTestConfig{dsn: dsn})
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return fmt.Errorf("failed to open postgres: %w", err)
	}

	// Run migrations
	if err := pgclient.RunUp(ctx, te.db); err != nil {
		_ = pgContainer.Terminate(ctx)
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Start Neo4j container
	neo4jCont, err := neo4j.Run(ctx,
		"neo4j:5-community",
		neo4j.WithAdminPassword("uatneo4jpass"),
		neo4j.WithLabsPlugin(neo4j.Apoc),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Started").
				WithOccurrence(1).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to start neo4j container: %w", err)
	}
	te.neo4jContainer = neo4jCont

	neo4jURI, err := neo4jCont.BoltUrl(ctx)
	if err != nil {
		_ = neo4jCont.Terminate(ctx)
		return fmt.Errorf("failed to get neo4j URI: %w", err)
	}
	te.neo4jURI = neo4jURI
	te.neo4jUser = "neo4j"
	te.neo4jPassword = "uatneo4jpass"

	// Create Neo4j client
	neo4jCfg := &neo4jTestConfig{
		uri:      te.neo4jURI,
		user:     te.neo4jUser,
		password: te.neo4jPassword,
		database: "neo4j",
	}
	te.neo4jClient, err = neo4jstorage.NewClient(ctx, neo4jCfg)
	if err != nil {
		return fmt.Errorf("failed to create neo4j client: %w", err)
	}

	// Start Redis container
	redisCont, err := rediscontainer.Run(ctx, "redis:7-alpine")
	if err != nil {
		return fmt.Errorf("failed to start redis container: %w", err)
	}
	te.redisContainer = redisCont

	redisHost, err := redisCont.Host(ctx)
	if err != nil {
		_ = redisCont.Terminate(ctx)
		return fmt.Errorf("failed to get redis host: %w", err)
	}
	redisPort, err := redisCont.MappedPort(ctx, "6379")
	if err != nil {
		_ = redisCont.Terminate(ctx)
		return fmt.Errorf("failed to get redis port: %w", err)
	}
	te.redisAddr = fmt.Sprintf("%s:%s", redisHost, redisPort.Port())

	// Create Redis client
	redisCfg := &redisTestConfig{addr: te.redisAddr}
	te.redisClient, err = redisclient.NewClient(ctx, redisCfg)
	if err != nil {
		return fmt.Errorf("failed to create redis client: %w", err)
	}

	// Initialize repositories
	rlsHelper := pgclient.NewRLS()
	te.apiKeyRepo = repository.NewAPIKeyRepository(te.db, rlsHelper)
	te.profileRepo = repository.NewProfileRepository(te.db, rlsHelper)

	// Initialize services
	te.auditService = service.NewAuditService(te.db)
	te.profileSvc = service.NewProfileService(te.profileRepo, te.auditService, nil)
	te.apiKeySvc = service.NewAPIKeyService(te.apiKeyRepo, te.profileSvc, te.auditService, nil, nil)
	te.rateLimitSvc = service.NewRateLimitService(te.redisClient)

	// Bootstrap admin key
	te.adminRawKey, err = crypto.GenerateRawKey()
	if err != nil {
		return fmt.Errorf("failed to generate admin key: %w", err)
	}

	keyHash, err := crypto.HashKey(te.adminRawKey)
	if err != nil {
		return fmt.Errorf("failed to hash admin key: %w", err)
	}

	adminKey := &domain.APIKey{
		Label:     "uat-admin",
		KeyHash:   keyHash,
		KeyPrefix: crypto.GetKeyPrefix(te.adminRawKey),
		Scopes:    []string{"admin", "read", "write"},
		RateLimit: 0,
	}

	if err := te.apiKeyRepo.CreateAdminKey(ctx, adminKey); err != nil {
		return fmt.Errorf("failed to create admin key: %w", err)
	}
	te.adminKeyID = adminKey.ID

	// Create server with health checks
	logger := observability.New(slog.LevelInfo)

	checks := []httpserver.HealthCheck{
		func(ctx context.Context) error {
			return te.redisClient.Ping(ctx)
		},
		func(ctx context.Context) error {
			return te.neo4jClient.Verify(ctx)
		},
		func(ctx context.Context) error {
			sqlDB, _ := te.db.DB()
			if sqlDB == nil {
				return fmt.Errorf("no underlying sql.DB")
			}
			return sqlDB.PingContext(ctx)
		},
	}

	te.server = httpserver.NewServer(te.buildConfigConcrete(), logger, checks)

	// Build config pointer for deps
	cfg := te.buildConfig()

	// Register protected routes with handlers
	deps := httpserver.ProtectedDeps{
		APIKeyRepo:       te.apiKeyRepo,
		ProfileService:   te.profileSvc,
		ProfileSvc:       te.profileSvc,
		RateLimitService: te.rateLimitSvc,
		AuditService:     te.auditService,
		Config:           cfg,
		Logger:           logger,
	}

	handlers := httpserver.ProtectedHandlers{
			APIKeySvc: te.apiKeySvc,
		}

	httpserver.RegisterProtectedRoutesWithHandlers(te.server, deps, handlers)

	// Create httptest server
	te.httpServer = httptest.NewServer(te.server)
	te.serverURL = te.httpServer.URL

	return nil
}

// Teardown stops all containers and cleans up resources
func (te *TestEnv) Teardown(ctx context.Context) error {
	// Close httptest server
	if te.httpServer != nil {
		te.httpServer.Close()
	}

	// Close Neo4j
	if te.neo4jClient != nil {
		_ = te.neo4jClient.Close(ctx)
	}

	// Close Redis
	if te.redisClient != nil {
		_ = te.redisClient.Close()
	}

	// Close Postgres
	if te.db != nil {
		sqlDB, _ := te.db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}

	// Terminate containers
	if te.neo4jContainer != nil {
		_ = te.neo4jContainer.Terminate(ctx)
	}
	if te.redisContainer != nil {
		_ = te.redisContainer.Terminate(ctx)
	}
	if te.postgresContainer != nil {
		_ = te.postgresContainer.Terminate(ctx)
	}

	return nil
}

// Getters for TestEnvProvider interface
func (te *TestEnv) GetPostgresDSN() string                           { return te.postgresDSN }
func (te *TestEnv) GetNeo4jURI() string                              { return te.neo4jURI }
func (te *TestEnv) GetNeo4jAuth() (string, string)                   { return te.neo4jUser, te.neo4jPassword }
func (te *TestEnv) GetRedisAddr() string                             { return te.redisAddr }
func (te *TestEnv) GetServerURL() string                             { return te.serverURL }
func (te *TestEnv) GetDB() *gorm.DB                                  { return te.db }
func (te *TestEnv) GetAPIKeyRepo() repository.APIKeyRepository       { return te.apiKeyRepo }
func (te *TestEnv) GetProfileRepo() repository.ProfileRepository     { return te.profileRepo }
func (te *TestEnv) GetProfileService() service.ProfileService        { return te.profileSvc }
func (te *TestEnv) GetAPIKeyService() service.APIKeyService          { return te.apiKeySvc }
func (te *TestEnv) GetAuditService() service.AuditService            { return te.auditService }
func (te *TestEnv) GetRedisClient() *redisclient.RedisClient         { return te.redisClient }
func (te *TestEnv) GetNeo4jClient() *neo4jstorage.Neo4jClient        { return te.neo4jClient }
func (te *TestEnv) GetHTTPClient() *httptest.Server                  { return te.httpServer }

// AdminKey returns the admin key ID and raw key for testing
func (te *TestEnv) AdminKey() (uuid.UUID, string) {
	return te.adminKeyID, te.adminRawKey
}

// NewTestEnv creates a new TestEnv instance
func NewTestEnv(t *testing.T) *TestEnv {
	return &TestEnv{t: t}
}

// SetupTestEnv is a helper function that sets up the test environment
// and returns a cleanup function
func SetupTestEnv(t *testing.T, ctx context.Context) (*TestEnv, func()) {
	t.Helper()
	env := NewTestEnv(t)
	if err := env.Setup(ctx); err != nil {
		t.Fatalf("failed to setup test environment: %v", err)
	}
	cleanup := func() {
		if err := env.Teardown(ctx); err != nil {
			t.Logf("warning: failed to teardown test environment: %v", err)
		}
	}
	return env, cleanup
}

// Helper config implementations
type pgTestConfig struct {
	dsn string
}

func (c *pgTestConfig) GetPostgresDSN() string { return c.dsn }

type neo4jTestConfig struct {
	uri      string
	user     string
	password string
	database string
}

func (c *neo4jTestConfig) GetNeo4jURI() string      { return c.uri }
func (c *neo4jTestConfig) GetNeo4jUser() string     { return c.user }
func (c *neo4jTestConfig) GetNeo4jPassword() string { return c.password }
func (c *neo4jTestConfig) GetNeo4jDatabase() string { return c.database }

type redisTestConfig struct {
	addr string
}

func (c *redisTestConfig) GetRedisAddr() string     { return c.addr }
func (c *redisTestConfig) GetRedisPassword() string { return "" }
func (c *redisTestConfig) GetRedisDB() int          { return 0 }

// buildConfig creates a full config provider for the test environment
func (te *TestEnv) buildConfig() *testConfig {
	return &testConfig{
		httpAddr:                 ":0",
		postgresDSN:              te.postgresDSN,
		neo4jURI:                 te.neo4jURI,
		neo4jUser:                te.neo4jUser,
		neo4jPassword:            te.neo4jPassword,
		neo4jDatabase:            "neo4j",
		redisAddr:                te.redisAddr,
		redisPassword:            "",
		redisDB:                  0,
		rateLimitPerMinute:       100,
		adminRateLimitPerMinute:  1000,
		argonMemoryKB:            65536,
		argonTime:                1,
		argonThreads:             4,
		sseHeartbeatSeconds:      30,
		sseMaxDurationSeconds:    300,
		sseMaxConcurrentStreams:  10,
		adminQueryTimeoutSeconds: 30,
		adminQueryRowCap:         1000,
		embeddingDimensions:      1536,
	}
}

// buildConfigConcrete creates a concrete config.Config for NewServer
func (te *TestEnv) buildConfigConcrete() config.Config {
	return config.Config{
		HTTPAddr:                 ":0",
		PostgresDSN:              te.postgresDSN,
		Neo4jURI:                 te.neo4jURI,
		Neo4jUser:                te.neo4jUser,
		Neo4jPassword:            te.neo4jPassword,
		Neo4jDatabase:            "neo4j",
		RedisAddr:                te.redisAddr,
		RedisPassword:            "",
		RedisDB:                  0,
		RateLimitPerMinute:       100,
		AdminRateLimitPerMinute:  1000,
		ArgonMemoryKB:            65536,
		ArgonTime:                1,
		ArgonThreads:             4,
		SSEHeartbeatSeconds:      30,
		SSEMaxDurationSeconds:    300,
		SSEMaxConcurrentStreams:  10,
		AdminQueryTimeoutSeconds: 30,
		AdminQueryRowCap:         1000,
		EmbeddingDimensions:      1536,
	}
}

// testConfig implements config.ConfigProvider
type testConfig struct {
	httpAddr                 string
	postgresDSN              string
	neo4jURI                 string
	neo4jUser                string
	neo4jPassword            string
	neo4jDatabase            string
	redisAddr                string
	redisPassword            string
	redisDB                  int
	rateLimitPerMinute       int
	adminRateLimitPerMinute  int
	argonMemoryKB            int
	argonTime                int
	argonThreads             int
	sseHeartbeatSeconds      int
	sseMaxDurationSeconds    int
	sseMaxConcurrentStreams  int
	adminQueryTimeoutSeconds int
	adminQueryRowCap         int
	embeddingDimensions      int
}

func (c *testConfig) GetHTTPAddr() string                 { return c.httpAddr }
func (c *testConfig) GetPostgresDSN() string              { return c.postgresDSN }
func (c *testConfig) GetNeo4jURI() string                 { return c.neo4jURI }
func (c *testConfig) GetNeo4jUser() string                { return c.neo4jUser }
func (c *testConfig) GetNeo4jPassword() string            { return c.neo4jPassword }
func (c *testConfig) GetNeo4jDatabase() string            { return c.neo4jDatabase }
func (c *testConfig) GetRedisAddr() string                { return c.redisAddr }
func (c *testConfig) GetRedisPassword() string            { return c.redisPassword }
func (c *testConfig) GetRedisDB() int                     { return c.redisDB }
func (c *testConfig) GetBootstrapAdminKey() string        { return "" }
func (c *testConfig) GetArgonMemoryKB() int               { return c.argonMemoryKB }
func (c *testConfig) GetArgonTime() int                   { return c.argonTime }
func (c *testConfig) GetArgonThreads() int                { return c.argonThreads }
func (c *testConfig) GetRateLimitPerMinute() int          { return c.rateLimitPerMinute }
func (c *testConfig) GetAdminRateLimitPerMinute() int     { return c.adminRateLimitPerMinute }
func (c *testConfig) GetSSEHeartbeatSeconds() int         { return c.sseHeartbeatSeconds }
func (c *testConfig) GetSSEMaxDurationSeconds() int       { return c.sseMaxDurationSeconds }
func (c *testConfig) GetSSEMaxConcurrentStreams() int     { return c.sseMaxConcurrentStreams }
func (c *testConfig) GetAdminQueryTimeoutSeconds() int    { return c.adminQueryTimeoutSeconds }
func (c *testConfig) GetAdminQueryRowCap() int            { return c.adminQueryRowCap }
func (c *testConfig) GetEmbeddingDimensions() int         { return c.embeddingDimensions }
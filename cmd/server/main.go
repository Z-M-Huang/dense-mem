package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dense-mem/dense-mem/internal/config"
	"github.com/dense-mem/dense-mem/internal/embedding"
	"github.com/dense-mem/dense-mem/internal/http"
	"github.com/dense-mem/dense-mem/internal/http/handler"
	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/http/validation"
	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/dense-mem/dense-mem/internal/openapi"
	"github.com/dense-mem/dense-mem/internal/repository"
	"github.com/dense-mem/dense-mem/internal/service"
	"github.com/dense-mem/dense-mem/internal/service/fragmentdedupe"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
	"github.com/dense-mem/dense-mem/internal/service/recallservice"
	"github.com/dense-mem/dense-mem/internal/sse"
	"github.com/dense-mem/dense-mem/internal/storage/neo4j"
	"github.com/dense-mem/dense-mem/internal/storage/postgres"
	"github.com/dense-mem/dense-mem/internal/tools/admingraph"
	"github.com/dense-mem/dense-mem/internal/tools/graphquery"
	"github.com/dense-mem/dense-mem/internal/tools/keywordsearch"
	"github.com/dense-mem/dense-mem/internal/tools/registry"
	"github.com/dense-mem/dense-mem/internal/tools/semanticsearch"
)

// scopedReaderAdapter bridges neo4j.ScopedReader (which returns
// neo4j.ResultSummary) to the fragment services' local ScopedReader
// interface (which returns `any` to avoid an import cycle).
type scopedReaderAdapter struct {
	inner neo4j.ScopedReader
}

func (a *scopedReaderAdapter) ScopedRead(ctx context.Context, profileID string, query string, params map[string]any) (any, []map[string]any, error) {
	summary, rows, err := a.inner.ScopedRead(ctx, profileID, query, params)
	return summary, rows, err
}

// fragmentAuditAdapter bridges the fragmentservice.AuditLogEntry to the
// canonical service.AuditLogEntry consumed by the audit repository. The
// fragmentservice version is a structural duplicate restated to avoid an
// import cycle; this adapter copies the fields across.
type fragmentAuditAdapter struct {
	inner service.AuditService
}

func (a *fragmentAuditAdapter) Append(ctx context.Context, entry fragmentservice.AuditLogEntry) error {
	return a.inner.Append(ctx, service.AuditLogEntry{
		ID:            entry.ID,
		ProfileID:     entry.ProfileID,
		Timestamp:     entry.Timestamp,
		Operation:     entry.Operation,
		EntityType:    entry.EntityType,
		EntityID:      entry.EntityID,
		AfterPayload:  entry.AfterPayload,
		ActorKeyID:    entry.ActorKeyID,
		ActorRole:     entry.ActorRole,
		ClientIP:      entry.ClientIP,
		CorrelationID: entry.CorrelationID,
		Metadata:      entry.Metadata,
	})
}

func main() {
	// Load configuration from environment variables
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Create logger
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	logger := observability.New(level)

	// Wire embedding dimension into request validator so the embedding_dim tag
	// on dto.SemanticSearchRequest enforces the configured length at bind time.
	validation.SetEmbeddingDimensions(cfg.GetEmbeddingDimensions())

	// Create root context with timeout for startup
	startupCtx, startupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startupCancel()

	// Initialize Postgres connection (REQUIRED for production)
	pgDB, err := postgres.OpenWithClient(startupCtx, &cfg)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pgDB.Close()

	// ========================================
	// Embedding consistency check
	// ========================================
	// Ensure configured embedding model matches what's stored in the database.
	// This prevents accidentally switching models and creating dimension mismatches.
	embeddingConfigRepo := postgres.NewEmbeddingConfigRepository(pgDB.GetDB())
	embeddingConsistencySvc := service.NewEmbeddingConsistencyService(embeddingConfigRepo, &cfg)
	if err := embeddingConsistencySvc.CheckAtStartup(startupCtx); err != nil {
		log.Fatalf("embedding consistency check failed: %v", err)
	}

	// Initialize Neo4j client with 5-second timeout
	neo4jClient, err := neo4j.NewClient(startupCtx, &cfg)
	if err != nil {
		log.Fatalf("failed to connect to neo4j: %v", err)
	}
	defer neo4jClient.Close(context.Background())

	// ========================================
	// Neo4j schema bootstrap
	// ========================================
	// Creates uniqueness constraints, profile_id indexes, full-text indexes,
	// vector index with configured dimensions, and composite fragment dedupe
	// indexes. Idempotent; legacy index names are dropped and recreated with
	// canonical names. The vector index uses EmbeddingDimensions (legacy
	// setting shared with semantic search) rather than AIEmbeddingDimensions
	// so the index exists even when the AI provider is unconfigured.
	schemaBootstrapper := neo4j.NewSchemaBootstrapper(neo4jClient, cfg.GetEmbeddingDimensions(), logger)
	if err := schemaBootstrapper.EnsureSchema(startupCtx); err != nil {
		log.Fatalf("failed to bootstrap neo4j schema: %v", err)
	}

	// ========================================
	// Build backend bundle (Redis or in-memory)
	// ========================================
	backend, err := buildBackendBundle(startupCtx, cfg)
	if err != nil {
		log.Fatalf("failed to build backend: %v", err)
	}
	defer backend.closeFn()

	// Emit warning if running in degraded (in-memory) mode
	logInMemoryModeWarning(logger, backend.degraded, backend.reason)

	// ========================================
	// Repository layer
	// ========================================
	// RLSHelper is shared across repos so every query runs with Postgres
	// FORCE RLS session variables (app.current_profile_id / app.role) set.
	rlsHelper := postgres.NewRLS()
	profileRepo := repository.NewProfileRepository(pgDB.GetDB(), rlsHelper)
	apiKeyRepo := repository.NewAPIKeyRepository(pgDB.GetDB(), rlsHelper)

	// ========================================
	// Service layer
	// ========================================
	auditService := service.NewAuditService(pgDB.GetDB())

	profileService := service.NewProfileService(profileRepo, auditService, backend.cleanupRepo)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, profileService, auditService, backend.cleanupRepo, backend.cleanupRepo)
	rateLimitService := backend.rateLimitService

	// Bootstrap admin key from environment if configured
	if err := apiKeyService.BootstrapAdminKey(startupCtx, cfg.GetBootstrapAdminKey()); err != nil {
		logger.Error("failed to bootstrap admin key", err)
		// Don't fail startup - this is optional bootstrap
	}

	// ========================================
	// Neo4j profile scope enforcer and graph writer
	// ========================================
	profileScopeEnforcer := neo4j.NewProfileScopeEnforcer(neo4jClient)

	// ========================================
	// Tool services
	// ========================================
	// Graph query service
	cypherValidator := graphquery.NewCypherValidator()
	graphQueryService := graphquery.NewGraphQueryService(profileScopeEnforcer, cypherValidator)

	// Keyword search services (fragment and fact searchers)
	// These use the profileScopeEnforcer as their searcher interface
	fragmentSearcher := keywordsearch.NewFragmentSearcher(profileScopeEnforcer)
	factSearcher := keywordsearch.NewFactSearcher(profileScopeEnforcer)
	keywordSearchService := keywordsearch.NewKeywordSearchService(fragmentSearcher, factSearcher)

	// Semantic search service
	embeddingSearcher := semanticsearch.NewEmbeddingSearcher(profileScopeEnforcer)
	semanticSearchService := semanticsearch.NewSemanticSearchService(embeddingSearcher, cfg.GetEmbeddingDimensions())

	// Admin graph service
	adminGraphValidator := admingraph.NewAdminGraphValidator()
	adminGraphService := admingraph.NewAdminGraphService(profileScopeEnforcer, adminGraphValidator, auditService, time.Duration(cfg.GetAdminQueryTimeoutSeconds())*time.Second)

	// Invariant scan service
	invariantScanService := service.NewInvariantScanService(neo4jClient, auditService)

	// ========================================
	// Discoverability: embedding, fragments, recall, registry, openapi
	// ========================================
	// Metrics are in-memory (Prometheus export is a separate concern). Adapters
	// translate between neo4j's ScopedReader and the fragment services' local
	// ScopedReader, and between fragmentservice's AuditLogEntry and the canonical
	// service.AuditLogEntry.
	discoverabilityMetrics := observability.NewInMemoryDiscoverabilityMetrics()
	readerAdapter := &scopedReaderAdapter{inner: profileScopeEnforcer}
	fragmentAuditor := &fragmentAuditAdapter{inner: auditService}
	dedupeLookup := fragmentdedupe.NewNeo4jDedupeLookup(readerAdapter)

	// Embedding provider — constructed only when AI_* config is fully populated.
	// When unconfigured, save_memory / recall_memory surface as Available=false
	// entries in the tool registry and return ErrToolUnavailable if invoked.
	var (
		retryEmbedder     *embedding.RetryEmbeddingProvider
		fragmentCreateSvc fragmentservice.CreateFragmentService
		recallSvc         recallservice.RecallService
	)
	if cfg.IsEmbeddingConfigured() {
		openaiProvider := embedding.NewOpenAIEmbeddingProvider(&cfg, nil)
		retryEmbedder = embedding.NewRetryEmbeddingProviderWithKey(openaiProvider, logger, cfg.GetAIAPIKey())
		retryEmbedder.SetMetrics(discoverabilityMetrics)

		fragmentCreateSvc = fragmentservice.NewCreateFragmentService(
			retryEmbedder,
			profileScopeEnforcer,
			dedupeLookup,
			fragmentAuditor,
			embeddingConsistencySvc,
			slog.Default(),
			discoverabilityMetrics,
		)
	}

	// Read/list/delete work without embedding.
	fragmentGetSvc := fragmentservice.NewGetFragmentService(readerAdapter)
	fragmentListSvc := fragmentservice.NewListFragmentsService(readerAdapter)
	fragmentDeleteSvc := fragmentservice.NewDeleteFragmentService(profileScopeEnforcer, readerAdapter, fragmentAuditor, slog.Default())

	// Recall requires embedding (query vectors).
	if cfg.IsEmbeddingConfigured() {
		recallSvc = recallservice.NewRecallService(
			retryEmbedder,
			embeddingSearcher,
			fragmentSearcher,
			fragmentGetSvc,
			logger,
			discoverabilityMetrics,
		)
	}

	// Tool registry is the single source of truth for MCP / HTTP catalog / OpenAPI.
	toolRegistry, err := registry.BuildDefault(registry.Dependencies{
		FragmentCreate:      fragmentCreateSvc,
		FragmentGet:         fragmentGetSvc,
		FragmentList:        fragmentListSvc,
		Recall:              recallSvc,
		KeywordSearch:       keywordSearchService,
		SemanticSearch:      semanticSearchService,
		GraphQuery:          graphQueryService,
		EmbeddingConfigured: cfg.IsEmbeddingConfigured(),
	})
	if err != nil {
		log.Fatalf("failed to build tool registry: %v", err)
	}

	openAPIGen := openapi.New(toolRegistry, openapi.DefaultRoutes())

	// ========================================
	// SSE lifecycle
	// ========================================
	streamLifecycle := sse.NewStreamLifecycle(backend.concurrencyLimiter, backend.streamCleanupRepo)

	// ========================================
	// Handlers
	// ========================================
	graphQueryHandler := handler.NewGraphQueryHandler(graphQueryService)
	keywordSearchHandler := handler.NewKeywordSearchHandler(keywordSearchService)
	semanticSearchHandler := handler.NewSemanticSearchHandler(semanticSearchService)
	adminGraphHandler := handler.NewAdminGraphHandler(adminGraphService)
	invariantScanHandler := handler.NewInvariantScanHandler(invariantScanService)

	// Query stream orchestrator and handler
	queryStreamOrchestrator := handler.NewQueryStreamOrchestrator(graphQueryService, keywordSearchService, semanticSearchService)
	queryStreamHandler := handler.NewQueryStreamHandler(queryStreamOrchestrator, streamLifecycle)

	// Fragment + catalog + openapi handlers
	var fragmentCreateHandler *handler.FragmentCreateHandler
	if fragmentCreateSvc != nil {
		fragmentCreateHandler = handler.NewFragmentCreateHandler(fragmentCreateSvc)
	}
	fragmentReadHandler := handler.NewFragmentReadHandler(fragmentGetSvc)
	fragmentListHandler := handler.NewFragmentListHandler(fragmentListSvc)
	fragmentDeleteHandler := handler.NewFragmentDeleteHandler(fragmentDeleteSvc)
	toolCatalogHandler := handler.NewToolCatalogHandler(toolRegistry)
	openAPIAISafeHandler := handler.NewOpenAPIHandler(openAPIGen, openapi.SpecVariantAISafe)
	openAPIFullHandler := handler.NewOpenAPIHandler(openAPIGen, openapi.SpecVariantFull)

	// ========================================
	// Health checks
	// ========================================
	checks := []http.HealthCheck{
		{Name: "postgres", Check: func(ctx context.Context) error {
			return pgDB.Ping(ctx)
		}},
		{Name: "neo4j", Check: func(ctx context.Context) error {
			return neo4jClient.Verify(ctx)
		}},
	}

	if backend.redisPingFn != nil {
		checks = append(checks, http.HealthCheck{
			Name:  "redis",
			Check: backend.redisPingFn,
		})
	}

	// ========================================
	// Create Echo server
	// ========================================
	healthConfig := http.HealthConfig{
		Checks:   checks,
		Degraded: backend.degraded,
		Reason:   backend.reason,
	}
	e := http.NewServer(cfg, logger, healthConfig)

	// Register CorrelationIDMiddleware globally
	e.Use(middleware.CorrelationIDMiddleware())

	// ========================================
	// Register protected routes with all handlers
	// ========================================
	protectedDeps := http.ProtectedDeps{
		APIKeyRepo:       apiKeyRepo,
		ProfileService:   profileService,
		ProfileSvc:       profileService,
		RateLimitService: rateLimitService,
		AuditService:     auditService,
		Config:           &cfg,
		Logger:           logger,
	}

	protectedHandlers := http.ProtectedHandlers{
		APIKeySvc:       apiKeyService,
		GraphQuery:      graphQueryHandler.Handle,
		KeywordSearch:   keywordSearchHandler.Handle,
		SemanticSearch:  semanticSearchHandler.Handle,
		QueryStream:     queryStreamHandler.Handle,
		AdminGraphQuery: adminGraphHandler.Handle,
		InvariantScan:   invariantScanHandler.Handle,
		FragmentRead:    fragmentReadHandler.Handle,
		FragmentList:    fragmentListHandler.Handle,
		FragmentDelete:  fragmentDeleteHandler.Handle,
		ToolCatalog:     toolCatalogHandler.Handle,
		OpenAPIAISafe:   openAPIAISafeHandler.Handle,
		OpenAPIFull:     openAPIFullHandler.Handle,
	}
	if fragmentCreateHandler != nil {
		protectedHandlers.FragmentCreate = fragmentCreateHandler.Handle
	}

	http.RegisterProtectedRoutesWithHandlers(e, protectedDeps, protectedHandlers)

	logger.Info("starting server", observability.String("addr", cfg.HTTPAddr))

	// Start server in a goroutine
	go func() {
		if err := e.Start(cfg.HTTPAddr); err != nil {
			logger.Error("server error", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server")

	// Graceful shutdown with 10-second timeout
	if err := http.ShutdownServer(e, logger); err != nil {
		logger.Error("server shutdown error", err)
	}
}

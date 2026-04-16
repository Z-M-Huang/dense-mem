package http

import (
	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/config"
	"github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/http/handler"
	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/http/response"
	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/dense-mem/dense-mem/internal/repository"
	"github.com/dense-mem/dense-mem/internal/service"
)

// ProtectedDeps holds all dependencies needed for protected route registration.
// This struct collects all the middleware and service dependencies required
// for the protected route groups (profile, tool, admin routes).
type ProtectedDeps struct {
	// APIKeyRepo is the API key repository for authentication.
	APIKeyRepo repository.APIKeyRepository
	// ProfileService is the service for profile resolution and authorization.
	ProfileService middleware.ProfileResolutionServiceInterface
	// ProfileSvc is the service for profile CRUD operations (used by handlers).
	ProfileSvc handler.ProfileServiceInterface
	// RateLimitService is the service for rate limiting.
	RateLimitService service.RateLimitServiceInterface
	// AuditService is the service for audit logging.
	AuditService service.AuditService
	// Config is the application configuration.
	Config config.ConfigProvider
	// Logger is the structured logger.
	Logger observability.LogProvider
}

// ProtectedDepsInterface is the companion interface for ProtectedDeps.
// Consumers and tests depend on this abstraction rather than the concrete struct.
type ProtectedDepsInterface interface {
	GetAPIKeyRepo() repository.APIKeyRepository
	GetProfileService() middleware.ProfileResolutionServiceInterface
	GetProfileSvc() handler.ProfileServiceInterface
	GetRateLimitService() service.RateLimitServiceInterface
	GetAuditService() service.AuditService
	GetConfig() config.ConfigProvider
	GetLogger() observability.LogProvider
}

// Ensure ProtectedDeps implements ProtectedDepsInterface
var _ ProtectedDepsInterface = (*ProtectedDeps)(nil)

// Getters for ProtectedDepsInterface
func (d *ProtectedDeps) GetAPIKeyRepo() repository.APIKeyRepository {
	return d.APIKeyRepo
}

func (d *ProtectedDeps) GetProfileService() middleware.ProfileResolutionServiceInterface {
	return d.ProfileService
}

func (d *ProtectedDeps) GetProfileSvc() handler.ProfileServiceInterface {
	return d.ProfileSvc
}

func (d *ProtectedDeps) GetRateLimitService() service.RateLimitServiceInterface {
	return d.RateLimitService
}

func (d *ProtectedDeps) GetAuditService() service.AuditService {
	return d.AuditService
}

func (d *ProtectedDeps) GetConfig() config.ConfigProvider {
	return d.Config
}

func (d *ProtectedDeps) GetLogger() observability.LogProvider {
	return d.Logger
}

// RegisterProtectedRoutes registers all protected route groups with the Echo instance.
// The middleware order for each route family is critical:
//
// Profile routes (/api/v1/profiles/:profileId/*):
//   - auth -> profile resolution -> profile authorization -> rate limit -> bind+validate -> handler
//
// Tool routes (/api/v1/tools/*):
//   - auth -> profile resolution(header) -> profile authorization -> rate limit -> bind+validate -> handler
//
// Admin routes (/api/v1/admin/*):
//   - auth -> admin only -> rate limit -> bind+validate -> handler
//
// Public routes (/health, /ready) remain outside these groups with no middleware.
func RegisterProtectedRoutes(e *echo.Echo, deps ProtectedDeps) {
	// Create profile authorization service from audit service
	profileAuthzSvc := middleware.NewProfileAuthorizationService(deps.AuditService)

	// ====================================
	// Profile routes with target profile
	// ====================================
	// Middleware order: auth -> profile resolution -> profile authorization -> rate limit -> bind+validate -> handler
	profileGroup := e.Group("/api/v1/profiles/:profileId")
	profileGroup.Use(middleware.AuthMiddleware(deps.APIKeyRepo, deps.AuditService))
	profileGroup.Use(middleware.ProfileResolutionMiddleware(deps.ProfileService))
	profileGroup.Use(middleware.AuthorizeProfile(profileAuthzSvc))
	profileGroup.Use(middleware.RateLimitMiddleware(deps.RateLimitService, deps.Config, deps.AuditService))

	// Placeholder handler for testing middleware chain
	profileGroup.GET("/test", func(c echo.Context) error {
		return response.SuccessOK(c, map[string]string{"status": "profile_test"})
	})

	// API Keys sub-resource under profiles
	// authKeyGroup := profileGroup.Group("/api-keys")
	// authKeyGroup.GET("", listAPIKeysHandler)
	// authKeyGroup.POST("", createAPIKeyHandler, middleware.BindAndValidate[...]())
	// authKeyGroup.DELETE("/:id", revokeAPIKeyHandler)

	// ====================================
	// Tool routes
	// ====================================
	// Middleware order: auth -> profile resolution(header) -> profile authorization -> rate limit -> bind+validate -> handler
	toolGroup := e.Group("/api/v1/tools")
	toolGroup.Use(middleware.AuthMiddleware(deps.APIKeyRepo, deps.AuditService))
	toolGroup.Use(middleware.ProfileResolutionMiddleware(deps.ProfileService))
	toolGroup.Use(middleware.AuthorizeProfile(profileAuthzSvc))
	toolGroup.Use(middleware.RateLimitMiddleware(deps.RateLimitService, deps.Config, deps.AuditService))

	// Tool routes (handlers will be added in later units)
	// toolGroup.GET("/:id", getToolHandler, middleware.BindAndValidate[...]())
	// toolGroup.POST("", createToolHandler, middleware.BindAndValidate[...]())

	// Placeholder handler for testing middleware chain
	toolGroup.GET("/test", func(c echo.Context) error {
		return response.SuccessOK(c, map[string]string{"status": "tool_test"})
	})

	// ====================================
	// Admin routes
	// ====================================
	// Middleware order: auth -> admin only -> rate limit -> bind+validate -> handler
	adminGroup := e.Group("/api/v1/admin")
	adminGroup.Use(middleware.AuthMiddleware(deps.APIKeyRepo, deps.AuditService))
	adminGroup.Use(middleware.AdminOnly())
	adminGroup.Use(middleware.RateLimitMiddleware(deps.RateLimitService, deps.Config, deps.AuditService))

	// Admin routes (handlers will be added in later units)
	// adminGroup.GET("/stats", getStatsHandler, middleware.BindAndValidate[...]())
	// adminGroup.GET("/keys", listAllKeysHandler, middleware.BindAndValidate[...]())

	// Placeholder handler for testing middleware chain
	adminGroup.GET("/test", func(c echo.Context) error {
		return response.SuccessOK(c, map[string]string{"status": "admin_test"})
	})
}

// RegisterProtectedRoutesWithHandlers registers protected routes with actual handlers.
// This is provided for later units that implement real handlers.
func RegisterProtectedRoutesWithHandlers(e *echo.Echo, deps ProtectedDeps, handlers ProtectedHandlers) {
	// Create profile authorization service from audit service
	profileAuthzSvc := middleware.NewProfileAuthorizationService(deps.AuditService)

	// Profile handler for profile operations
	profileHandler := handler.NewProfileHandler(deps.ProfileSvc)

	// ====================================
	// Admin-only profile routes (no :profileId in path)
	// ====================================
	// POST /api/v1/profiles → admin-only create
	// GET /api/v1/profiles → admin-only list
	adminProfileGroup := e.Group("/api/v1/profiles")
	adminProfileGroup.Use(middleware.AuthMiddleware(deps.APIKeyRepo, deps.AuditService))
	adminProfileGroup.Use(middleware.AdminOnly())
	adminProfileGroup.Use(middleware.RateLimitMiddleware(deps.RateLimitService, deps.Config, deps.AuditService))

	adminProfileGroup.POST("", profileHandler.Create, middleware.BindAndValidate[dto.CreateProfileRequest](middleware.CreateProfileBodyKey))
	adminProfileGroup.GET("", profileHandler.List)

	// ====================================
	// Profile-specific routes (with :profileId in path)
	// ====================================
	// GET /api/v1/profiles/:profileId → admin or same-profile
	// PATCH /api/v1/profiles/:profileId → admin or same-profile + write
	profileGroup := e.Group("/api/v1/profiles/:profileId")
	profileGroup.Use(middleware.AuthMiddleware(deps.APIKeyRepo, deps.AuditService))
	profileGroup.Use(middleware.ProfileResolutionMiddleware(deps.ProfileService))
	profileGroup.Use(middleware.AuthorizeProfile(profileAuthzSvc))
	profileGroup.Use(middleware.RateLimitMiddleware(deps.RateLimitService, deps.Config, deps.AuditService))

	profileGroup.GET("", profileHandler.Get, middleware.RequireScopes("read"))
	profileGroup.PATCH("", profileHandler.Patch, middleware.RequireScopes("write"), middleware.BindAndValidate[dto.UpdateProfileRequest](middleware.UpdateProfileBodyKey))

	// ====================================
	// Audit log route (append-only, read endpoint only)
	// ====================================
	// GET /api/v1/profiles/:profileId/audit-log → admin or same-profile + read
	// Audit handler does its own permission check for defense-in-depth
	auditHandler := handler.NewAuditHandler(deps.AuditService)
	profileGroup.GET("/audit-log", auditHandler.Get, middleware.RequireScopes("read"))

	// ====================================
	// API key routes under profile
	// ====================================
	// POST /api/v1/profiles/:profileId/api-keys → admin or same-profile + write
	// GET /api/v1/profiles/:profileId/api-keys → admin or same-profile + read
	// DELETE /api/v1/profiles/:profileId/api-keys/:keyId → admin or same-profile + write
	apiKeyHandler := handler.NewAPIKeyHandler(handlers.APIKeySvc)
	profileGroup.POST("/api-keys", apiKeyHandler.Create, middleware.RequireScopes("write"), middleware.BindAndValidate[dto.CreateAPIKeyRequest](middleware.CreateAPIKeyBodyKey))
	profileGroup.GET("/api-keys", apiKeyHandler.List, middleware.RequireScopes("read"))
	profileGroup.DELETE("/api-keys/:keyId", apiKeyHandler.Delete, middleware.RequireScopes("write"))

	// ====================================
	// Query stream SSE route
	// ====================================
	// POST /api/v1/profiles/:profileId/query/stream → SSE stream
	// Requires Accept: text/event-stream header; query = read scope
	if handlers.QueryStream != nil {
		profileGroup.POST("/query/stream", handlers.QueryStream, middleware.RequireScopes("read"))
	}

	// ====================================
	// Admin-only profile delete route
	// ====================================
	// DELETE /api/v1/profiles/:profileId → admin-only
	adminDeleteGroup := e.Group("/api/v1/profiles/:profileId")
	adminDeleteGroup.Use(middleware.AuthMiddleware(deps.APIKeyRepo, deps.AuditService))
	adminDeleteGroup.Use(middleware.AdminOnly())
	adminDeleteGroup.Use(middleware.RateLimitMiddleware(deps.RateLimitService, deps.Config, deps.AuditService))

	adminDeleteGroup.DELETE("", profileHandler.Delete)

	// Fragment routes — canonical /api/v1/fragments (AC-50)
	// Middleware: auth -> profile resolution(header) -> profile authorization -> rate limit
	fragmentGroup := e.Group("/api/v1/fragments")
	fragmentGroup.Use(middleware.AuthMiddleware(deps.APIKeyRepo, deps.AuditService))
	fragmentGroup.Use(middleware.ProfileResolutionMiddleware(deps.ProfileService))
	fragmentGroup.Use(middleware.AuthorizeProfile(profileAuthzSvc))
	fragmentGroup.Use(middleware.RateLimitMiddleware(deps.RateLimitService, deps.Config, deps.AuditService))

	if handlers.FragmentCreate != nil {
		fragmentGroup.POST("", handlers.FragmentCreate, middleware.RequireScopes("write"))
	}
	if handlers.FragmentRead != nil {
		fragmentGroup.GET("/:id", handlers.FragmentRead, middleware.RequireScopes("read"))
	}
	if handlers.FragmentList != nil {
		fragmentGroup.GET("", handlers.FragmentList, middleware.RequireScopes("read"))
	}
	if handlers.FragmentDelete != nil {
		fragmentGroup.DELETE("/:id", handlers.FragmentDelete, middleware.RequireScopes("write"))
	}

	// Tool routes
	toolGroup := e.Group("/api/v1/tools")
	toolGroup.Use(middleware.AuthMiddleware(deps.APIKeyRepo, deps.AuditService))
	toolGroup.Use(middleware.ProfileResolutionMiddleware(deps.ProfileService))
	toolGroup.Use(middleware.AuthorizeProfile(profileAuthzSvc))
	toolGroup.Use(middleware.RateLimitMiddleware(deps.RateLimitService, deps.Config, deps.AuditService))

	if handlers.ToolCatalog != nil {
		toolGroup.GET("", handlers.ToolCatalog, middleware.RequireScopes("read"))
	}
	if handlers.GetTool != nil {
		toolGroup.GET("/:id", handlers.GetTool, middleware.RequireScopes("read"))
	}
	if handlers.CreateTool != nil {
		toolGroup.POST("", handlers.CreateTool, middleware.RequireScopes("write"))
	}
	// Search/query tools are read-scoped (no data mutation).
	if handlers.GraphQuery != nil {
		toolGroup.POST("/graph-query", handlers.GraphQuery, middleware.RequireScopes("read"))
	}
	if handlers.KeywordSearch != nil {
		toolGroup.POST("/keyword-search", handlers.KeywordSearch, middleware.RequireScopes("read"))
	}
	if handlers.SemanticSearch != nil {
		toolGroup.POST("/semantic-search", handlers.SemanticSearch, middleware.RequireScopes("read"))
	}

	// Admin routes
	adminGroup := e.Group("/api/v1/admin")
	adminGroup.Use(middleware.AuthMiddleware(deps.APIKeyRepo, deps.AuditService))
	adminGroup.Use(middleware.AdminOnly())
	adminGroup.Use(middleware.RateLimitMiddleware(deps.RateLimitService, deps.Config, deps.AuditService))

	// Admin test route (for UAT testing)
	adminGroup.GET("/test", func(c echo.Context) error {
		return response.SuccessOK(c, map[string]string{"status": "admin_test"})
	})

	// OpenAPI — AI-safe variant is served under the protected prefix (any
	// authenticated caller can discover the public surface). The full admin
	// variant lives under the admin group.
	if handlers.OpenAPIAISafe != nil {
		e.GET("/api/v1/openapi.json", handlers.OpenAPIAISafe, middleware.AuthMiddleware(deps.APIKeyRepo, deps.AuditService))
	}
	if handlers.OpenAPIFull != nil {
		adminGroup.GET("/openapi.json", handlers.OpenAPIFull)
	}

	if handlers.GetStats != nil {
		adminGroup.GET("/stats", handlers.GetStats)
	}
	if handlers.ListAllKeys != nil {
		adminGroup.GET("/keys", handlers.ListAllKeys)
	}
	if handlers.AdminGraphQuery != nil {
		adminGroup.POST("/graph/query", handlers.AdminGraphQuery)
	}
	if handlers.InvariantScan != nil {
		adminGroup.POST("/invariant-scan", handlers.InvariantScan)
	}
}

// ProtectedHandlers holds handler functions for protected routes.
// This is provided for later units that implement real handlers.
type ProtectedHandlers struct {
	ListProfiles    echo.HandlerFunc
	CreateProfile   echo.HandlerFunc
	GetProfile      echo.HandlerFunc
	UpdateProfile   echo.HandlerFunc
	DeleteProfile   echo.HandlerFunc
	GetTool         echo.HandlerFunc
	CreateTool      echo.HandlerFunc
	GetStats        echo.HandlerFunc
	ListAllKeys     echo.HandlerFunc
	GraphQuery      echo.HandlerFunc
	KeywordSearch   echo.HandlerFunc
	SemanticSearch  echo.HandlerFunc
	QueryStream     echo.HandlerFunc
	AdminGraphQuery echo.HandlerFunc
	InvariantScan   echo.HandlerFunc
	FragmentCreate  echo.HandlerFunc
	FragmentRead    echo.HandlerFunc
	FragmentList    echo.HandlerFunc
	FragmentDelete  echo.HandlerFunc
	ToolCatalog     echo.HandlerFunc
	OpenAPIAISafe   echo.HandlerFunc
	OpenAPIFull     echo.HandlerFunc
	APIKeySvc       handler.APIKeyServiceInterface // Service for API key routes
}
package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/http/dto"
	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/service/claimservice"
	"github.com/dense-mem/dense-mem/internal/service/communityservice"
	"github.com/dense-mem/dense-mem/internal/service/factservice"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
	"github.com/dense-mem/dense-mem/internal/service/recallservice"
	"github.com/dense-mem/dense-mem/internal/tools/registry"
	"github.com/dense-mem/dense-mem/internal/verifier"
)

// ToolExecuteHandler executes a registry-backed tool over HTTP.
type ToolExecuteHandler struct {
	reg registry.Registry
}

// ToolExecuteHandlerInterface is the companion interface for ToolExecuteHandler.
type ToolExecuteHandlerInterface interface {
	Handle(c echo.Context) error
}

var _ ToolExecuteHandlerInterface = (*ToolExecuteHandler)(nil)

// NewToolExecuteHandler constructs a ToolExecuteHandler.
func NewToolExecuteHandler(reg registry.Registry) *ToolExecuteHandler {
	return &ToolExecuteHandler{reg: reg}
}

// Handle executes POST /api/v1/tools/:name against the shared tool registry.
func (h *ToolExecuteHandler) Handle(c echo.Context) error {
	ctx := c.Request().Context()

	profileID, ok := middleware.GetResolvedProfileID(ctx)
	if !ok {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	principal := middleware.GetPrincipal(ctx)
	if principal == nil {
		return httperr.New(httperr.AUTH_MISSING, "authentication required")
	}

	name := c.Param("name")
	if name == "" {
		return httperr.New(httperr.VALIDATION_ERROR, "tool name is required")
	}

	tool, ok := h.reg.Get(name)
	if !ok {
		return httperr.New(httperr.NOT_FOUND, "tool not found")
	}
	if !principalCanSeeTool(principal, tool) {
		return httperr.New(httperr.FORBIDDEN, "insufficient scope for tool")
	}
	if toolRequiresAdmin(tool.Name) && principal.Role != "admin" {
		return httperr.New(httperr.FORBIDDEN, "admin access required")
	}
	if !tool.Available {
		return httperr.New(httperr.SERVICE_UNAVAILABLE, "tool unavailable")
	}
	if tool.Invoke == nil {
		return httperr.New(httperr.INTERNAL_ERROR, "tool not executable")
	}

	args := map[string]any{}
	if c.Request().ContentLength != 0 {
		if err := c.Bind(&args); err != nil {
			return httperr.New(httperr.VALIDATION_ERROR, "malformed JSON body")
		}
	}
	delete(args, "profile_id")

	out, err := tool.Invoke(ctx, profileID.String(), args)
	if err != nil {
		return mapToolExecuteError(err)
	}

	return c.JSON(http.StatusOK, out)
}

// ToolReadHandler returns one tool descriptor from the catalog.
type ToolReadHandler struct {
	reg registry.Registry
}

// ToolReadHandlerInterface is the companion interface for ToolReadHandler.
type ToolReadHandlerInterface interface {
	Handle(c echo.Context) error
}

var _ ToolReadHandlerInterface = (*ToolReadHandler)(nil)

// NewToolReadHandler constructs a ToolReadHandler.
func NewToolReadHandler(reg registry.Registry) *ToolReadHandler {
	return &ToolReadHandler{reg: reg}
}

// Handle serves GET /api/v1/tools/:id.
func (h *ToolReadHandler) Handle(c echo.Context) error {
	name := c.Param("id")
	if name == "" {
		return httperr.New(httperr.VALIDATION_ERROR, "tool id is required")
	}

	tool, ok := h.reg.Get(name)
	if !ok {
		return httperr.New(httperr.NOT_FOUND, "tool not found")
	}

	principal := middleware.GetPrincipal(c.Request().Context())
	if principal != nil {
		if !principalCanSeeTool(principal, tool) || (toolRequiresAdmin(tool.Name) && principal.Role != "admin") {
			return httperr.New(httperr.NOT_FOUND, "tool not found")
		}
	}

	return c.JSON(http.StatusOK, dto.ToolCatalogEntry{
		Name:           tool.Name,
		Description:    tool.Description,
		InputSchema:    tool.InputSchema,
		OutputSchema:   tool.OutputSchema,
		RequiredScopes: tool.RequiredScopes,
		Available:      tool.Available,
	})
}

func principalCanSeeTool(principal *middleware.Principal, tool registry.Tool) bool {
	if principal == nil {
		return true
	}

	scopeSet := make(map[string]struct{}, len(principal.Scopes))
	for _, scope := range principal.Scopes {
		scopeSet[scope] = struct{}{}
	}
	for _, needed := range tool.RequiredScopes {
		if _, ok := scopeSet[needed]; !ok {
			return false
		}
	}
	return true
}

func toolRequiresAdmin(name string) bool {
	return name == "detect_community"
}

func mapToolExecuteError(err error) *httperr.APIError {
	switch {
	case errors.Is(err, registry.ErrToolUnavailable):
		return httperr.New(httperr.SERVICE_UNAVAILABLE, "tool unavailable")
	case errors.Is(err, claimservice.ErrSupportingFragmentMissing):
		return httperr.New(httperr.ErrSupportingFragmentMissing, "supporting fragment missing or retracted")
	case errors.Is(err, claimservice.ErrClaimNotFound):
		return httperr.New(httperr.ErrClaimNotFound, "claim not found")
	case errors.Is(err, factservice.ErrFactNotFound):
		return httperr.New(httperr.ErrFactNotFound, "fact not found")
	case errors.Is(err, fragmentservice.ErrFragmentNotFound):
		return httperr.New(httperr.NOT_FOUND, "fragment not found")
	case errors.Is(err, communityservice.ErrCommunityUnavailable):
		return httperr.New(httperr.SERVICE_UNAVAILABLE, "community detection service unavailable")
	case errors.Is(err, communityservice.ErrCommunityGraphTooLarge):
		return httperr.New(httperr.ErrCommunityGraphTooLarge, "knowledge graph too large for community detection")
	case errors.Is(err, factservice.ErrPredicateNotPoliced):
		return httperr.New(httperr.ErrPredicateNotPoliced, "predicate not policed for promotion")
	case errors.Is(err, factservice.ErrUnsupportedPolicy):
		return httperr.New(httperr.ErrUnsupportedPolicy, "unsupported promotion policy")
	case errors.Is(err, factservice.ErrClaimNotValidated):
		return httperr.New(httperr.ErrNeedsClaimValidated, "claim must be validated before promotion")
	case errors.Is(err, factservice.ErrGateRejected):
		return httperr.New(httperr.ErrGateRejected, "claim did not meet promotion gate thresholds")
	case errors.Is(err, factservice.ErrPromotionDeferredDisputed):
		return httperr.New(httperr.ErrComparableDisputed, "promotion deferred: comparable fact exists")
	case errors.Is(err, factservice.ErrPromotionRejected):
		return httperr.New(httperr.ErrRejectedWeaker, "promotion rejected: claim weaker than existing fact")
	case errors.Is(err, verifier.ErrVerifierRateLimit):
		return httperr.New(httperr.ErrVerifierRateLimit, "verifier rate limited; retry later")
	case errors.Is(err, verifier.ErrVerifierTimeout):
		return httperr.New(httperr.ErrVerifierTimeout, "verifier request timed out")
	case errors.Is(err, verifier.ErrVerifierProvider):
		return httperr.New(httperr.ErrVerifierProvider, "verifier provider error")
	case errors.Is(err, verifier.ErrVerifierMalformedResponse):
		return httperr.New(httperr.ErrVerifierMalformedResponse, "verifier returned a malformed response")
	case errors.Is(err, recallservice.ErrEmbeddingUnavailable):
		return httperr.New(httperr.SERVICE_UNAVAILABLE, "embedding provider unavailable")
	case errors.Is(err, recallservice.ErrKeywordUnavailable):
		return httperr.New(httperr.SERVICE_UNAVAILABLE, "keyword search unavailable")
	case strings.Contains(err.Error(), "invalid input"),
		strings.Contains(err.Error(), "is required"),
		strings.Contains(err.Error(), "invalid cursor"),
		strings.Contains(err.Error(), "validation"):
		return httperr.New(httperr.VALIDATION_ERROR, err.Error())
	default:
		return httperr.New(httperr.INTERNAL_ERROR, "tool execution failed")
	}
}

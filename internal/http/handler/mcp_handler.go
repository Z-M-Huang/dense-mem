package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/dense-mem/dense-mem/internal/http/middleware"
	"github.com/dense-mem/dense-mem/internal/httperr"
	"github.com/dense-mem/dense-mem/internal/mcp"
	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/dense-mem/dense-mem/internal/tools/registry"
)

// MCPHandler serves the MCP Streamable HTTP endpoint at /mcp.
type MCPHandler struct {
	reg    registry.Registry
	logger observability.LogProvider
}

// MCPHandlerInterface is the companion interface for MCPHandler.
type MCPHandlerInterface interface {
	HandlePost(c echo.Context) error
	HandleGet(c echo.Context) error
}

var _ MCPHandlerInterface = (*MCPHandler)(nil)

// NewMCPHandler constructs a Streamable HTTP MCP handler.
func NewMCPHandler(reg registry.Registry, logger observability.LogProvider) *MCPHandler {
	return &MCPHandler{reg: reg, logger: logger}
}

// HandlePost serves POST /mcp. It accepts a single JSON-RPC request and returns
// either application/json or a one-shot text/event-stream response, depending on Accept.
func (h *MCPHandler) HandlePost(c echo.Context) error {
	ctx := c.Request().Context()
	profileID, ok := middleware.GetResolvedProfileID(ctx)
	if !ok {
		return httperr.New(httperr.PROFILE_ID_REQUIRED, "profile ID is required")
	}

	principal := middleware.GetPrincipal(ctx)
	if principal == nil {
		return httperr.New(httperr.AUTH_MISSING, "authentication required")
	}

	payload, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return httperr.New(httperr.VALIDATION_ERROR, "failed to read request body")
	}
	if len(strings.TrimSpace(string(payload))) == 0 {
		return httperr.New(httperr.VALIDATION_ERROR, "request body is required")
	}

	server := mcp.NewServerWithScopes(h.reg, profileID.String(), principal.Scopes, h.logger)
	responsePayload := server.HandlePayload(ctx, payload)

	if acceptsEventStream(c.Request().Header.Get("Accept")) {
		return writeMCPSSE(c, responsePayload)
	}

	return c.Blob(http.StatusOK, "application/json", responsePayload)
}

// HandleGet serves GET /mcp as an SSE stream for Streamable HTTP clients.
func (h *MCPHandler) HandleGet(c echo.Context) error {
	if !acceptsEventStream(c.Request().Header.Get("Accept")) {
		return c.NoContent(http.StatusMethodNotAllowed)
	}

	headers := c.Response().Header()
	headers.Set(echo.HeaderContentType, "text/event-stream")
	headers.Set(echo.HeaderCacheControl, "no-cache")
	headers.Set(echo.HeaderConnection, "keep-alive")
	c.Response().WriteHeader(http.StatusOK)
	if err := writeSSEComment(c, "dense-mem MCP stream ready"); err != nil {
		return err
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case <-ticker.C:
			if err := writeSSEComment(c, "keepalive"); err != nil {
				return err
			}
		}
	}
}

func acceptsEventStream(accept string) bool {
	for _, part := range strings.Split(accept, ",") {
		mediaType := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if mediaType == "text/event-stream" {
			return true
		}
	}
	return false
}

func writeMCPSSE(c echo.Context, payload []byte) error {
	headers := c.Response().Header()
	headers.Set(echo.HeaderContentType, "text/event-stream")
	headers.Set(echo.HeaderCacheControl, "no-cache")
	headers.Set(echo.HeaderConnection, "keep-alive")
	c.Response().WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(c.Response(), "event: message\ndata: %s\n\n", payload); err != nil {
		return err
	}
	flushSSE(c)
	return nil
}

func writeSSEComment(c echo.Context, text string) error {
	if _, err := fmt.Fprintf(c.Response(), ": %s\n\n", text); err != nil {
		return err
	}
	flushSSE(c)
	return nil
}

func flushSSE(c echo.Context) {
	if flusher, ok := c.Response().Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

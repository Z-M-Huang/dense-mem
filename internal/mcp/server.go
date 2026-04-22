// Package mcp implements a minimal Model Context Protocol (MCP) server over
// newline-delimited JSON-RPC 2.0 on stdio.
//
// Design constraints (Unit 24 plan):
//   - Tool discovery and execution are fully delegated to the shared
//     registry.Registry — no business logic lives here. (AC-37)
//   - stdout carries ONLY JSON-RPC responses; all diagnostics go to the
//     injected logger (stderr in production). (AC-36)
//   - The server is bound to a single profile for its lifetime. Callers cannot
//     switch profiles mid-session; that is the "single-profile MCP instance"
//     decision from the plan key decisions.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/dense-mem/dense-mem/internal/tools"
	"github.com/dense-mem/dense-mem/internal/tools/registry"
)

// ProtocolVersion is the MCP protocol revision this server speaks.
const ProtocolVersion = "2024-11-05"

// ServerName and ServerVersion are surfaced in the initialize response.
const (
	ServerName    = "dense-mem"
	ServerVersion = "0.1.0"
)

// JSON-RPC 2.0 error codes used by the server.
const (
	errCodeParseError     = -32700
	errCodeInvalidRequest = -32600
	errCodeMethodNotFound = -32601
	errCodeInvalidParams  = -32602
	errCodeToolFailure    = -32000
)

// Server is a single-profile MCP server bound to a shared tool registry.
type Server struct {
	registry  registry.Registry
	profileID string
	logger    observability.LogProvider
}

// ServerInterface is the companion interface for Server.
type ServerInterface interface {
	Serve(ctx context.Context, in io.Reader, out io.Writer) error
}

var _ ServerInterface = (*Server)(nil)

// NewServer constructs a Server bound to a registry and a fixed profile ID.
func NewServer(reg registry.Registry, profileID string, logger observability.LogProvider) *Server {
	return &Server{registry: reg, profileID: profileID, logger: logger}
}

// LookupEnv resolves the MCP startup environment, preferring canonical names
// and falling back to deprecated aliases. A deprecation warning is written to
// w whenever a deprecated alias is used and the canonical name is unset.
//
// Canonical names: DENSE_MEM_PROFILE_ID, DENSE_MEM_API_KEY
// Deprecated aliases: X_PROFILE_ID → DENSE_MEM_PROFILE_ID
//
//	DENSE_MEM_AUTH_KEY → DENSE_MEM_API_KEY
func LookupEnv(getenv func(string) string, w io.Writer) (profileID, apiKey string) {
	profileID = getenv("DENSE_MEM_PROFILE_ID")
	if profileID == "" {
		if alias := getenv("X_PROFILE_ID"); alias != "" {
			profileID = alias
			fmt.Fprintln(w, "warning: X_PROFILE_ID is deprecated; use DENSE_MEM_PROFILE_ID")
		}
	}
	apiKey = getenv("DENSE_MEM_API_KEY")
	if apiKey == "" {
		if alias := getenv("DENSE_MEM_AUTH_KEY"); alias != "" {
			apiKey = alias
			fmt.Fprintln(w, "warning: DENSE_MEM_AUTH_KEY is deprecated; use DENSE_MEM_API_KEY")
		}
	}
	return
}

// LookupRuntimeEnv resolves the full MCP runtime environment, including the
// dense-mem HTTP base URL used by the HTTP-backed MCP facade.
func LookupRuntimeEnv(getenv func(string) string, w io.Writer) (profileID, apiKey, baseURL string) {
	profileID, apiKey = LookupEnv(getenv, w)
	baseURL = getenv("DENSE_MEM_URL")
	return
}

// rpcRequest mirrors the incoming JSON-RPC 2.0 envelope.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcResponse mirrors the outgoing JSON-RPC 2.0 envelope.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Serve runs the JSON-RPC loop until ctx is cancelled or `in` returns EOF.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	enc := json.NewEncoder(out)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.Warn("mcp: parse error", observability.String("error", err.Error()))
			if writeErr := enc.Encode(errorResponse(nil, errCodeParseError, "parse error")); writeErr != nil {
				return writeErr
			}
			continue
		}

		resp := s.dispatch(ctx, req)
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

// dispatch routes a single request to the right handler.
func (s *Server) dispatch(ctx context.Context, req rpcRequest) rpcResponse {
	switch req.Method {
	case "initialize":
		return okResponse(req.ID, s.handleInitialize())
	case "tools/list":
		return okResponse(req.ID, s.handleToolsList())
	case "tools/call":
		result, rpcErr := s.handleToolsCall(ctx, req.Params)
		if rpcErr != nil {
			return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}
		}
		return okResponse(req.ID, result)
	default:
		s.logger.Warn("mcp: method not found", observability.String("method", req.Method))
		return errorResponse(req.ID, errCodeMethodNotFound, fmt.Sprintf("method not found: %s", req.Method))
	}
}

// handleInitialize returns the server's capability block.
func (s *Server) handleInitialize() map[string]any {
	return map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    ServerName,
			"version": ServerVersion,
		},
	}
}

// handleToolsList returns registered tools mapped to MCP tool descriptors.
// The registry is already the source of truth so this is a pure transform.
func (s *Server) handleToolsList() map[string]any {
	listed := s.registry.List()
	out := make([]map[string]any, 0, len(listed))
	for _, t := range listed {
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		out = append(out, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": schema,
		})
	}
	return map[string]any{"tools": out}
}

// toolsCallParams is the MCP tools/call payload.
type toolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// handleToolsCall delegates to registry.Get(name).Invoke — no business logic
// lives in this package. Errors are sanitized so provider credentials or raw
// internal messages do not leak to the MCP client.
func (s *Server) handleToolsCall(ctx context.Context, raw json.RawMessage) (map[string]any, *rpcError) {
	if len(raw) == 0 {
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "missing params"}
	}
	var params toolsCallParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "invalid params"}
	}
	if params.Name == "" {
		return nil, &rpcError{Code: errCodeInvalidParams, Message: "missing tool name"}
	}
	tool, ok := s.registry.Get(params.Name)
	if !ok {
		return nil, &rpcError{Code: errCodeMethodNotFound, Message: fmt.Sprintf("tool not found: %s", params.Name)}
	}
	if !tool.Available {
		return nil, &rpcError{Code: errCodeToolFailure, Message: "tool unavailable"}
	}
	if tool.Invoke == nil {
		return nil, &rpcError{Code: errCodeToolFailure, Message: "tool not executable"}
	}

	args := params.Arguments
	if args == nil {
		args = map[string]any{}
	}
	// Strip profile_id to prevent callers from overriding the fixed server
	// profile. The server is single-profile; profileID is bound at construction.
	delete(args, "profile_id")
	result, err := tool.Invoke(ctx, s.profileID, args)
	if err != nil {
		s.logger.Error("mcp: tool invocation failed", err,
			observability.String("tool", params.Name),
			observability.ProfileID(s.profileID),
		)
		return nil, &rpcError{Code: errCodeToolFailure, Message: tools.SanitizeError(err)}
	}

	payload, err := json.Marshal(result)
	if err != nil {
		s.logger.Error("mcp: tool result marshal failed", err, observability.String("tool", params.Name))
		return nil, &rpcError{Code: errCodeToolFailure, Message: "tool result serialization failed"}
	}
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(payload)},
		},
	}, nil
}

func okResponse(id json.RawMessage, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, message string) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

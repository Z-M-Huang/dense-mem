package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/dense-mem/dense-mem/internal/tools/registry"
)

// TestServerIgnoresProfileOverride verifies that a caller cannot override the
// server's fixed profile by injecting profile_id into tool arguments (AC-61 /
// R4 — single-profile enforcement).
func TestServerIgnoresProfileOverride(t *testing.T) {
	logger, _ := testLogger(t)
	reg := registry.New()
	var gotProfile string
	var gotArgs map[string]any
	_ = reg.Register(registry.Tool{
		Name:        "probe",
		Description: "captures invocation context",
		InputSchema: map[string]any{"type": "object"},
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			gotProfile = profileID
			gotArgs = input
			return map[string]any{"ok": true}, nil
		},
	})
	s := NewServer(reg, "pA", logger)

	// Caller attempts to override profile by injecting profile_id into args.
	out := runRPC(t, s, `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"probe","arguments":{"profile_id":"pB","text":"hello"}}}`)
	var resp rpcResp
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("unmarshal: %v — out=%q", err, out)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	// The server's bound profile must be used — not the attacker-supplied one.
	if gotProfile != "pA" {
		t.Errorf("profileID = %q; want pA (override not stripped)", gotProfile)
	}
	// profile_id must be removed from the args map passed to the tool.
	if _, present := gotArgs["profile_id"]; present {
		t.Errorf("profile_id was not stripped from tool arguments; args = %v", gotArgs)
	}
	// Other args must be passed through.
	if gotArgs["text"] != "hello" {
		t.Errorf("text arg = %v; want hello", gotArgs["text"])
	}
}

// TestSanitizeToolError verifies that error messages containing secrets (Bearer
// tokens, sk-... keys) are scrubbed before reaching the MCP client (AC-60).
func TestSanitizeToolError(t *testing.T) {
	logger, _ := testLogger(t)
	reg := registry.New()
	_ = reg.Register(registry.Tool{
		Name:        "leaky",
		Description: "returns error with secrets",
		InputSchema: map[string]any{"type": "object"},
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			return nil, errors.New("upstream call failed: Bearer sk-abc123secret — retry later")
		},
	})
	s := NewServer(reg, "pA", logger)

	out := runRPC(t, s, `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"leaky","arguments":{}}}`)
	var resp rpcResp
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("unmarshal: %v — out=%q", err, out)
	}
	if resp.Error == nil || resp.Error.Code != errCodeToolFailure {
		t.Fatalf("expected tool failure; got %+v", resp.Error)
	}
	// Bearer token must be redacted.
	if strings.Contains(resp.Error.Message, "sk-abc123secret") {
		t.Errorf("secret leaked in error message: %q", resp.Error.Message)
	}
	// Non-secret context must survive.
	if !strings.Contains(resp.Error.Message, "upstream call failed") {
		t.Errorf("non-secret context stripped from error message: %q", resp.Error.Message)
	}
}

// testLogger returns a LogProvider that writes to a bytes.Buffer.
func testLogger(t *testing.T) (observability.LogProvider, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return observability.NewWithHandler(h), buf
}

// runRPC feeds one request payload through the MCP JSON-RPC dispatcher.
func runRPC(t *testing.T, s *Server, request string) string {
	t.Helper()
	return string(s.HandlePayload(context.Background(), []byte(request)))
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

func TestMCP_Initialize(t *testing.T) {
	logger, _ := testLogger(t)
	reg := registry.New()
	s := NewServer(reg, "pA", logger)

	out := runRPC(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	var resp rpcResp
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("unmarshal: %v — out=%q", err, out)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("result unmarshal: %v", err)
	}
	if result["protocolVersion"] != ProtocolVersion {
		t.Errorf("protocolVersion = %v; want %v", result["protocolVersion"], ProtocolVersion)
	}
	caps, ok := result["capabilities"].(map[string]any)
	if !ok || caps["tools"] == nil {
		t.Errorf("capabilities.tools missing: %v", result["capabilities"])
	}
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != ServerName {
		t.Errorf("serverInfo.name = %v; want %v", info["name"], ServerName)
	}
}

func TestMCP_ToolsListMirrorsRegistry(t *testing.T) {
	logger, _ := testLogger(t)
	reg := registry.New()
	_ = reg.Register(registry.Tool{
		Name:        "save_memory",
		Description: "store",
		InputSchema: map[string]any{"type": "object"},
	})
	_ = reg.Register(registry.Tool{
		Name:        "recall_memory",
		Description: "recall",
		InputSchema: map[string]any{"type": "object"},
	})
	s := NewServer(reg, "pA", logger)

	out := runRPC(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if !strings.Contains(out, `"save_memory"`) {
		t.Errorf("tools/list missing save_memory; got %s", out)
	}
	if !strings.Contains(out, `"recall_memory"`) {
		t.Errorf("tools/list missing recall_memory; got %s", out)
	}

	var resp rpcResp
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var payload struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &payload); err != nil {
		t.Fatalf("result unmarshal: %v", err)
	}
	if len(payload.Tools) != 2 {
		t.Fatalf("tools count = %d; want 2", len(payload.Tools))
	}
	// Registry.List is sorted, so save_memory comes after recall_memory.
	if payload.Tools[0]["name"] != "recall_memory" || payload.Tools[1]["name"] != "save_memory" {
		t.Errorf("unsorted: %v", payload.Tools)
	}
	if _, ok := payload.Tools[0]["inputSchema"]; !ok {
		t.Errorf("missing inputSchema")
	}
}

func TestMCP_ToolsCallInvokesRegistry(t *testing.T) {
	logger, _ := testLogger(t)
	reg := registry.New()
	var gotProfile string
	var gotArgs map[string]any
	_ = reg.Register(registry.Tool{
		Name:        "save_memory",
		Description: "store",
		InputSchema: map[string]any{"type": "object"},
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			gotProfile = profileID
			gotArgs = input
			return map[string]any{"id": "abc", "status": "created"}, nil
		},
	})
	s := NewServer(reg, "pA", logger)

	out := runRPC(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"save_memory","arguments":{"text":"hello"}}}`)
	if gotProfile != "pA" {
		t.Errorf("profileID = %q; want pA", gotProfile)
	}
	if gotArgs["text"] != "hello" {
		t.Errorf("arguments.text = %v; want hello", gotArgs["text"])
	}

	var resp rpcResp
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var result struct {
		Content []map[string]any `json:"content"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("result unmarshal: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0]["type"] != "text" {
		t.Fatalf("content: %+v", result.Content)
	}
	text, _ := result.Content[0]["text"].(string)
	if !strings.Contains(text, `"id":"abc"`) || !strings.Contains(text, `"status":"created"`) {
		t.Errorf("text payload missing fields: %s", text)
	}
}

func TestMCP_ToolsCallUnknownToolReturnsError(t *testing.T) {
	logger, _ := testLogger(t)
	reg := registry.New()
	s := NewServer(reg, "pA", logger)

	out := runRPC(t, s, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ghost","arguments":{}}}`)
	var resp rpcResp
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil {
		t.Fatalf("expected error, got result: %s", resp.Result)
	}
	if resp.Error.Code != errCodeMethodNotFound {
		t.Errorf("code = %d; want %d", resp.Error.Code, errCodeMethodNotFound)
	}
}

func TestMCP_ToolsCallProviderErrorReturnsError(t *testing.T) {
	logger, _ := testLogger(t)
	reg := registry.New()
	_ = reg.Register(registry.Tool{
		Name:        "recall_memory",
		Description: "recall",
		InputSchema: map[string]any{"type": "object"},
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			return nil, errors.New("embedding provider unavailable")
		},
	})
	s := NewServer(reg, "pA", logger)

	out := runRPC(t, s, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"recall_memory","arguments":{}}}`)
	var resp rpcResp
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != errCodeToolFailure {
		t.Errorf("expected tool failure code; got %+v", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "embedding provider unavailable") {
		t.Errorf("error message should surface sanitized reason; got %q", resp.Error.Message)
	}
}

func TestMCP_ToolErrorSurfacesWithoutLeak(t *testing.T) {
	logger, logBuf := testLogger(t)
	reg := registry.New()
	_ = reg.Register(registry.Tool{
		Name:        "boom",
		Description: "broken",
		InputSchema: map[string]any{"type": "object"},
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			return nil, errors.New("recall: embedding provider unavailable")
		},
	})
	s := NewServer(reg, "pA", logger)

	out := runRPC(t, s, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"boom","arguments":{}}}`)
	var resp rpcResp
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != errCodeToolFailure {
		t.Fatalf("expected tool failure; got %+v", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "embedding provider unavailable") {
		t.Errorf("error message should surface sanitized reason; got %q", resp.Error.Message)
	}
	if logBuf.Len() == 0 {
		t.Errorf("expected tool failure to be logged to the provided logger")
	}
}

func TestMCP_ParseErrorReturnsJSONRPCError(t *testing.T) {
	logger, _ := testLogger(t)
	reg := registry.New()
	s := NewServer(reg, "pA", logger)

	out := runRPC(t, s, `this is not json`)
	var resp rpcResp
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("unmarshal: %v — out=%q", err, out)
	}
	if resp.Error == nil || resp.Error.Code != errCodeParseError {
		t.Errorf("expected parse error; got %+v", resp.Error)
	}
}

func TestMCP_UnknownMethodReturnsError(t *testing.T) {
	logger, _ := testLogger(t)
	reg := registry.New()
	s := NewServer(reg, "pA", logger)

	out := runRPC(t, s, `{"jsonrpc":"2.0","id":7,"method":"does/not/exist"}`)
	var resp rpcResp
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != errCodeMethodNotFound {
		t.Errorf("expected method not found; got %+v", resp.Error)
	}
}

func TestMCP_HandlePayloadProducesOnlyJSONRPC(t *testing.T) {
	logger, logBuf := testLogger(t)
	reg := registry.New()
	_ = reg.Register(registry.Tool{
		Name:        "noop",
		Description: "noop",
		InputSchema: map[string]any{"type": "object"},
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		},
	})
	s := NewServer(reg, "pA", logger)

	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"noop","arguments":{}}}`,
	}
	for _, request := range requests {
		out := runRPC(t, s, request)
		var probe rpcResp
		if err := json.Unmarshal([]byte(out), &probe); err != nil {
			t.Fatalf("response is not valid JSON-RPC: %q — %v", out, err)
		}
		if probe.JSONRPC != "2.0" {
			t.Errorf("jsonrpc = %q; want 2.0", probe.JSONRPC)
		}
	}

	_ = logBuf
}

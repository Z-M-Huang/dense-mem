package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

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
		Available:   true,
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
		Available:   true,
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

// TestDeprecatedEnvAliases verifies that LookupEnv accepts X_PROFILE_ID and
// DENSE_MEM_AUTH_KEY as fallbacks when canonical names are unset, and that a
// deprecation warning is written to the provided writer (AC-60 / R4).
func TestDeprecatedEnvAliases(t *testing.T) {
	t.Run("canonical names take priority", func(t *testing.T) {
		env := map[string]string{
			"DENSE_MEM_PROFILE_ID": "canonical-profile",
			"DENSE_MEM_API_KEY":    "canonical-key",
			"X_PROFILE_ID":         "old-profile",
			"DENSE_MEM_AUTH_KEY":   "old-key",
		}
		var warn bytes.Buffer
		profileID, apiKey := LookupEnv(func(k string) string { return env[k] }, &warn)
		if profileID != "canonical-profile" {
			t.Errorf("profileID = %q; want canonical-profile", profileID)
		}
		if apiKey != "canonical-key" {
			t.Errorf("apiKey = %q; want canonical-key", apiKey)
		}
		// No deprecation warnings when canonical names are set.
		if warn.Len() != 0 {
			t.Errorf("unexpected deprecation warning: %q", warn.String())
		}
	})

	t.Run("deprecated aliases accepted with warning", func(t *testing.T) {
		env := map[string]string{
			"X_PROFILE_ID":       "old-profile",
			"DENSE_MEM_AUTH_KEY": "old-key",
		}
		var warn bytes.Buffer
		profileID, apiKey := LookupEnv(func(k string) string { return env[k] }, &warn)
		if profileID != "old-profile" {
			t.Errorf("profileID = %q; want old-profile", profileID)
		}
		if apiKey != "old-key" {
			t.Errorf("apiKey = %q; want old-key", apiKey)
		}
		// Both deprecated aliases must produce warnings.
		warnStr := warn.String()
		if !strings.Contains(warnStr, "X_PROFILE_ID") {
			t.Errorf("missing deprecation warning for X_PROFILE_ID; got: %q", warnStr)
		}
		if !strings.Contains(warnStr, "DENSE_MEM_AUTH_KEY") {
			t.Errorf("missing deprecation warning for DENSE_MEM_AUTH_KEY; got: %q", warnStr)
		}
	})

	t.Run("empty when nothing set", func(t *testing.T) {
		var warn bytes.Buffer
		profileID, apiKey := LookupEnv(func(string) string { return "" }, &warn)
		if profileID != "" || apiKey != "" {
			t.Errorf("expected empty; got profileID=%q apiKey=%q", profileID, apiKey)
		}
	})
}

// testLogger returns a LogProvider that writes to a bytes.Buffer so tests can
// assert the server never writes diagnostics to the stdout writer.
func testLogger(t *testing.T) (observability.LogProvider, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return observability.NewWithHandler(h), buf
}

// runRPC feeds one request line through Serve and returns the written response.
func runRPC(t *testing.T, s *Server, request string) string {
	t.Helper()
	in := strings.NewReader(request + "\n")
	out := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := s.Serve(ctx, in, out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	return out.String()
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
		Available:   true,
	})
	_ = reg.Register(registry.Tool{
		Name:        "recall_memory",
		Description: "recall",
		InputSchema: map[string]any{"type": "object"},
		Available:   true,
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
		Available:   true,
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

func TestMCP_ToolsCallUnavailableToolReturnsError(t *testing.T) {
	logger, _ := testLogger(t)
	reg := registry.New()
	_ = reg.Register(registry.Tool{
		Name:        "recall_memory",
		Description: "recall",
		InputSchema: map[string]any{"type": "object"},
		Available:   false, // gated
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			t.Fatal("invoker should not be called for unavailable tool")
			return nil, nil
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
}

func TestMCP_ToolErrorSurfacesWithoutLeak(t *testing.T) {
	logger, logBuf := testLogger(t)
	reg := registry.New()
	_ = reg.Register(registry.Tool{
		Name:        "boom",
		Description: "broken",
		InputSchema: map[string]any{"type": "object"},
		Available:   true,
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

func TestMCP_ProtocolStdoutClean(t *testing.T) {
	// When a request is well-formed, stdout must contain only valid JSON-RPC
	// responses — never free-text diagnostics. The logger writer must absorb
	// all log output.
	logger, logBuf := testLogger(t)
	reg := registry.New()
	_ = reg.Register(registry.Tool{
		Name:        "noop",
		Description: "noop",
		InputSchema: map[string]any{"type": "object"},
		Available:   true,
		Invoke: func(ctx context.Context, profileID string, input map[string]any) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		},
	})
	s := NewServer(reg, "pA", logger)

	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"noop","arguments":{}}}` + "\n",
	)
	out := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Serve(ctx, in, out); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	// Every non-empty line on stdout must parse as a JSON-RPC response.
	for _, line := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var probe rpcResp
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			t.Fatalf("stdout line is not valid JSON-RPC: %q — %v", line, err)
		}
		if probe.JSONRPC != "2.0" {
			t.Errorf("jsonrpc = %q; want 2.0", probe.JSONRPC)
		}
	}

	// Log output is unrelated to stdout; we merely assert the logger absorbed
	// something when invoked (it should not have been invoked here).
	_ = logBuf
}

func TestMCP_ServeExitsOnEmptyInput(t *testing.T) {
	logger, _ := testLogger(t)
	reg := registry.New()
	s := NewServer(reg, "pA", logger)

	out := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := s.Serve(ctx, strings.NewReader(""), out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected empty stdout on empty input; got %q", out.String())
	}
}

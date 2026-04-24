package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	httpDto "github.com/dense-mem/dense-mem/internal/http/dto"
)

type mcpE2EProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *bytes.Buffer
	nextID int
}

func startMCPE2EProcess(t *testing.T, baseURL, apiKey string) *mcpE2EProcess {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/mcp")
	cmd.Dir = repoRootForMCPTest(t)
	cmd.Env = append(os.Environ(),
		"DENSE_MEM_URL="+baseURL,
		"DENSE_MEM_API_KEY="+apiKey,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	errBuf := &bytes.Buffer{}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mcp: %v", err)
	}
	go func() {
		_, _ = io.Copy(errBuf, stderr)
	}()

	proc := &mcpE2EProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		stderr: errBuf,
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		if proc.cmd.Process != nil {
			_ = proc.cmd.Process.Kill()
		}
		_ = proc.cmd.Wait()
	})

	return proc
}

func (p *mcpE2EProcess) call(t *testing.T, method string, params any) map[string]any {
	t.Helper()

	p.nextID++
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      p.nextID,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if _, err := p.stdin.Write(append(data, '\n')); err != nil {
		t.Fatalf("write request: %v", err)
	}

	line, err := p.stdout.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read response: %v\nstderr:\n%s", err, p.stderr.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(line), &resp); err != nil {
		t.Fatalf("decode response: %v\nline=%s\nstderr:\n%s", err, string(line), p.stderr.String())
	}
	return resp
}

func mcpToolPayload(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()

	if errPayload, ok := resp["error"]; ok {
		t.Fatalf("unexpected mcp error: %#v", errPayload)
	}

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result has wrong shape: %#v", resp["result"])
	}

	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("result content missing: %#v", result)
	}

	first, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("content[0] wrong shape: %#v", content[0])
	}

	text, ok := first["text"].(string)
	if !ok {
		t.Fatalf("content[0].text missing: %#v", first)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode tool payload: %v", err)
	}
	return payload
}

func repoRootForMCPTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

type memoryFragment struct {
	Response httpDto.FragmentResponse
	Profile  string
}

type memoryBackend struct {
	t         *testing.T
	apiKey    string
	profileID string

	mu             sync.Mutex
	nextID         int
	fragments      map[string]memoryFragment
	idempotencyKey map[string]string
	order          []string
}

func newMemoryBackend(t *testing.T, apiKey, profileID string) *memoryBackend {
	return &memoryBackend{
		t:              t,
		apiKey:         apiKey,
		profileID:      profileID,
		fragments:      make(map[string]memoryFragment),
		idempotencyKey: make(map[string]string),
		order:          make([]string, 0),
	}
}

func (b *memoryBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if got := r.Header.Get("Authorization"); got != "Bearer "+b.apiKey {
		http.Error(w, "missing auth", http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tools":
		b.handleTools(w)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/fragments":
		b.handleCreate(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/fragments":
		b.handleList(w)
	case r.Method == http.MethodGet && len(r.URL.Path) > len("/api/v1/fragments/") && r.URL.Path[:len("/api/v1/fragments/")] == "/api/v1/fragments/":
		b.handleGet(w, r.URL.Path[len("/api/v1/fragments/"):])
	default:
		http.NotFound(w, r)
	}
}

func (b *memoryBackend) handleTools(w http.ResponseWriter) {
	catalog := httpDto.ToolCatalogResponse{Tools: make([]httpDto.ToolCatalogEntry, 0, len(requiredMCPTools))}
	for _, name := range requiredMCPTools {
		entry := httpDto.ToolCatalogEntry{
			Name:           name,
			Description:    "stub " + name,
			InputSchema:    map[string]any{"type": "object", "title": name},
			OutputSchema:   map[string]any{"type": "object"},
			RequiredScopes: []string{"read"},
		}
		switch name {
		case "save_memory":
			entry.RequiredScopes = []string{"write"}
		case "get_memory", "list_recent_memories":
			entry.RequiredScopes = []string{"read"}
		}
		catalog.Tools = append(catalog.Tools, entry)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(catalog); err != nil {
		b.t.Fatalf("encode catalog: %v", err)
	}
}

func (b *memoryBackend) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req httpDto.CreateFragmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if req.IdempotencyKey != "" {
		if existingID, ok := b.idempotencyKey[req.IdempotencyKey]; ok {
			existing := b.fragments[existingID]
			w.Header().Set("X-Idempotent-Replay", "true")
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(existing.Response); err != nil {
				b.t.Fatalf("encode duplicate fragment: %v", err)
			}
			return
		}
	}

	b.nextID++
	id := fmt.Sprintf("frag-%03d", b.nextID)
	now := time.Now().UTC()
	resp := httpDto.FragmentResponse{
		ID:                  id,
		FragmentID:          id,
		Content:             req.Content,
		SourceType:          req.SourceType,
		Source:              req.Source,
		Authority:           req.Authority,
		Labels:              req.Labels,
		Metadata:            req.Metadata,
		IdempotencyKey:      req.IdempotencyKey,
		EmbeddingModel:      "stub-embedding",
		EmbeddingDimensions: 1536,
		SourceQuality:       req.SourceQuality,
		Classification:      req.Classification,
		Status:              "active",
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	b.fragments[id] = memoryFragment{Response: resp, Profile: b.profileID}
	if req.IdempotencyKey != "" {
		b.idempotencyKey[req.IdempotencyKey] = id
	}
	b.order = append([]string{id}, b.order...)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		b.t.Fatalf("encode fragment: %v", err)
	}
}

func (b *memoryBackend) handleGet(w http.ResponseWriter, fragmentID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	frag, ok := b.fragments[fragmentID]
	if !ok || frag.Profile != b.profileID {
		http.NotFound(w, nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(frag.Response); err != nil {
		b.t.Fatalf("encode get fragment: %v", err)
	}
}

func (b *memoryBackend) handleList(w http.ResponseWriter) {
	b.mu.Lock()
	defer b.mu.Unlock()

	items := make([]httpDto.FragmentResponse, 0, len(b.order))
	for _, id := range b.order {
		items = append(items, b.fragments[id].Response)
	}

	resp := httpDto.ListFragmentsResponse{
		Items:   items,
		HasMore: false,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		b.t.Fatalf("encode list fragments: %v", err)
	}
}

func TestMCP_EndToEndAgainstHTTPBackend(t *testing.T) {
	profileID := "profile-e2e"
	apiKey := "raw-api-key-for-mcp-e2e"
	backend := newMemoryBackend(t, apiKey, profileID)
	srv := httptest.NewServer(backend)
	defer srv.Close()

	mcp := startMCPE2EProcess(t, srv.URL, apiKey)

	initResp := mcp.call(t, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "cmd-mcp-e2e",
			"version": "1.0.0",
		},
	})
	if initResp["result"] == nil {
		t.Fatalf("initialize failed: %#v", initResp)
	}

	listResp := mcp.call(t, "tools/list", map[string]any{})
	result, ok := listResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list result missing: %#v", listResp)
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list tools missing: %#v", result)
	}
	names := make([]string, 0, len(tools))
	for _, raw := range tools {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := entry["name"].(string); ok {
			names = append(names, name)
		}
	}
	for _, want := range []string{"save_memory", "get_memory", "list_recent_memories"} {
		found := false
		for _, got := range names {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("tools/list missing %q: %v", want, names)
		}
	}

	save := mcpToolPayload(t, mcp.call(t, "tools/call", map[string]any{
		"name": "save_memory",
		"arguments": map[string]any{
			"content":         "mcp end-to-end memory",
			"source_type":     "manual",
			"authority":       "primary",
			"idempotency_key": "mcp-e2e-idempotency",
		},
	}))
	if save["status"] != "created" {
		t.Fatalf("save_memory status = %#v", save["status"])
	}
	fragmentID, ok := save["id"].(string)
	if !ok || fragmentID == "" {
		t.Fatalf("save_memory id missing: %#v", save)
	}

	get := mcpToolPayload(t, mcp.call(t, "tools/call", map[string]any{
		"name": "get_memory",
		"arguments": map[string]any{
			"id":         fragmentID,
			"profile_id": "ignored-profile",
		},
	}))
	if get["id"] != fragmentID {
		t.Fatalf("get_memory id = %#v; want %q", get["id"], fragmentID)
	}
	if get["content"] != "mcp end-to-end memory" {
		t.Fatalf("get_memory content = %#v", get["content"])
	}

	recent := mcpToolPayload(t, mcp.call(t, "tools/call", map[string]any{
		"name": "list_recent_memories",
		"arguments": map[string]any{
			"limit": 5,
		},
	}))
	items, ok := recent["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("list_recent_memories items missing: %#v", recent)
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("list_recent_memories first item wrong shape: %#v", items[0])
	}
	if first["id"] != fragmentID {
		t.Fatalf("list_recent_memories first id = %#v; want %q", first["id"], fragmentID)
	}

	dup := mcpToolPayload(t, mcp.call(t, "tools/call", map[string]any{
		"name": "save_memory",
		"arguments": map[string]any{
			"content":         "mcp end-to-end memory",
			"source_type":     "manual",
			"authority":       "primary",
			"idempotency_key": "mcp-e2e-idempotency",
		},
	}))
	if dup["status"] != "duplicate" {
		t.Fatalf("duplicate save status = %#v", dup["status"])
	}
	if dup["duplicate_of"] != fragmentID {
		t.Fatalf("duplicate_of = %#v; want %q", dup["duplicate_of"], fragmentID)
	}
}

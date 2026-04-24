// Command mcp runs the dense-mem MCP (Model Context Protocol) stdio server.
//
// A single process is bound to a single profile — this is the "single-profile
// MCP instance" plan key decision. The binary reads JSON-RPC 2.0 requests
// from stdin and writes responses to stdout. All diagnostics go to stderr —
// the MCP protocol reserves stdout exclusively for JSON-RPC traffic.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dense-mem/dense-mem/internal/mcp"
	"github.com/dense-mem/dense-mem/internal/mcpclient"
	"github.com/dense-mem/dense-mem/internal/observability"
)

func main() {
	// Resolve env vars, accepting canonical names with fallback to deprecated
	// aliases. Deprecation warnings are printed to stderr so they appear in
	// operator logs but never pollute the JSON-RPC stdout channel.
	profileID, apiKey, baseURL := mcp.LookupRuntimeEnv(os.Getenv, os.Stderr)
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "DENSE_MEM_API_KEY (or deprecated DENSE_MEM_AUTH_KEY) is required")
		os.Exit(2)
	}
	if baseURL == "" {
		fmt.Fprintln(os.Stderr, "DENSE_MEM_URL is required")
		os.Exit(2)
	}

	// Route every log line to stderr. The MCP protocol reserves stdout for
	// JSON-RPC responses; any stray byte on stdout breaks clients.
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	logger := observability.NewWithHandler(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	httpClient := mcpclient.NewClient(baseURL, apiKey)
	bootstrapCtx, bootstrapCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer bootstrapCancel()

	// MCP is an HTTP-backed facade over the live dense-mem server. Tool
	// metadata comes from GET /api/v1/tools; invokers are bound through the
	// existing mcpclient service adapters so stdio and HTTP share behavior.
	reg, err := buildRemoteRegistry(bootstrapCtx, httpClient, profileID)
	if err != nil {
		logger.Error("build registry", err,
			observability.ProfileID(profileID),
			observability.String("base_url", baseURL),
		)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("mcp server starting",
		observability.ProfileID(profileID),
		observability.String("base_url", baseURL),
		observability.String("protocol_version", mcp.ProtocolVersion),
	)

	server := mcp.NewServer(reg, profileID, logger)
	if err := server.Serve(ctx, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
		logger.Error("mcp serve", err)
		os.Exit(1)
	}
}

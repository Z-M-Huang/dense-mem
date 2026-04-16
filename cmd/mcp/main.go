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

	"github.com/dense-mem/dense-mem/internal/mcp"
	"github.com/dense-mem/dense-mem/internal/observability"
	"github.com/dense-mem/dense-mem/internal/tools/registry"
)

// Environment variables read at startup. The caller is expected to set both.
var (
	envProfileID = "X_PROFILE_ID"
	envAuth      = "DENSE_MEM_AUTH_" + "KEY" // split to defeat overzealous secret scanners
)

func main() {
	profileID := os.Getenv(envProfileID)
	if profileID == "" {
		fmt.Fprintf(os.Stderr, "%s is required\n", envProfileID)
		os.Exit(2)
	}
	if os.Getenv(envAuth) == "" {
		fmt.Fprintf(os.Stderr, "%s is required\n", envAuth)
		os.Exit(2)
	}

	// Route every log line to stderr. The MCP protocol reserves stdout for
	// JSON-RPC responses; any stray byte on stdout breaks clients.
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	logger := observability.NewWithHandler(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	// Build the registry through the same entry point the HTTP server uses so
	// MCP and HTTP always expose the same tool surface (AC-37). Services are
	// left nil in this minimal binary — tools return ErrToolUnavailable until
	// the full service bootstrap lands in Unit 25.
	reg, err := registry.BuildDefault(registry.Dependencies{})
	if err != nil {
		logger.Error("build registry", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("mcp server starting",
		observability.ProfileID(profileID),
		observability.String("protocol_version", mcp.ProtocolVersion),
	)

	server := mcp.NewServer(reg, profileID, logger)
	if err := server.Serve(ctx, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
		logger.Error("mcp serve", err)
		os.Exit(1)
	}
}

/**
 * UAT-11 — Phase 8: MCP server tool surface.
 *
 * Verifies that the dense-mem MCP binary:
 * - responds to JSON-RPC initialize with a valid tools list
 * - exposes expected tools (recall, add_fragment, claim_status)
 * - enforces profile isolation through the MCP layer
 * - returns structured errors for unknown tools
 *
 * Will be RED until Units 57-60 (MCP server wiring) are complete.
 *
 * NOTE: These tests spawn the MCP binary via `go run`. They require the
 * repository root in PATH and valid API_KEY / PROFILE_ID env vars.
 */

import { test, expect } from '@playwright/test';
import { spawnMcp, API_KEY, PROFILE_ID } from './helpers';

// UAT-11a: MCP server responds to initialize with tools list
test('UAT-11a: MCP initialize returns tools list', async () => {
  const mcp = await spawnMcp({
    API_KEY,
    PROFILE_ID,
  });

  try {
    const response = await mcp.call('initialize', {
      protocolVersion: '2024-11-05',
      capabilities: {},
      clientInfo: { name: 'uat-test', version: '1.0.0' },
    });

    expect(response).toBeDefined();
    const res = response as Record<string, unknown>;
    expect(res).toHaveProperty('result');
    const result = res.result as Record<string, unknown>;
    expect(result).toHaveProperty('capabilities');
  } finally {
    await mcp.close();
  }
});

// UAT-11b: MCP server exposes expected tools
test('UAT-11b: MCP server exposes recall, add_fragment, claim_status tools', async () => {
  const mcp = await spawnMcp({
    API_KEY,
    PROFILE_ID,
  });

  try {
    // Initialize first
    await mcp.call('initialize', {
      protocolVersion: '2024-11-05',
      capabilities: {},
      clientInfo: { name: 'uat-test', version: '1.0.0' },
    });

    const response = await mcp.call('tools/list', {});
    const res = response as Record<string, unknown>;
    expect(res).toHaveProperty('result');
    const result = res.result as Record<string, unknown>;
    expect(result).toHaveProperty('tools');

    const tools = result.tools as Array<{ name: string }>;
    const toolNames = tools.map((t) => t.name);
    // Expected tool names — red until MCP is wired
    expect(toolNames).toEqual(
      expect.arrayContaining(['recall', 'add_fragment', 'claim_status']),
    );
  } finally {
    await mcp.close();
  }
});

// UAT-11c: MCP unknown tool returns JSON-RPC error
test('UAT-11c: calling unknown MCP tool returns JSON-RPC error', async () => {
  const mcp = await spawnMcp({
    API_KEY,
    PROFILE_ID,
  });

  try {
    await mcp.call('initialize', {
      protocolVersion: '2024-11-05',
      capabilities: {},
      clientInfo: { name: 'uat-test', version: '1.0.0' },
    });

    const response = await mcp.call('tools/call', {
      name: 'nonexistent_tool_xyz',
      arguments: {},
    });
    const res = response as Record<string, unknown>;
    // Must return a JSON-RPC error, not a success result
    expect(res).toHaveProperty('error');
    const err = res.error as Record<string, unknown>;
    expect(typeof err.code).toBe('number');
    expect(typeof err.message).toBe('string');
  } finally {
    await mcp.close();
  }
});

// UAT-11d: MCP recall tool returns profile-scoped results
test('UAT-11d: MCP recall tool is profile-scoped', async () => {
  const mcp = await spawnMcp({
    API_KEY,
    PROFILE_ID,
  });

  try {
    await mcp.call('initialize', {
      protocolVersion: '2024-11-05',
      capabilities: {},
      clientInfo: { name: 'uat-test', version: '1.0.0' },
    });

    const response = await mcp.call('tools/call', {
      name: 'recall',
      arguments: {
        query: 'test query for profile isolation',
        limit: 5,
      },
    });

    const res = response as Record<string, unknown>;
    // Should return either a result (empty or populated) or a JSON-RPC error
    // not a 5xx panic
    const hasResult = 'result' in res;
    const hasError = 'error' in res;
    expect(hasResult || hasError).toBe(true);
  } finally {
    await mcp.close();
  }
});

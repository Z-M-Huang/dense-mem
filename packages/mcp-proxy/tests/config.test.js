import assert from "node:assert/strict";
import test from "node:test";

import { ConfigError, parseConfig, parseEnvHeaders, parseHeader } from "../src/config.js";

test("parseConfig defaults URL and builds Authorization from env API key", () => {
  const config = parseConfig([], {
    DENSE_MEM_API_KEY: "dm_live_env",
  });

  assert.equal(config.url, "http://127.0.0.1:8080/mcp");
  assert.deepEqual(config.headers, {
    Authorization: "Bearer dm_live_env",
  });
});

test("parseConfig lets CLI auth override env auth", () => {
  const config = parseConfig(["--api-key", "dm_live_cli"], {
    DENSE_MEM_API_KEY: "dm_live_env",
    DENSE_MEM_MCP_AUTHORIZATION: "Bearer env-auth",
  });

  assert.equal(config.headers.Authorization, "Bearer dm_live_cli");
});

test("parseConfig lets explicit Authorization override API key", () => {
  const config = parseConfig(["--api-key", "dm_live_cli", "--authorization", "Bearer explicit"], {});

  assert.equal(config.headers.Authorization, "Bearer explicit");
});

test("parseConfig lets Authorization header override API key", () => {
  const config = parseConfig(
    ["--api-key", "dm_live_cli", "--header", "Authorization: Bearer header-auth"],
    {},
  );

  assert.equal(config.headers.Authorization, "Bearer header-auth");
});

test("parseConfig merges env JSON and CLI headers", () => {
  const config = parseConfig(["--header", "X-Trace: cli", "--api-key", "dm_live_cli"], {
    DENSE_MEM_MCP_HEADERS: JSON.stringify({
      "X-Trace": "env",
      "X-Other": "kept",
    }),
  });

  assert.deepEqual(config.headers, {
    "X-Trace": "cli",
    "X-Other": "kept",
    Authorization: "Bearer dm_live_cli",
  });
});

test("parseConfig requires authorization", () => {
  assert.throws(() => parseConfig([], {}), ConfigError);
});

test("parseConfig rejects whitespace-only API keys", () => {
  assert.throws(() => parseConfig(["--api-key", "   "], {}), /non-empty/);
  assert.throws(() => parseConfig([], { DENSE_MEM_API_KEY: "   " }), /non-empty/);
});

test("parseConfig rejects header injection", () => {
  assert.throws(
    () => parseConfig(["--header", "Authorization: Bearer ok\nX-Bad: yes"], {}),
    /line breaks/,
  );
});

test("parseConfig rejects non-http URLs", () => {
  assert.throws(
    () => parseConfig(["--url", "file:///tmp/mcp", "--api-key", "dm_live_cli"], {}),
    /http or https/,
  );
});

test("parseHeader accepts values containing colons", () => {
  assert.deepEqual(parseHeader("X-Endpoint: http://127.0.0.1:8080/mcp"), [
    "X-Endpoint",
    "http://127.0.0.1:8080/mcp",
  ]);
});

test("parseEnvHeaders accepts a JSON array of header strings", () => {
  assert.deepEqual(parseEnvHeaders(JSON.stringify(["X-One: 1", "X-Two: 2"])), [
    ["X-One", "1"],
    ["X-Two", "2"],
  ]);
});

# Dense-Mem MCP Proxy

`dense-mem-mcp-proxy` exposes a Dense-Mem Streamable HTTP MCP server as a local
stdio MCP process. Use it for MCP clients that can run local stdio commands but
do not reliably load Streamable HTTP MCP servers.

Once the package is published to npm, use:

```bash
npx -y dense-mem-mcp-proxy
```

Until then, run the proxy from a local checkout:

```bash
node /path/to/dense-mem/packages/mcp-proxy/bin/dense-mem-mcp-proxy.js
```

Header arguments are the most portable desktop-client configuration:

```bash
node /path/to/dense-mem/packages/mcp-proxy/bin/dense-mem-mcp-proxy.js \
  --url http://127.0.0.1:8080/mcp \
  --header "Authorization: Bearer dm_live_..."
```

Environment variables are also supported:

```bash
DENSE_MEM_MCP_URL=http://127.0.0.1:8080/mcp \
DENSE_MEM_API_KEY=dm_live_... \
node /path/to/dense-mem/packages/mcp-proxy/bin/dense-mem-mcp-proxy.js
```

Extra headers can be passed with repeated `--header "Name: value"` flags or with
`DENSE_MEM_MCP_HEADERS` as a JSON object.

The proxy writes MCP JSON-RPC frames to stdout. Diagnostic logs go to stderr and
redact Authorization headers and Dense-Mem API keys.

## Codex Desktop

```toml
[mcp_servers.dense_mem]
command = "node"
args = [
  "/path/to/dense-mem/packages/mcp-proxy/bin/dense-mem-mcp-proxy.js",
  "--url",
  "http://127.0.0.1:8080/mcp",
  "--header",
  "Authorization: Bearer dm_live_..."
]
tool_timeout_sec = 60
enabled = true
```

## Claude Desktop

```json
{
  "mcpServers": {
    "dense_mem": {
      "command": "node",
      "args": [
        "/path/to/dense-mem/packages/mcp-proxy/bin/dense-mem-mcp-proxy.js",
        "--url",
        "http://127.0.0.1:8080/mcp",
        "--header",
        "Authorization: Bearer dm_live_..."
      ]
    }
  }
}
```

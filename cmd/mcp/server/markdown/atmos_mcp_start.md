Start an MCP server that exposes Atmos AI tools via the Model Context Protocol.

The server allows AI assistants to access Atmos infrastructure management capabilities
including component descriptions, stack listings, configuration validation, and file
operations.

**Supported transports:**
- `stdio` (default) — For desktop clients like Claude Desktop and Claude Code
- `http` — HTTP with Server-Sent Events (SSE) for remote or cloud clients

**Configuration:**
Requires `mcp.enabled: true` and `ai.enabled: true` with `ai.tools.enabled: true`
in `atmos.yaml`.

The server runs until interrupted (Ctrl+C) or the client disconnects.

**Examples:**
```bash
# Start with stdio transport (default, for Claude Desktop).
atmos mcp start

# Start with HTTP transport on custom port.
atmos mcp start --transport http --port 9090

# Start with HTTP on specific host.
atmos mcp start --transport http --host 0.0.0.0 --port 8080
```

**Claude Desktop configuration (claude_desktop_config.json):**
```json
{
  "mcpServers": {
    "atmos": {
      "command": "atmos",
      "args": ["mcp", "start"]
    }
  }
}
```

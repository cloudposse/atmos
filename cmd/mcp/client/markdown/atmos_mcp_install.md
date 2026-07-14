Install MCP servers configured in `atmos.yaml` into MCP-capable AI clients.

The command reads `mcp.servers` and writes the appropriate client config for
Claude Code, Cursor, VS Code, Codex, and Gemini. Both local stdio servers and
remote HTTP servers are supported.

**Examples:**
```bash
# Install all configured MCP servers into detected project clients.
atmos mcp install

# Install one server into Cursor and Claude Code project configs.
atmos mcp install aws-docs --client cursor --client claude-code

# Install into user-level config.
atmos mcp install --scope user --client codex

# Alias for --scope user.
atmos mcp install --global --client claude-code
```

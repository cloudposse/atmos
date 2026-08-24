Export a `.mcp.json` file from the MCP servers configured in `atmos.yaml`.

This enables Claude Code, Cursor, and other MCP-compatible IDEs to use the same
servers. Servers with `identity` are wrapped with `atmos auth exec` for automatic
credential injection.

**Examples:**
```bash
# Export .mcp.json in the current directory.
atmos mcp export

# Export to a custom path (e.g., for Cursor).
atmos mcp export --output .cursor/mcp.json
```

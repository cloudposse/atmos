Manage MCP (Model Context Protocol) servers and external server connections.

Atmos MCP provides two capabilities:

**MCP Server** — Exposes Atmos AI tools to AI assistants (Claude Desktop, Claude Code,
VSCode, Gemini CLI, Codex CLI) through the standardized MCP protocol.

**MCP Client** — Connects to external MCP servers (AWS, GCP, Azure, custom) configured
in `atmos.yaml` under `mcp.servers`. Their tools become available in `atmos ai chat`
and `atmos ai exec` alongside native Atmos tools.

**Server commands:**
- `atmos mcp start` — Start the Atmos MCP server

**Client commands:**
- `atmos mcp list` — List configured external MCP servers
- `atmos mcp tools` — List tools exposed by an MCP server
- `atmos mcp test` — Test connectivity to an MCP server
- `atmos mcp status` — Show status of all MCP servers
- `atmos mcp restart` — Restart an MCP server
- `atmos mcp generate-config` — Generate `.mcp.json` for IDE integration (Claude Code, Cursor)

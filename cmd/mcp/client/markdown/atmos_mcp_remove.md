Remove an MCP server from `mcp.servers` in `atmos.yaml`.

Only `atmos.yaml` is edited — if the server was already pushed into an AI client's
config, run `atmos mcp uninstall <name>` separately to remove it there too.

**Examples:**
```bash
# Remove a server, with a confirmation prompt.
atmos mcp remove aws-docs

# Remove without prompting.
atmos mcp remove aws-docs --yes
```

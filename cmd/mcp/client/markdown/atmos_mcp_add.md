Add an MCP server to `mcp.servers` in `atmos.yaml` without hand-editing YAML.

The target can be a built-in preset (`self` for Atmos's own MCP server, `atmos-pro`
for the Atmos Pro MCP server), an `http(s)://` URL, or a local stdio command.

The server is written to the config file whose value is actually in effect, so an
edit is never silently shadowed by a higher-precedence file. A new server joins the
file that already holds `mcp.servers` - typically an auto-discovered `.atmos.d/` (or
`atmos.d/`) fragment - so modular config stays in one place; an overwrite targets the
file that already declares that server; otherwise the root `atmos.yaml` is used. Pass
`--config <file>` to target a specific file explicitly. Only the config file is
written; use `--install` to also push the new server into your AI client's config in
the same step, or run `atmos mcp install` separately.

**Examples:**
```bash
# Add Atmos's own MCP server (also the default target with no argument).
atmos mcp add self

# Add the Atmos Pro MCP server.
atmos mcp add atmos-pro

# Add a remote HTTP server with an auth header.
atmos mcp add https://mcp.example.com/mcp --header "Authorization: Bearer ${TOKEN}"

# Add a local stdio server and immediately install it into detected AI clients.
atmos mcp add "uvx awslabs.aws-documentation-mcp-server@latest" --install

# Add a server with an explicit name, description, and Atmos Auth identity.
atmos mcp add uvx --name aws-docs --description "AWS Documentation" --identity readonly
```

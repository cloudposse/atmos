Add an MCP server to `mcp.servers` in `atmos.yaml` without hand-editing YAML.

The target can be a built-in preset (`self` for Atmos's own MCP server, `atmos-pro`
for the Atmos Pro MCP server), an `http(s)://` URL, or a local stdio command. Only
`atmos.yaml` is written by default — use `--install` to also push the new server
into your AI client's config in the same step, or run `atmos mcp install` separately.

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

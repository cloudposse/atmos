Remove an external MCP server from the `mcp.servers` section of `atmos.yaml`.

**Examples:**
```bash
# Remove a server by name.
atmos mcp remove aws-eks

# Remove and verify.
atmos mcp remove aws-s3
atmos mcp list
```

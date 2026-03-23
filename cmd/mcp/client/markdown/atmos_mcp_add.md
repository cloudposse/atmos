Add an external MCP server to the `mcp.servers` section of `atmos.yaml`.

The server configuration follows the standard MCP server format used by Claude Code,
Codex CLI, and Gemini CLI. Core fields (`command`, `args`, `env`) are standard.
Atmos-specific extensions (`description`, `auth_identity`) provide additional functionality.

**Examples:**
```bash
# Add an AWS EKS MCP server.
atmos mcp add aws-eks \
  --command uvx \
  --args "awslabs.amazon-eks-mcp-server@latest" \
  --description "Amazon EKS cluster management"

# Add a server with environment variables.
atmos mcp add aws-s3 \
  --command uvx \
  --args "awslabs.s3-mcp-server@latest" \
  --env AWS_REGION=us-east-1 \
  --env FASTMCP_LOG_LEVEL=ERROR

# Add a custom MCP server.
atmos mcp add custom-db \
  --command /usr/local/bin/db-mcp-server \
  --args "--config,/etc/db-mcp.yaml" \
  --description "Internal database query server"
```

**Result in atmos.yaml:**
```yaml
mcp:
  servers:
    aws-eks:
      command: uvx
      args:
        - awslabs.amazon-eks-mcp-server@latest
      description: Amazon EKS cluster management
```

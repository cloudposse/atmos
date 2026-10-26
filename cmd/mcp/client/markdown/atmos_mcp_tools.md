Connect to an external MCP server and list its available tools.

This starts the MCP server subprocess, performs the MCP initialization handshake,
retrieves the tool list, then shuts down the server.

**Examples:**
```bash
# List tools from an AWS EKS MCP server.
atmos mcp tools aws-eks

# Example output:
# TOOL                    DESCRIPTION
# list_clusters           List EKS clusters in the account
# describe_cluster        Get details of an EKS cluster
# list_nodegroups         List node groups for a cluster
```

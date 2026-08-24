Test connectivity to an external MCP server.

Performs a full connectivity check:
1. Starts the MCP server subprocess
2. Verifies the MCP initialization handshake
3. Lists available tools
4. Sends a ping to verify responsiveness
5. Reports results with success/failure indicators

**Examples:**
```bash
# Test an AWS EKS server.
atmos mcp test aws-eks

# Example output:
# ✓ Server started successfully
# ✓ Initialization handshake complete
# ✓ 12 tools available
# ✓ Server responds to ping
```

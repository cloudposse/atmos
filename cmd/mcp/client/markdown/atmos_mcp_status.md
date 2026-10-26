Show the connection status of all configured MCP servers.

Starts each configured server, checks connectivity, counts available tools,
and displays a summary table. Servers are stopped after the check.

**Example output:**
```
NAME              STATUS    TOOLS   DESCRIPTION
aws-eks           running   12      Amazon EKS cluster management
aws-cost-explorer running   7       AWS Cost Explorer
custom-db         error     0       Internal database query server (connection refused)
```

**Status values:**
- `running` — Server started, handshake complete, ping successful
- `degraded` — Server started but ping failed
- `error` — Server failed to start

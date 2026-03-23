List all external MCP servers configured in `atmos.yaml` under `mcp.servers`.

Displays a table with the server name, connection status, and description.
Servers are not started — use `atmos mcp status` to see live connection status.

**Example output:**
```
NAME              STATUS    DESCRIPTION
aws-eks           stopped   Amazon EKS cluster management
aws-cost-explorer stopped   AWS Cost Explorer
custom-db         stopped   Internal database query server
```

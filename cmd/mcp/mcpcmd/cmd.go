// Package mcpcmd holds the shared MCP root command variable.
// This package exists to break the import cycle between cmd/mcp and its
// subpackages (cmd/mcp/server, cmd/mcp/client).
package mcpcmd

import "github.com/spf13/cobra"

// McpCmd is the root mcp command. Subpackages add their subcommands to it.
var McpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP servers and external server connections",
	Long: `Manage the Atmos MCP (Model Context Protocol) server and external MCP server connections.

The MCP server allows AI assistants (Claude Desktop, Claude Code, VSCode, etc.) to access
Atmos infrastructure management capabilities through a standardized protocol.

External MCP servers (AWS, GCP, custom) can be configured in atmos.yaml under mcp.servers
and their tools become available in atmos ai chat and atmos ai exec.

Use 'atmos mcp start' to start the Atmos MCP server.
Use 'atmos mcp list' to see configured external MCP servers.`,
}

// Package mcpcmd holds the shared MCP root command variable.
// This package exists to break the import cycle between cmd/mcp and its
// subpackages (cmd/mcp/server, cmd/mcp/client).
package mcpcmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed markdown/atmos_mcp.md
var mcpLongMarkdown string

// McpCmd is the root mcp command. Subpackages add their subcommands to it.
var McpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP servers and external server connections",
	Long:  mcpLongMarkdown,
}

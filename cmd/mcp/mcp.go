package mcp

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// mcpCmd represents the mcp command.
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage Model Context Protocol (MCP) server",
	Long: `Manage the Atmos MCP (Model Context Protocol) server.

The MCP server allows AI assistants (Claude Desktop, Claude Code, VSCode, etc.) to access
Atmos infrastructure management capabilities through a standardized protocol.

Use 'atmos mcp start' to start the server.`,
}

// init registers the mcp command provider with the internal registry.
func init() {
	internal.Register(&MCPCommandProvider{})
}

// MCPCommandProvider implements the CommandProvider interface.
type MCPCommandProvider struct{}

// GetCommand returns the mcp command.
func (m *MCPCommandProvider) GetCommand() *cobra.Command {
	return mcpCmd
}

// GetName returns the command name.
func (m *MCPCommandProvider) GetName() string {
	return "mcp"
}

// GetGroup returns the command group for help organization.
func (m *MCPCommandProvider) GetGroup() string {
	return "AI Commands"
}

// GetFlagsBuilder returns the flags builder for this command.
func (m *MCPCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (m *MCPCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (m *MCPCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases (none for mcp).
func (m *MCPCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (m *MCPCommandProvider) IsExperimental() bool {
	return true
}

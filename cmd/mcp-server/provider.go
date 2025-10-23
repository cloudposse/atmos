package mcpserver

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// Provider implements CommandProvider for the mcp-server command.
type Provider struct{}

// GetCommand returns the mcp-server command.
func (p *Provider) GetCommand() *cobra.Command {
	return NewCommand()
}

// GetName returns the command name.
func (p *Provider) GetName() string {
	return "mcp-server"
}

// GetGroup returns the command group for help organization.
func (p *Provider) GetGroup() string {
	return "Other Commands"
}

func init() {
	// Register this command provider with the command registry.
	internal.Register(&Provider{})
}

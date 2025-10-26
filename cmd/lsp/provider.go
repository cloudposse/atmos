package lsp

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// Provider implements CommandProvider for the lsp command.
type Provider struct{}

// GetCommand returns the lsp command.
func (p *Provider) GetCommand() *cobra.Command {
	return NewLSPCommand()
}

// GetName returns the command name.
func (p *Provider) GetName() string {
	return "lsp"
}

// GetGroup returns the command group for help organization.
func (p *Provider) GetGroup() string {
	return "Other Commands"
}

func init() {
	// Register this command provider with the command registry.
	internal.Register(&Provider{})
}

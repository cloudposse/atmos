package lsp

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
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

// GetFlagsBuilder returns the flags builder for this command.
func (p *Provider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (p *Provider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (p *Provider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns a list of command aliases to register.
func (p *Provider) GetAliases() []internal.CommandAlias {
	return nil
}

func init() {
	// Register this command provider with the command registry.
	internal.Register(&Provider{})
}

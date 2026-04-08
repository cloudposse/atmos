// Package pro provides CLI commands for Atmos Pro features.
package pro

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// proCmd represents the pro command group.
var proCmd = &cobra.Command{
	Use:   "pro",
	Short: "Access premium features integrated with atmos-pro.com",
	Long:  `This command allows you to manage and configure premium features available through atmos-pro.com.`,
	Args:  cobra.NoArgs,
}

func init() {
	// Add subcommands.
	proCmd.AddCommand(lockCmd)
	proCmd.AddCommand(unlockCmd)
	proCmd.AddCommand(installCmd)

	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&ProCommandProvider{})
}

// ProCommandProvider implements the CommandProvider interface.
type ProCommandProvider struct{}

// GetCommand returns the pro command.
func (p *ProCommandProvider) GetCommand() *cobra.Command {
	return proCmd
}

// GetName returns the command name.
func (p *ProCommandProvider) GetName() string {
	return "pro"
}

// GetGroup returns the command group for help organization.
func (p *ProCommandProvider) GetGroup() string {
	return "Pro Features"
}

// GetFlagsBuilder returns the flags builder for this command.
func (p *ProCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (p *ProCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (p *ProCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases.
func (p *ProCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns true because pro commands are experimental.
func (p *ProCommandProvider) IsExperimental() bool {
	return true
}

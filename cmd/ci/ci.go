// Package ci provides CI/CD integration commands.
package ci

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// ciCmd represents the ci command group.
var ciCmd = &cobra.Command{
	Use:   "ci",
	Short: "CI/CD integration commands",
	Long:  `Commands for CI/CD integration, including status checks, outputs, and planfile management.`,
}

func init() {
	// Add subcommands.
	ciCmd.AddCommand(statusCmd)

	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&CICommandProvider{})
}

// CICommandProvider implements the CommandProvider interface.
type CICommandProvider struct{}

// GetCommand returns the ci command.
func (c *CICommandProvider) GetCommand() *cobra.Command {
	return ciCmd
}

// GetName returns the command name.
func (c *CICommandProvider) GetName() string {
	return "ci"
}

// GetGroup returns the command group for help organization.
func (c *CICommandProvider) GetGroup() string {
	return "CI/CD Integration"
}

// GetFlagsBuilder returns the flags builder for this command.
// CI command has no flags at the parent level.
func (c *CICommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
// CI command has no positional arguments.
func (c *CICommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
// CI command has no compatibility flags.
func (c *CICommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases.
// CI command has no aliases.
func (c *CICommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns true because CI commands are experimental.
func (c *CICommandProvider) IsExperimental() bool {
	return true
}

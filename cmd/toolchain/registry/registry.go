package registry

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// registryCmd represents the registry command group.
var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage toolchain registries",
	Long:  `Commands for searching and listing tools in toolchain registries.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Show help when no subcommands are provided.
		return cmd.Help()
	},
}

func init() {
	// Add subcommands.
	registryCmd.AddCommand(listCmd)
	registryCmd.AddCommand(searchCmd)
}

// GetRegistryCommand returns the registry command for parent command to add.
func GetRegistryCommand() *cobra.Command {
	return registryCmd
}

// RegistryCommandProvider implements the CommandProvider interface.
type RegistryCommandProvider struct{}

func (r *RegistryCommandProvider) GetCommand() *cobra.Command {
	return registryCmd
}

func (r *RegistryCommandProvider) GetName() string {
	return "registry"
}

func (r *RegistryCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (r *RegistryCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (r *RegistryCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (r *RegistryCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

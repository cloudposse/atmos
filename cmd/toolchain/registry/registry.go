package registry

import (
	"github.com/spf13/cobra"
)

// registryCmd represents the registry command group.
var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage toolchain registries",
	Long:  `Commands for searching and listing tools in toolchain registries.`,
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

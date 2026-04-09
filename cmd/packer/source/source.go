// Package source provides CLI commands for managing packer component sources.
// This includes JIT (just-in-time) vendoring from source configuration.
package source

import (
	"github.com/spf13/cobra"

	sourcecmd "github.com/cloudposse/atmos/pkg/provisioner/source/cmd"
)

// packerConfig holds the component-type-specific configuration for packer.
var packerConfig = &sourcecmd.Config{
	ComponentType: "packer",
	TypeLabel:     "Packer",
}

// sourceCmd represents the source command.
var sourceCmd = &cobra.Command{
	Use:   "source",
	Short: "Manage Packer component sources (JIT vendoring)",
	Long: `Manage Packer component sources defined in stack configuration.

The source provisioner enables just-in-time (JIT) vendoring of component sources
directly from stack configuration. Components can declare their source location
inline using the source field without requiring a separate component.yaml file.

Commands:
  pull      Vendor component source (use --force to re-vendor)
  list      List components with source in a stack
  describe  Show source configuration for a component
  delete    Remove vendored source directory`,
}

func init() {
	// Add subcommands from shared package.
	sourceCmd.AddCommand(sourcecmd.PullCommand(packerConfig))
	sourceCmd.AddCommand(sourcecmd.ListCommand(packerConfig))
	sourceCmd.AddCommand(sourcecmd.DescribeCommand(packerConfig))
	sourceCmd.AddCommand(sourcecmd.DeleteCommand(packerConfig))
}

// GetSourceCommand returns the source command for parent registration.
func GetSourceCommand() *cobra.Command {
	return sourceCmd
}

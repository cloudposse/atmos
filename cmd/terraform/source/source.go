// Package source provides CLI commands for managing terraform component sources.
// This includes JIT (just-in-time) vendoring from source configuration.
package source

import (
	"github.com/spf13/cobra"

	sourcecmd "github.com/cloudposse/atmos/pkg/provisioner/source/cmd"
)

// terraformConfig holds the component-type-specific configuration for terraform.
var terraformConfig = &sourcecmd.Config{
	ComponentType: "terraform",
	TypeLabel:     "Terraform",
}

// sourceCmd represents the source command.
var sourceCmd = &cobra.Command{
	Use:   "source",
	Short: "Manage Terraform component sources (JIT vendoring)",
	Long: `Manage Terraform component sources defined in stack configuration.

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
	sourceCmd.AddCommand(sourcecmd.PullCommand(terraformConfig))
	sourceCmd.AddCommand(sourcecmd.ListCommand(terraformConfig))
	sourceCmd.AddCommand(sourcecmd.DescribeCommand(terraformConfig))
	sourceCmd.AddCommand(sourcecmd.DeleteCommand(terraformConfig))
}

// GetSourceCommand returns the source command for parent registration.
func GetSourceCommand() *cobra.Command {
	return sourceCmd
}

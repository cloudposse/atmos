// Package source provides CLI commands for inspecting aws/cloudformation component sources.
// This includes JIT (just-in-time) vendoring from source configuration.
package source

import (
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	sourcecmd "github.com/cloudposse/atmos/pkg/provisioner/source/cmd"
)

// cloudFormationConfig holds the component-type-specific configuration for aws/cloudformation.
var cloudFormationConfig = &sourcecmd.Config{
	ComponentType: cfg.CloudFormationComponentType,
	TypeLabel:     "CloudFormation",
	CLIName:       "aws cloudformation",
}

// sourceCmd represents the source command.
var sourceCmd = &cobra.Command{
	Use:   "source",
	Short: "Manage aws/cloudformation component sources (JIT vendoring)",
	Long: `Manage aws/cloudformation component sources defined in stack configuration.

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
	sourceCmd.AddCommand(sourcecmd.PullCommand(cloudFormationConfig))
	sourceCmd.AddCommand(sourcecmd.ListCommand(cloudFormationConfig))
	sourceCmd.AddCommand(sourcecmd.DescribeCommand(cloudFormationConfig))
	sourceCmd.AddCommand(sourcecmd.DeleteCommand(cloudFormationConfig))
}

// GetSourceCommand returns the source command for parent registration.
func GetSourceCommand() *cobra.Command {
	return sourceCmd
}

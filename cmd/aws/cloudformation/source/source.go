// Package source provides CLI commands for inspecting aws/cloudformation component sources.
// This includes JIT (just-in-time) vendoring from source configuration.
package source

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
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
	pullCmd := sourcecmd.PullCommand(cloudFormationConfig)
	internal.RegisterCommandMetadata(
		pullCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/aws/cloudformation/source/pull"),
	)
	sourceCmd.AddCommand(pullCmd)
	listCmd := sourcecmd.ListCommand(cloudFormationConfig)
	internal.RegisterCommandMetadata(
		listCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/aws/cloudformation/source/list"),
	)
	sourceCmd.AddCommand(listCmd)
	describeCmd := sourcecmd.DescribeCommand(cloudFormationConfig)
	internal.RegisterCommandMetadata(
		describeCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/aws/cloudformation/source/describe"),
	)
	sourceCmd.AddCommand(describeCmd)
	deleteCmd := sourcecmd.DeleteCommand(cloudFormationConfig)
	internal.RegisterCommandMetadata(
		deleteCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/aws/cloudformation/source/delete"),
	)
	sourceCmd.AddCommand(deleteCmd)
	internal.RegisterCommandMetadata(
		sourceCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/aws/cloudformation/source/source"),
	)
}

// GetSourceCommand returns the source command for parent registration.
func GetSourceCommand() *cobra.Command {
	return sourceCmd
}

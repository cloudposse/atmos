// Package source provides CLI commands for managing helmfile component sources.
// This includes JIT (just-in-time) vendoring from source configuration.
package source

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	sourcecmd "github.com/cloudposse/atmos/pkg/provisioner/source/cmd"
)

// helmfileConfig holds the component-type-specific configuration for helmfile.
var helmfileConfig = &sourcecmd.Config{
	ComponentType: "helmfile",
	TypeLabel:     "Helmfile",
}

// sourceCmd represents the source command.
var sourceCmd = &cobra.Command{
	Use:   "source",
	Short: "Manage Helmfile component sources (JIT vendoring)",
	Long: `Manage Helmfile component sources defined in stack configuration.

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
	pullCmd := sourcecmd.PullCommand(helmfileConfig)
	internal.RegisterCommandMetadata(
		pullCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/helmfile/source/pull"),
	)
	sourceCmd.AddCommand(pullCmd)
	listCmd := sourcecmd.ListCommand(helmfileConfig)
	internal.RegisterCommandMetadata(
		listCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/helmfile/source/list"),
	)
	sourceCmd.AddCommand(listCmd)
	describeCmd := sourcecmd.DescribeCommand(helmfileConfig)
	internal.RegisterCommandMetadata(
		describeCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/helmfile/source/describe"),
	)
	sourceCmd.AddCommand(describeCmd)
	deleteCmd := sourcecmd.DeleteCommand(helmfileConfig)
	internal.RegisterCommandMetadata(
		deleteCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/helmfile/source/delete"),
	)
	sourceCmd.AddCommand(deleteCmd)
	internal.RegisterCommandMetadata(
		sourceCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/helmfile/source"),
	)
}

// GetSourceCommand returns the source command for parent registration.
func GetSourceCommand() *cobra.Command {
	return sourceCmd
}

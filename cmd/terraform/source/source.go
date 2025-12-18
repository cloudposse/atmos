// Package source provides CLI commands for managing terraform component sources.
// This includes JIT (just-in-time) vendoring from source configuration.
package source

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/schema"
)

// AtmosConfigPtr will be set by SetAtmosConfig before command execution.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig sets the Atmos configuration for the source command.
// This is called from root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// sourceCmd represents the source command.
var sourceCmd = &cobra.Command{
	Use:   "source",
	Short: "Manage terraform component sources (JIT vendoring)",
	Long: `Manage terraform component sources defined in stack configuration.

The source provisioner enables just-in-time (JIT) vendoring of component sources
directly from stack configuration. Components can declare their source location
inline using the source field without requiring a separate component.yaml file.

Commands:
  create    Vendor component source from source configuration
  update    Re-vendor component source (force refresh)
  list      List components with source in a stack
  describe  Show source configuration for a component
  delete    Remove vendored source directory`,
}

func init() {
	// Add CRUD subcommands.
	sourceCmd.AddCommand(createCmd)
	sourceCmd.AddCommand(updateCmd)
	sourceCmd.AddCommand(listCmd)
	sourceCmd.AddCommand(describeCmd)
	sourceCmd.AddCommand(deleteCmd)
}

// GetSourceCommand returns the source command for parent registration.
func GetSourceCommand() *cobra.Command {
	return sourceCmd
}

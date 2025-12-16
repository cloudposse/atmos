package workdir

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/schema"
)

// atmosConfigPtr will be set by SetAtmosConfig before command execution.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig sets the Atmos configuration for the workdir command.
// This is called from root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// workdirCmd represents the workdir command.
var workdirCmd = &cobra.Command{
	Use:   "workdir",
	Short: "Manage component working directories",
	Long:  `List, describe, show, and clean component working directories.`,
}

func init() {
	// Add subcommands.
	workdirCmd.AddCommand(listCmd)
	workdirCmd.AddCommand(describeCmd)
	workdirCmd.AddCommand(showCmd)
	workdirCmd.AddCommand(cleanCmd)
}

// GetWorkdirCommand returns the workdir command for parent registration.
func GetWorkdirCommand() *cobra.Command {
	return workdirCmd
}

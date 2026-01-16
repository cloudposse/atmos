package backend

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/schema"
)

// AtmosConfigPtr will be set by SetAtmosConfig before command execution.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig sets the Atmos configuration for the backend command.
// This is called from root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// backendCmd represents the backend command.
var backendCmd = &cobra.Command{
	Use:   "backend",
	Short: "Manage Terraform state backends",
	Long:  `Create, list, describe, update, and delete Terraform state backends.`,
}

func init() {
	// Mark this subcommand as experimental.
	backendCmd.Annotations = map[string]string{"experimental": "true"}

	// Add CRUD subcommands.
	backendCmd.AddCommand(createCmd)
	backendCmd.AddCommand(listCmd)
	backendCmd.AddCommand(describeCmd)
	backendCmd.AddCommand(updateCmd)
	backendCmd.AddCommand(deleteCmd)
}

// GetBackendCommand returns the backend command for parent registration.
func GetBackendCommand() *cobra.Command {
	return backendCmd
}

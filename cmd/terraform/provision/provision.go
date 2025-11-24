package provision

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/terraform/provision/backend"
)

// provisionCmd represents the provision command.
var provisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Provision infrastructure resources",
	Long:  `Provision and manage infrastructure resources like backends.`,
}

func init() {
	// Add backend subcommand.
	provisionCmd.AddCommand(backend.GetBackendCommand())
}

// GetProvisionCommand returns the provision command for attachment to terraform parent.
// This follows the existing pattern used by terraform subcommands.
func GetProvisionCommand() *cobra.Command {
	return provisionCmd
}

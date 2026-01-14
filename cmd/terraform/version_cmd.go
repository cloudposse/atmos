package terraform

import (
	"github.com/spf13/cobra"
)

// versionCmd represents the terraform version command.
// Named versionCmd_ to avoid conflict with the version package.
var versionCmd_ = &cobra.Command{
	Use:   "version",
	Short: "Show the current Terraform version",
	Long: `Displays the current version of Terraform installed on the system.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/version
  https://opentofu.org/docs/cli/commands/version`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Note: No RegisterTerraformCompletions call - version command doesn't use components.

	// Attach to parent terraform command.
	terraformCmd.AddCommand(versionCmd_)
}

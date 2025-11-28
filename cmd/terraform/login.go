package terraform

import (
	"github.com/spf13/cobra"
)

// loginCmd represents the terraform login command.
var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Obtain and save credentials for a remote host",
	Long: `Retrieves an authentication token for the given hostname and saves it in a credentials file.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/login
  https://opentofu.org/docs/cli/commands/login`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for loginCmd.
	RegisterTerraformCompletions(loginCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(loginCmd)
}

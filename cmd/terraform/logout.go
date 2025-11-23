package terraform

import (
	"github.com/spf13/cobra"
)

// logoutCmd represents the terraform logout command.
var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove locally-stored credentials for a remote host",
	Long: `Removes locally-stored credentials for the given hostname.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/logout
  https://opentofu.org/docs/cli/commands/logout`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for logoutCmd.
	RegisterTerraformCompletions(logoutCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(logoutCmd)
}

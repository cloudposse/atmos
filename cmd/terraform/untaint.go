package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// untaintCmd represents the terraform untaint command.
var untaintCmd = &cobra.Command{
	Use:   "untaint",
	Short: "Remove the 'tainted' state from a resource instance",
	Long: `Remove the tainted state from a resource, preventing it from being destroyed and recreated on the next apply.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/untaint
  https://opentofu.org/docs/cli/commands/untaint`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for untaintCmd.
	RegisterTerraformCompletions(untaintCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "untaint", UntaintCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(untaintCmd)
}

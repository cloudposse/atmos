package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// destroyCmd represents the terraform destroy command.
var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy previously-created infrastructure",
	Long: `Destroy all the infrastructure managed by Terraform, removing resources as defined in the state file.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/destroy
  https://opentofu.org/docs/cli/commands/destroy`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for destroy command.
	RegisterTerraformCompletions(destroyCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "destroy", DestroyCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(destroyCmd)
}

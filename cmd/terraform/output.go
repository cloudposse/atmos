package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// outputCmd represents the terraform output command.
var outputCmd = &cobra.Command{
	Use:   "output",
	Short: "Show output values from your root module",
	Long: `Read an output variable from the state file.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/output
  https://opentofu.org/docs/cli/commands/output`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for outputCmd.
	RegisterTerraformCompletions(outputCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "output", OutputCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(outputCmd)
}

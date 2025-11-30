package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// showCmd represents the terraform show command.
var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current state or a saved plan",
	Long: `Show the current state or a saved plan in a human-readable format.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/show
  https://opentofu.org/docs/cli/commands/show`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for showCmd.
	RegisterTerraformCompletions(showCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "show", ShowCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(showCmd)
}

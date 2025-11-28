package terraform

import (
	"github.com/spf13/cobra"
)

// refreshCmd represents the terraform refresh command.
var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Update the state to match remote systems",
	Long: `Refresh the Terraform state, reconciling the local state with the actual infrastructure state.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/refresh
  https://opentofu.org/docs/cli/commands/refresh`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags
	setCustomHelp(refreshCmd, RefreshCompatFlagDescriptions())

	// Register completions for refreshCmd.
	RegisterTerraformCompletions(refreshCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(refreshCmd)
}

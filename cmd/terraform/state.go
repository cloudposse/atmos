package terraform

import (
	"github.com/spf13/cobra"
)

// stateCmd represents the terraform state command.
var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Advanced state management",
	Long: `Advanced commands for managing Terraform state.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/state
  https://opentofu.org/docs/cli/commands/state`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags
	setCustomHelp(stateCmd, StateCompatFlagDescriptions())

	// Register completions for stateCmd.
	RegisterTerraformCompletions(stateCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(stateCmd)
}

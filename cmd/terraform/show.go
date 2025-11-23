package terraform

import (
	"github.com/spf13/cobra"
)

// showCmd represents the terraform show command.
var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current state or a saved plan",
	Long: `Show the current state or a saved plan in a human-readable format.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/show
  https://opentofu.org/docs/cli/commands/show`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags
	setCustomHelp(showCmd, ShowCompatFlagDescriptions())

	// Register completions for showCmd.
	RegisterTerraformCompletions(showCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(showCmd)
}

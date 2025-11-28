package terraform

import (
	"github.com/spf13/cobra"
)

// consoleCmd represents the terraform console command.
var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Try Terraform expressions at an interactive command prompt",
	Long: `Start an interactive console for evaluating Terraform expressions.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/console
  https://opentofu.org/docs/cli/commands/console`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags
	setCustomHelp(consoleCmd, ConsoleCompatFlagDescriptions())

	// Register completions for consoleCmd.
	RegisterTerraformCompletions(consoleCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(consoleCmd)
}

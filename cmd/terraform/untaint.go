package terraform

import (
	"github.com/spf13/cobra"
)

// untaintCmd represents the terraform untaint command.
var untaintCmd = &cobra.Command{
	Use:   "untaint",
	Short: "Remove the 'tainted' state from a resource instance",
	Long: `Remove the tainted state from a resource, preventing it from being destroyed and recreated on the next apply.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/untaint
  https://opentofu.org/docs/cli/commands/untaint`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags
	setCustomHelp(untaintCmd, UntaintCompatFlagDescriptions())

	// Register completions for untaintCmd.
	RegisterTerraformCompletions(untaintCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(untaintCmd)
}

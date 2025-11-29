package terraform

import (
	"github.com/spf13/cobra"
)

// taintCmd represents the terraform taint command.
var taintCmd = &cobra.Command{
	Use:   "taint",
	Short: "Mark a resource instance as not fully functional",
	Long: `Mark a resource as tainted, forcing it to be destroyed and recreated on the next apply.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/taint
  https://opentofu.org/docs/cli/commands/taint`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags.
	setCustomHelp(taintCmd)

	// Register completions for taintCmd.
	RegisterTerraformCompletions(taintCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(taintCmd)
}

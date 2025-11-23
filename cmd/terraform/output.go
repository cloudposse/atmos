package terraform

import (
	"github.com/spf13/cobra"
)

// outputCmd represents the terraform output command.
var outputCmd = &cobra.Command{
	Use:   "output",
	Short: "Show output values from your root module",
	Long: `Read an output variable from the state file.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/output
  https://opentofu.org/docs/cli/commands/output`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags
	setCustomHelp(outputCmd, OutputCompatFlagDescriptions())

	// Register completions for outputCmd.
	RegisterTerraformCompletions(outputCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(outputCmd)
}

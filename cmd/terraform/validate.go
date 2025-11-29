package terraform

import (
	"github.com/spf13/cobra"
)

// validateCmd represents the terraform validate command.
var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check whether the configuration is valid",
	Long: `Validate runs checks that verify whether a configuration is syntactically valid and internally consistent.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/validate
  https://opentofu.org/docs/cli/commands/validate`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags.
	setCustomHelp(validateCmd)

	// Register completions for validateCmd.
	RegisterTerraformCompletions(validateCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(validateCmd)
}

package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// testCmd represents the terraform test command.
var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Execute integration tests for Terraform modules",
	Long: `Run integration tests for Terraform modules.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/test
  https://opentofu.org/docs/cli/commands/test`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for testCmd.
	RegisterTerraformCompletions(testCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "test", TestCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(testCmd)
}

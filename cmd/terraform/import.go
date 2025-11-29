package terraform

import (
	"github.com/spf13/cobra"
)

// importCmd represents the terraform import command.
var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import existing infrastructure into Terraform state",
	Long: `Import existing infrastructure resources into Terraform's state.

Before executing the command, it searches for the 'region' variable in the specified
component and stack configuration. If the 'region' variable is found, it sets the
'AWS_REGION' environment variable with the corresponding value before executing
the import command.

The import command runs: 'terraform import [ADDRESS] [ID]'

Arguments:
- ADDRESS: The Terraform address of the resource to import.
- ID: The ID of the resource to import.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/import
  https://opentofu.org/docs/cli/commands/import`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags.
	setCustomHelp(importCmd)

	// Register completions for importCmd.
	RegisterTerraformCompletions(importCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(importCmd)
}

package terraform

import (
	"github.com/spf13/cobra"
)

// initCmd represents the terraform init command.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Prepare your working directory for other commands",
	Long: `Initialize the working directory containing Terraform configuration files.

It will download necessary provider plugins and set up the backend.
Note: Atmos will automatically call init for you when running plan and apply commands.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/init
  https://opentofu.org/docs/cli/commands/init`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags
	setCustomHelp(initCmd, InitCompatFlagDescriptions())

	// Register completions for initCmd.
	RegisterTerraformCompletions(initCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(initCmd)
}

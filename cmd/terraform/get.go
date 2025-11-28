package terraform

import (
	"github.com/spf13/cobra"
)

// getCmd represents the terraform get command.
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Install or upgrade remote Terraform modules",
	Long: `Download and install modules needed for the configuration.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/get
  https://opentofu.org/docs/cli/commands/get`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags
	setCustomHelp(getCmd, GetCompatFlagDescriptions())

	// Register completions for getCmd.
	RegisterTerraformCompletions(getCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(getCmd)
}

package terraform

import (
	"github.com/spf13/cobra"
)

// providersCmd represents the terraform providers command.
var providersCmd = &cobra.Command{
	Use:   "providers",
	Short: "Show the providers required for this configuration",
	Long: `Prints a tree of the providers used in the configuration.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/providers
  https://opentofu.org/docs/cli/commands/providers`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Set custom help to show terraform native flags
	setCustomHelp(providersCmd, ProvidersCompatFlagDescriptions())

	// Register completions for providersCmd.
	RegisterTerraformCompletions(providersCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(providersCmd)
}

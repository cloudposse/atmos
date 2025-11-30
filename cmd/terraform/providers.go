package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// providersCmd represents the terraform providers command.
var providersCmd = &cobra.Command{
	Use:   "providers",
	Short: "Show the providers required for this configuration",
	Long: `Prints a tree of the providers used in the configuration.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/providers
  https://opentofu.org/docs/cli/commands/providers`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for providersCmd.
	RegisterTerraformCompletions(providersCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "providers", ProvidersCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(providersCmd)
}

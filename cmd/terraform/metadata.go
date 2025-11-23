package terraform

import (
	"github.com/spf13/cobra"
)

// metadataCmd represents the terraform metadata command.
var metadataCmd = &cobra.Command{
	Use:   "metadata",
	Short: "Metadata related commands",
	Long: `Commands for working with Terraform metadata.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/metadata
  https://opentofu.org/docs/cli/commands/metadata`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for metadataCmd.
	RegisterTerraformCompletions(metadataCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(metadataCmd)
}

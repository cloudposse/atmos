package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// metadataCmd represents the terraform metadata command.
var metadataCmd = &cobra.Command{
	Use:   "metadata",
	Short: "Metadata related commands",
	Long: `Commands for working with Terraform metadata.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/metadata
  https://opentofu.org/docs/cli/commands/metadata`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Note: No RegisterTerraformCompletions call - metadata command doesn't use components.

	// Attach to parent terraform command.
	terraformCmd.AddCommand(metadataCmd)
	internal.RegisterCommandMetadata(
		metadataCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/terraform/metadata"),
	)
}

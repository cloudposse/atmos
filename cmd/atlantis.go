package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// atlantisCmd executes Atlantis commands.
var atlantisCmd = &cobra.Command{
	Use:                "atlantis",
	Short:              "Generate and manage Atlantis configurations",
	Long:               `Generate and manage Atlantis configurations that use Atmos under the hood to run Terraform workflows, bringing the power of Atmos to Atlantis for streamlined infrastructure automation.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	atlantisCmd.PersistentFlags().Bool("", false, doubleDashHint)
	RootCmd.AddCommand(atlantisCmd)
	internal.RegisterCommandMetadata(
		atlantisCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/atlantis/usage"),
		internal.WithExample("examples/demo-atlantis", "Atlantis"),
	)
}

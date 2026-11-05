package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// proCmd executes 'atmos pro' CLI commands
var proCmd = &cobra.Command{
	Use:                "pro",
	Short:              "Access premium features integrated with atmos-pro.com",
	Long:               `This command allows you to manage and configure premium features available through atmos-pro.com.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	RootCmd.AddCommand(proCmd)
	internal.RegisterCommandMetadata(
		proCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/pro/usage"),
	)
}

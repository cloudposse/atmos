package cmd

import (
	"github.com/spf13/cobra"
)

// awsCmd executes 'aws' CLI commands.
var awsCmd = &cobra.Command{
	Use:                "aws",
	Short:              "Run AWS-specific commands for interacting with cloud resources",
	Long:               `This command allows interaction with AWS resources through various CLI commands.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	awsCmd.PersistentFlags().Bool("", false, doubleDashHint)
	RootCmd.AddCommand(awsCmd)
}

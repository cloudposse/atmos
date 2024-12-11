package cmd

import (
	"github.com/spf13/cobra"
)

// awsCmd executes 'aws' CLI commands
var awsCmd = &cobra.Command{
	Use:                "aws",
	Short:              "Run AWS-specific commands for interacting with cloud resources",
	Long:               `This command executes 'aws' CLI commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	RootCmd.AddCommand(awsCmd)
}

package cmd

import (
	"github.com/spf13/cobra"
)

// awsCmd executes 'aws' CLI commands
var awsCmd = &cobra.Command{
	Use:                "aws",
	Short:              "Execute 'aws' commands",
	Long:               `This command executes 'aws' CLI commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	addUsageCommand(awsCmd, false)
	RootCmd.AddCommand(awsCmd)
}

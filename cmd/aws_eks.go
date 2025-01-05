package cmd

import (
	"github.com/spf13/cobra"
)

// awsCmd executes 'aws eks' CLI commands
var awsEksCmd = &cobra.Command{
	Use:                "eks",
	Short:              "Execute 'aws eks' commands",
	Long:               `This command executes 'aws eks' CLI commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	addUsageCommand(awsEksCmd, false)
	awsCmd.AddCommand(awsEksCmd)
}

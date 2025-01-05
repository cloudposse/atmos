package cmd

import (
	"github.com/spf13/cobra"
)

// proCmd executes 'atmos pro' CLI commands
var proCmd = &cobra.Command{
	Use:                "pro",
	Short:              "Execute 'pro' commands",
	Long:               `This command executes 'atmos pro' CLI commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	addUsageCommand(proCmd, false)
	RootCmd.AddCommand(proCmd)
}

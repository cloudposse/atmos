package cmd

import (
	"github.com/spf13/cobra"
)

// listCmd commands list stacks and components
var listCmd = &cobra.Command{
	Use:                "list",
	Short:              "Execute 'list' commands",
	Long:               `This command lists stacks and components`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	addUsageCommand(listCmd, false)
	RootCmd.AddCommand(listCmd)
}

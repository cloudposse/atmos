package cmd

import (
	"github.com/spf13/cobra"
)

// listCmd commands list stacks and components
var listCmd = &cobra.Command{
	Use:                "list",
	Short:              "List available stacks and components",
	Long:               `Display a list of all available stacks and components defined in your project.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	addUsageCommand(listCmd, false)
	RootCmd.AddCommand(listCmd)
}

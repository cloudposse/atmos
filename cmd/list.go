package cmd

import (
	"github.com/spf13/cobra"
)

// listCmd commands list stacks and components
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available stacks and components",
	Long: `Retrieve and display a list of all available stacks and components in your environment.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	RootCmd.AddCommand(listCmd)
}

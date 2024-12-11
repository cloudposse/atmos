package cmd

import (
	"github.com/spf13/cobra"
)

// listCmd commands list stacks and components
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available stacks and components",
	Long: `The 'list' command retrieves and displays a list of all stacks and components within the environment.
	It provides an overview of the current resources, making it easier to manage and navigate your infrastructure.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	RootCmd.AddCommand(listCmd)
}

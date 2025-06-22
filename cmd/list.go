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
	Args:               cobra.NoArgs,
}

func init() {
	listCmd.PersistentFlags().StringP("selector", "l", "", "Label selector to filter resources (e.g., 'env=prod,tier in (backend)')")
	RootCmd.AddCommand(listCmd)
}

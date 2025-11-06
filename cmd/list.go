package cmd

import (
	"github.com/spf13/cobra"
)

// listCmd commands list stacks and components
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available stacks and components",
	Long:  `Display a list of all available stacks and components defined in your project.`,
	Args:  cobra.NoArgs,
}

func init() {
	RootCmd.AddCommand(listCmd)
}

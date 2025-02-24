package cmd

import (
	"github.com/spf13/cobra"
)

// listCmd represents the base list command that provides subcommands for listing
// various Atmos resources like stacks, components, settings, metadata, etc.
var listCmd = &cobra.Command{
	Use:                "list [command]",
	Short:              "List Atmos resources and configurations",
	Long:               "List and display Atmos resources and configurations",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	RootCmd.AddCommand(listCmd)
}

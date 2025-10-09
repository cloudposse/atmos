package cmd

import (
	"github.com/spf13/cobra"
)

// describeCmd describes configuration for stacks and components.
var describeCmd = &cobra.Command{
	Use:                "describe",
	Short:              "Show details about Atmos configurations and components",
	Long:               `Display configuration details for Atmos CLI, stacks, and components.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	describeCmd.PersistentFlags().StringP("query", "q", "", "Query the results of an `atmos describe` command using `yq` expressions")

	RootCmd.AddCommand(describeCmd)
}

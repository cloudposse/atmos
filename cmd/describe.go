package cmd

import (
	"github.com/spf13/cobra"
)

// describeCmd describes configuration for stacks and components
var describeCmd = &cobra.Command{
	Use:                "describe",
	Short:              "Show details about Atmos configurations and components",
	Long:               `Display configuration details for Atmos CLI, stacks, and components.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	RootCmd.AddCommand(describeCmd)
}

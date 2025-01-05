package cmd

import (
	"github.com/spf13/cobra"
)

// describeCmd describes configuration for stacks and components
var describeCmd = &cobra.Command{
	Use:                "describe",
	Short:              "Execute 'describe' commands",
	Long:               `This command shows configuration for CLI, stacks and components`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	addUsageCommand(describeCmd, false)
	RootCmd.AddCommand(describeCmd)
}

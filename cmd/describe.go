package cmd

import (
	"github.com/spf13/cobra"
)

// describeCmd describes configuration for stacks and components
var describeCmd = &cobra.Command{
	Use:                "describe",
	Short:              "describe",
	Long:               `This command shows configuration for cli, stacks and components`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
}

func init() {
	RootCmd.AddCommand(describeCmd)
}

package cmd

import (
	"github.com/spf13/cobra"
)

// spaceliftDescribeCmd describes various Spacelift configurations
var spaceliftDescribeCmd = &cobra.Command{
	Use:                "describe",
	Short:              "Execute 'spacelift describe' commands",
	Long:               "This command describes various Spacelift configurations",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	spaceliftCmd.AddCommand(spaceliftDescribeCmd)
}

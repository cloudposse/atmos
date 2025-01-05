package cmd

import (
	"github.com/spf13/cobra"
)

// vendorCmd executes 'atmos vendor' CLI commands
var vendorCmd = &cobra.Command{
	Use:                "vendor",
	Short:              "Execute 'vendor' commands",
	Long:               `This command executes 'atmos vendor' CLI commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	addUsageCommand(vendorCmd, false)
	RootCmd.AddCommand(vendorCmd)
}

package cmd

import (
	"github.com/spf13/cobra"
)

// terraformGenerateCmd generates backends and variables for terraform components
var terraformGenerateCmd = &cobra.Command{
	Use:                "generate",
	Short:              "generate",
	Long:               "This command generates backends and variables for terraform components",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
}

func init() {
	terraformCmd.AddCommand(terraformGenerateCmd)
}

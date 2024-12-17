package cmd

import (
	"github.com/spf13/cobra"
)

// terraformGenerateCmd generates configurations for terraform components
var terraformGenerateCmd = &cobra.Command{
	Use:                "generate",
	Short:              "Execute 'terraform generate' commands",
	Long:               "This command generates configurations for terraform components",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run:                terraformRun,
}

func init() {
	terraformCmd.AddCommand(terraformGenerateCmd)
}

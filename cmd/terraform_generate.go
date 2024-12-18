package cmd

import (
	"github.com/spf13/cobra"
)

// terraformGenerateCmd generates configurations for terraform components
var terraformGenerateCmd = &cobra.Command{
	Use:                "generate",
	Short:              "Generate configurations for Terraform components",
	Long:               "This command generates various configuration files for Terraform components in Atmos.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	terraformCmd.AddCommand(terraformGenerateCmd)
}

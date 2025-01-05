package cmd

import (
	"github.com/spf13/cobra"
)

// helmfileGenerateCmd generates configurations for helmfile components
var helmfileGenerateCmd = &cobra.Command{
	Use:                "generate",
	Short:              "Execute 'helmfile generate' commands",
	Long:               "This command generates configurations for helmfile components",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	addUsageCommand(helmfileGenerateCmd, false)
	helmfileCmd.AddCommand(helmfileGenerateCmd)
}

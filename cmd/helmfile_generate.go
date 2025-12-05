package cmd

import (
	"github.com/spf13/cobra"
)

// helmfileGenerateCmd generates configurations for helmfile components.
var helmfileGenerateCmd = &cobra.Command{
	Use:                "generate",
	Short:              "Generate configurations for Helmfile components",
	Long:               "This command generates various configuration files for Helmfile components in Atmos.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	helmfileCmd.AddCommand(helmfileGenerateCmd)
}

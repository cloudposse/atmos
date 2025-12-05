package cmd

import (
	"github.com/spf13/cobra"
)

// validateCmd commands validate stacks and components.
var validateCmd = &cobra.Command{
	Use:                "validate",
	Short:              "Validate configurations against OPA policies and JSON schemas",
	Long:               `This command validates stacks and components by checking their configurations against Open Policy Agent (OPA) policies and JSON schemas.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	RootCmd.AddCommand(validateCmd)
}

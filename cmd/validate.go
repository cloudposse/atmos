package cmd

import (
	"github.com/spf13/cobra"
)

// validateCmd commands validate stacks and components
var validateCmd = &cobra.Command{
	Use:                "validate",
	Short:              "Validate configurations against OPA policies and JSON schemas",
	Long:               `This command validates stacks and components`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	RootCmd.AddCommand(validateCmd)
}

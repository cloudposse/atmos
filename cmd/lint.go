package cmd

import (
	"github.com/spf13/cobra"
)

// lintCmd is the parent command for all lint subcommands.
var lintCmd = &cobra.Command{
	Use:                "lint",
	Short:              "Lint configurations for quality and best practices",
	Long:               `This command lints Atmos stack configurations for anti-patterns, optimization opportunities, and structural issues.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	RootCmd.AddCommand(lintCmd)
}

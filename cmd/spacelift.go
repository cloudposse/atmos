package cmd

import (
	"github.com/spf13/cobra"
)

// spaceliftCmd executes Spacelift commands
var spaceliftCmd = &cobra.Command{
	Use:                "spacelift",
	Short:              "Execute 'spacelift' commands",
	Long:               `This command executes Spacelift integration commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	RootCmd.AddCommand(spaceliftCmd)
}

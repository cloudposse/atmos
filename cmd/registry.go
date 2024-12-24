package cmd

import (
	"github.com/spf13/cobra"
)

// registryCmd executes 'atmos registry' CLI commands
var registryCmd = &cobra.Command{
	Use:                "registry",
	Short:              "Execute 'registry' commands",
	Long:               `This command executes 'atmos registry' CLI commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	RootCmd.AddCommand(registryCmd)
}

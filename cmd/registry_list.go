package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// registryListCmd executes 'registry list' CLI commands
var registryListCmd = &cobra.Command{
	Use:                "list",
	Short:              "Execute 'registry list' commands",
	Long:               `This command executes 'atmos registry list' CLI commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		checkAtmosConfig(WithStackValidation(false))
		err := e.ExecuteRegistryListCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}
	},
}

func init() {
	registryCmd.AddCommand(registryListCmd)
}

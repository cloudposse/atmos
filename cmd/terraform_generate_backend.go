package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformGenerateBackendCmd generates backend config for a terraform component
var terraformGenerateBackendCmd = &cobra.Command{
	Use:                "backend",
	Short:              "Execute 'terraform generate backend' command",
	Long:               `This command generates the backend config for a terraform component: atmos terraform generate backend <component> -s <stack>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateBackendCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	terraformGenerateBackendCmd.DisableFlagParsing = false
	terraformGenerateBackendCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform generate backend <component> -s <stack>")

	err := terraformGenerateBackendCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateBackendCmd)
}

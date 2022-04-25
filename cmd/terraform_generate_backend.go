package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// terraformGenerateBackendCmd generates backend config for a terraform component
var terraformGenerateBackendCmd = &cobra.Command{
	Use:                "backend",
	Short:              "Execute 'terraform generate backend' command",
	Long:               `This command generates the backend config for a terraform component: atmos terraform generate backend <component> -s <stack>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteTerraformGenerateBackend(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	terraformGenerateBackendCmd.DisableFlagParsing = false
	terraformGenerateBackendCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform generate backend <component> -s <stack>")

	err := terraformGenerateBackendCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateBackendCmd)
}

package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// terraformGenerateBackendsCmd generates backend configs for all terraform components
var terraformGenerateBackendsCmd = &cobra.Command{
	Use:                "backends",
	Short:              "Execute 'terraform generate backends' command",
	Long:               `This command generates backend configs for all terraform components`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteTerraformGenerateBackendsCmd(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	terraformGenerateBackendsCmd.DisableFlagParsing = false

	terraformGenerateBackendsCmd.PersistentFlags().String("format", "hcl", "Output format.\n"+
		"Supported formats: hcl, json ('hcl' is default).\n"+
		"atmos terraform generate backends --format=hcl/json")

	terraformGenerateCmd.AddCommand(terraformGenerateBackendsCmd)
}

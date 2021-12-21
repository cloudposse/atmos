package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"os"
)

// terraformGenerateBackendsCmd generates backend configs for all terraform components
var terraformGenerateBackendsCmd = &cobra.Command{
	Use:                "backends",
	Short:              "Execute 'terraform generate backends' command",
	Long:               `This command generates the backend configs for all terraform components`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteTerraformGenerateBackends(cmd, args)
		if err != nil {
			color.Red("%s\n\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	terraformGenerateBackendsCmd.DisableFlagParsing = false
	terraformGenerateBackendsCmd.PersistentFlags().StringP("stack", "s", "", "")

	err := terraformGenerateBackendsCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		color.Red("%s\n\n", err)
		os.Exit(1)
	}

	// terraformGenerateCmd.AddCommand(terraformGenerateBackendsCmd)
}

package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/hooks"
)

type contextKey string

const atmosInfoKey contextKey = "atmos_info"

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands (e.g., plan, apply, destroy) using Atmos stack configurations",
	Long:               `This command allows you to execute Terraform commands, such as plan, apply, and destroy, using Atmos stack configurations for consistent infrastructure management.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		info := getConfigAndStacksInfo("terraform", cmd, args)
		return hooks.RunE(cmd, args, &info)
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform <terraform_command> <component> -s <stack>")
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) {
	info := getConfigAndStacksInfo("terraform", cmd, args)

	if info.NeedHelp {
		err := actualCmd.Usage()
		if err != nil {
			u.LogErrorAndExit(err)
		}
		return
	}

	err := e.ExecuteTerraform(info)
	if err != nil {
		u.LogErrorAndExit(err)
	}
}

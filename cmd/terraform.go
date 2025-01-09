package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands",
	Long:               `This command executes Terraform commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
}

// Contains checks if a slice of strings contains an exact match for the target string.
func Contains(slice []string, target string) bool {
	for _, item := range slice {
		if item == target {
			return true
		}
	}
	return false
}

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
	info := getConfigAndStacksInfo("terraform", cmd, args)
	err := e.ExecuteTerraform(info)
	if err != nil {
		u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
	}
	return nil
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform <terraform_command> <component> -s <stack>")
	addUsageCommand(terraformCmd, true)
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}

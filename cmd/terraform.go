package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	h "github.com/cloudposse/atmos/pkg/hooks"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands (e.g., plan, apply, destroy) using Atmos stack configurations",
	Long:               `This command allows you to execute Terraform commands, such as plan, apply, and destroy, using Atmos stack configurations for consistent infrastructure management.`,
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

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) {
	info := getConfigAndStacksInfo("terraform", cmd, args)
	if info.NeedHelp {
		actualCmd.Usage()
		return
	}
	err := e.ExecuteTerraform(info)
	if err != nil {
		u.LogErrorAndExit(err)
	}
}

func runHooks(event h.HookEvent, cmd *cobra.Command, args []string) error {
	info := getConfigAndStacksInfo("terraform", cmd, append([]string{cmd.Name()}, args...))

	// Initialize the CLI config
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return fmt.Errorf("error initializing CLI config: %w", err)
	}

	hooks, err := h.GetHooks(&atmosConfig, &info)
	if err != nil {
		return fmt.Errorf("error getting hooks: %w", err)
	}

	return hooks.RunAll(event, &atmosConfig, &info, cmd, args)
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform <terraform_command> <component> -s <stack>")
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}

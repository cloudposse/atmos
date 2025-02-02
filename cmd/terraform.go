package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	h "github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Get the config and stacks info
		info := getConfigAndStacksInfo("terraform", cmd, append([]string{cmd.Name()}, args...))

		// Initialize the CLI config
		atmosConfig, err := cfg.InitCliConfig(info, true)
		if err != nil {
			return fmt.Errorf("error initializing CLI config: %w", err)
		}

		// Get the hooks
		hooks, err := h.GetHooks(&atmosConfig, &info)
		if err != nil {
			return fmt.Errorf("error getting hooks: %w", err)
		}

		// Set the the context so it can be accessed by any chiild commands
		ctx := context.WithValue(context.Background(), "atmosConfig", &atmosConfig)
		ctx = context.WithValue(ctx, "info", &info)
		ctx = context.WithValue(ctx, "hooks", hooks)
		cmd.SetContext(ctx)

		return nil
	},
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

func terraformRun(info *schema.ConfigAndStacksInfo, actualCmd *cobra.Command, args []string) {
	if info.NeedHelp {
		actualCmd.Usage()
		return
	}

	err := e.ExecuteTerraform(*info)
	if err != nil {
		u.LogErrorAndExit(err)
	}
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform <terraform_command> <component> -s <stack>")
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}

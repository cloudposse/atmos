package cmd

import (
	"errors"
	"fmt"

	l "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	terrerrors "github.com/cloudposse/atmos/pkg/errors"
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

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().Bool("", false, doubleDashHint)
	AddStackCompletion(terraformCmd)
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
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

	if hooks.HasHooks() {
		l.Info("running hooks", "event", event)
		return hooks.RunAll(event, &atmosConfig, &info, cmd, args)
	}

	return nil
}

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
	info := getConfigAndStacksInfo("terraform", cmd, args)
	if info.NeedHelp {
		err := actualCmd.Usage()
		if err != nil {
			u.LogErrorAndExit(err)
		}
		return nil
	}

	// For plan-diff, ExecuteTerraform will call OsExit directly if there are differences
	// So if we get here, it means there were no differences or there was an error
	err := e.ExecuteTerraform(info)
	if err != nil {
		if errors.Is(err, terrerrors.ErrPlanHasDiff) {
			// Print the error message but return the error to be handled by main.go
			u.PrintErrorMarkdown("", err, "")
			return err
		}
		// For other errors, continue with existing behavior
		u.PrintErrorMarkdownAndExit("", err, "")
	}
	return nil
}

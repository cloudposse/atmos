package cmd

import (
	"errors"
	"fmt"

	log "github.com/charmbracelet/log"
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

var ErrInvalidTerraformFlags = errors.New("the `--affected` flag can't be used with the `component` argument and with the `--all`, `--query` or `--components` flags")

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

	if hooks != nil && hooks.HasHooks() {
		log.Info("running hooks", "event", event)
		return hooks.RunAll(event, &atmosConfig, &info, cmd, args)
	}

	return nil
}

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
	info := getConfigAndStacksInfo("terraform", cmd, args)

	if info.NeedHelp {
		err := actualCmd.Usage()
		if err != nil {
			log.Fatal(err)
		}
		return nil
	}

	flags := cmd.Flags()

	processTemplates, err := flags.GetBool("process-templates")
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}

	processYamlFunctions, err := flags.GetBool("process-functions")
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}

	skip, err := flags.GetStringSlice("skip")
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}

	components, err := flags.GetStringSlice("components")
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}

	dryRun, err := flags.GetBool("dry-run")
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}

	info.ProcessTemplates = processTemplates
	info.ProcessFunctions = processYamlFunctions
	info.Skip = skip
	info.Components = components
	info.DryRun = dryRun == true

	if info.Affected &&
		(info.All || len(info.Components) > 0 || info.Query != "" || info.ComponentFromArg != "") {
		u.PrintErrorMarkdownAndExit("", ErrInvalidTerraformFlags, "")
	}

	// Execute `atmos terraform <sub-command> --affected` or `atmos terraform <sub-command> --affected --stack <stack>`
	if info.Affected {
		err = e.ExecuteTerraformAffected(cmd, args, &info)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
		return nil
	}

	// Execute `atmos terraform <sub-command>` with the filters if any of the following flags are specified:
	// `--all`
	// `--components c1,c2`
	// `--query <yq-expression>`
	// `--stack` (and the `component` argument is not passed)
	if info.All || len(info.Components) > 0 || info.Query != "" || (info.Stack != "" && info.ComponentFromArg == "") {
		err = e.ExecuteTerraformQuery(&info)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
		return nil
	}

	// Execute `atmos terraform <sub-command> <component> --stack <stack>`
	err = e.ExecuteTerraform(info)
	// For plan-diff, ExecuteTerraform will call OsExit directly if there are differences
	// So if we get here, it means there were no differences or there was an error
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

package cmd

import (
	"errors"
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	h "github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

func runHooks(event h.HookEvent, cmd *cobra.Command, args []string) error {
	info := getConfigAndStacksInfo("terraform", cmd, append([]string{cmd.Name()}, args...))

	// Initialize the CLI config
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return fmt.Errorf(errUtils.ErrStringWrappingFormat, errUtils.ErrInitializeCLIConfig, err)
	}

	hooks, err := h.GetHooks(&atmosConfig, &info)
	if err != nil {
		return fmt.Errorf(errUtils.ErrStringWrappingFormat, errUtils.ErrGetHooks, err)
	}

	if hooks != nil && hooks.HasHooks() {
		log.Info("Running hooks", "event", event)
		return hooks.RunAll(event, &atmosConfig, &info, cmd, args)
	}

	return nil
}

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
	info := getConfigAndStacksInfo(cfg.TerraformComponentType, cmd, args)

	if info.NeedHelp {
		err := actualCmd.Usage()
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	flags := cmd.Flags()

	processTemplates, err := flags.GetBool("process-templates")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	processYamlFunctions, err := flags.GetBool("process-functions")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	skip, err := flags.GetStringSlice("skip")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	components, err := flags.GetStringSlice("components")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	dryRun, err := flags.GetBool("dry-run")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	info.ProcessTemplates = processTemplates
	info.ProcessFunctions = processYamlFunctions
	info.Skip = skip
	info.Components = components
	info.DryRun = dryRun

	// Check Terraform Single-Component and Multi-Component flags
	err = checkTerraformFlags(&info)
	errUtils.CheckErrorPrintAndExit(err, "", "")

	// Execute `atmos terraform <sub-command> --affected` or `atmos terraform <sub-command> --affected --stack <stack>`
	if info.Affected {
		// Add these flags because `atmos describe affected` needs them, but `atmos terraform --affected` does not define them
		cmd.PersistentFlags().String("file", "", "")
		cmd.PersistentFlags().String("format", "yaml", "")
		cmd.PersistentFlags().Bool("verbose", false, "")
		cmd.PersistentFlags().Bool("include-spacelift-admin-stacks", false, "")
		cmd.PersistentFlags().Bool("include-settings", false, "")
		cmd.PersistentFlags().Bool("upload", false, "")

		a, err := e.ParseDescribeAffectedCliArgs(cmd, args)
		if err != nil {
			return err
		}

		a.IncludeSpaceliftAdminStacks = false
		a.IncludeSettings = false
		a.Upload = false
		a.OutputFile = ""

		err = e.ExecuteTerraformAffected(&a, &info)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	// Execute `atmos terraform <sub-command>` with the filters if any of the following flags are specified:
	// `--all`
	// `--components c1,c2`
	// `--query <yq-expression>`
	// `--stack` (and the `component` argument is not passed)
	if info.All || len(info.Components) > 0 || info.Query != "" || (info.Stack != "" && info.ComponentFromArg == "") {
		err = e.ExecuteTerraformQuery(&info)
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}

	// Execute `atmos terraform <sub-command> <component> --stack <stack>`
	err = e.ExecuteTerraform(info)
	// For plan-diff, ExecuteTerraform will call OsExit directly if there are differences
	// So if we get here, it means there were no differences or there was an error
	if err != nil {
		if errors.Is(err, errUtils.ErrPlanHasDiff) {
			// Print the error message but return the error to be handled by main.go
			errUtils.CheckErrorAndPrint(err, "", "")
			return err
		}
		// For other errors, continue with existing behavior
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
	return nil
}

// checkTerraformFlags checks the usage of the Single-Component and Multi-Component flags.
func checkTerraformFlags(info *schema.ConfigAndStacksInfo) error {
	// Check Multi-Component flags
	// 1. Specifying the `component` argument is not allowed with the Multi-Component flags
	if info.ComponentFromArg != "" && (info.All || info.Affected || len(info.Components) > 0 || info.Query != "") {
		return fmt.Errorf("component `%s`: %w", info.ComponentFromArg, errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags)
	}
	// 2. `--affected` is not allowed with the other Multi-Component flags
	if info.Affected && (info.All || len(info.Components) > 0 || info.Query != "") {
		return errUtils.ErrInvalidTerraformFlagsWithAffectedFlag
	}

	// Single-Component and Multi-Component flags are not allowed together
	singleComponentFlagPassed := info.PlanFile != "" || info.UseTerraformPlan
	multiComponentFlagPassed := info.Affected || info.All || len(info.Components) > 0 || info.Query != ""
	if singleComponentFlagPassed && multiComponentFlagPassed {
		return errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags
	}

	return nil
}

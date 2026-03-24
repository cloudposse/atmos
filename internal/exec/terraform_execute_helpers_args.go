package exec

// terraform_execute_helpers_args.go contains the per-subcommand argument builders
// extracted from buildTerraformCommandArgs.  Each function is small, pure, and
// independently testable.

import (
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// buildPlanSubcommandArgs extends allArgsAndFlags for the `terraform plan` subcommand.
// It adds the varfile, optionally the planfile, and handles the upload-status flag.
// The uploadStatusFlag is resolved by the caller (buildTerraformCommandArgs) and passed in.
func buildPlanSubcommandArgs( //nolint:revive // argument-limit: uploadStatusFlag avoids computing it twice.
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	allArgsAndFlags []string,
	varFile, planFile string,
	uploadStatusFlag bool,
) []string {
	allArgsAndFlags = append(allArgsAndFlags, varFileFlag, varFile)

	if !u.SliceContainsString(info.AdditionalArgsAndFlags, outFlag) &&
		!u.SliceContainsStringHasPrefix(info.AdditionalArgsAndFlags, outFlag+"=") &&
		!atmosConfig.Components.Terraform.Plan.SkipPlanfile {
		allArgsAndFlags = append(allArgsAndFlags, outFlag, planFile)
	}

	// Always remove the flag from AdditionalArgsAndFlags since it's only used internally by Atmos.
	info.AdditionalArgsAndFlags = u.SliceRemoveFlag(info.AdditionalArgsAndFlags, cfg.UploadStatusFlag)

	if uploadStatusFlag && !u.SliceContainsString(info.AdditionalArgsAndFlags, detailedExitCodeFlag) {
		allArgsAndFlags = append(allArgsAndFlags, detailedExitCodeFlag)
	}

	return allArgsAndFlags
}

// buildApplySubcommandArgs extends allArgsAndFlags for the `terraform apply` subcommand.
// When not consuming a pre-built plan, it appends the varfile.
func buildApplySubcommandArgs(
	info *schema.ConfigAndStacksInfo,
	allArgsAndFlags []string,
	varFile string,
) []string {
	if !info.UseTerraformPlan {
		allArgsAndFlags = append(allArgsAndFlags, varFileFlag, varFile)
	}

	return allArgsAndFlags
}

// appendApplyPlanFileArg appends the plan-file positional argument to allArgsAndFlags
// for `terraform apply` when a pre-built plan is being consumed.
// The positional argument must come after all flags.
func appendApplyPlanFileArg(info *schema.ConfigAndStacksInfo, allArgsAndFlags []string, planFile string) []string {
	if info.SubCommand != subcommandApply || !info.UseTerraformPlan {
		return allArgsAndFlags
	}
	if info.PlanFile != "" {
		return append(allArgsAndFlags, info.PlanFile)
	}
	return append(allArgsAndFlags, planFile)
}

// buildInitSubcommandArgs extends allArgsAndFlags for the `terraform init` subcommand.
// It runs provisioners (via prepareInitExecution), optionally updates *componentPath
// via the workdir provisioner, and adds the -reconfigure / -var-file flags when configured.
//
// MUTUAL EXCLUSION CONTRACT: this function is called ONLY when SubCommand == "init"
// (i.e. init is the main command).  For pre-step init invocations, executeTerraformInitPhase
// in terraform_execute_helpers.go handles the provisioner call via prepareInitExecution.
// These two paths must never both execute in the same command invocation or provisioners
// will run twice.
//
// NOTE: buildInitArgs (used by executeTerraformInitPhase) also adds -reconfigure when
// SubCommand == "workspace" because workspace operations need a clean state on each run.
// This function omits that check because the init-as-main-command path never originates
// from a workspace subcommand — the asymmetry is intentional.
func buildInitSubcommandArgs(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	allArgsAndFlags []string,
	varFile string,
	componentPath *string,
) ([]string, error) {
	newPath, provErr := prepareInitExecution(atmosConfig, info, *componentPath)
	if provErr != nil {
		return nil, provErr
	}
	*componentPath = newPath

	if atmosConfig.Components.Terraform.InitRunReconfigure {
		allArgsAndFlags = append(allArgsAndFlags, "-reconfigure")
	}
	if atmosConfig.Components.Terraform.Init.PassVars {
		allArgsAndFlags = append(allArgsAndFlags, varFileFlag, varFile)
	}

	return allArgsAndFlags, nil
}

// buildWorkspaceSubcommandArgs extends allArgsAndFlags for `terraform workspace` subcommands.
// Subcommands with a secondary argument (new, select, delete) also append the workspace name.
func buildWorkspaceSubcommandArgs(info *schema.ConfigAndStacksInfo, allArgsAndFlags []string) []string {
	switch {
	case info.SubCommand2 == "list" || info.SubCommand2 == "show":
		return append(allArgsAndFlags, info.SubCommand2)
	case info.SubCommand2 != "":
		return append(allArgsAndFlags, info.SubCommand2, info.TerraformWorkspace)
	}
	return allArgsAndFlags
}

// buildTerraformCommandArgs constructs the complete argument list for the main terraform
// command based on the subcommand.  For the "init" subcommand it also runs provisioners
// and may update *componentPath via the workdir provisioner.
// Returns the argument list, an uploadStatus flag, and any error from provisioners.
func buildTerraformCommandArgs(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	varFile, planFile string,
	componentPath *string,
) (allArgsAndFlags []string, uploadStatusFlag bool, err error) {
	allArgsAndFlags = strings.Fields(info.SubCommand)

	// Resolve upload-status flag: prefer the structured field set by Cobra/Viper,
	// fall back to parsing AdditionalArgsAndFlags for backward compatibility
	// (e.g., when invoked via legacy code paths that bypass Cobra).
	uploadStatusFlag = info.UploadStatus
	if !uploadStatusFlag {
		uploadStatusFlag = parseUploadStatusFlag(info.AdditionalArgsAndFlags, cfg.UploadStatusFlag)
	}

	switch info.SubCommand {
	case "plan":
		allArgsAndFlags = buildPlanSubcommandArgs(atmosConfig, info, allArgsAndFlags, varFile, planFile, uploadStatusFlag)

	case "destroy", "import", "refresh":
		allArgsAndFlags = append(allArgsAndFlags, varFileFlag, varFile)

	case subcommandApply:
		allArgsAndFlags = buildApplySubcommandArgs(info, allArgsAndFlags, varFile)

	case subcommandInit:
		allArgsAndFlags, err = buildInitSubcommandArgs(atmosConfig, info, allArgsAndFlags, varFile, componentPath)
		if err != nil {
			return nil, false, err
		}

	case subcommandWorkspace:
		allArgsAndFlags = buildWorkspaceSubcommandArgs(info, allArgsAndFlags)
	}

	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)

	// Positional plan-file argument must come after all flags.
	allArgsAndFlags = appendApplyPlanFileArg(info, allArgsAndFlags, planFile)

	return allArgsAndFlags, uploadStatusFlag, nil
}

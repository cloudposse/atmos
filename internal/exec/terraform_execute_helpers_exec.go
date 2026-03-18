package exec

// terraform_execute_helpers_exec.go contains the high-level execution pipeline and
// the lower-level workspace / TTY / exit-code helpers extracted from ExecuteTerraform.
//
// The two orchestrators here (prepareComponentExecution, executeCommandPipeline) are the
// primary tools for reducing ExecuteTerraform's cyclomatic complexity from ~25 to ~10.

import (
	"errors"
	"fmt"
	"os"
	osexec "os/exec"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/dependencies"
	git "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// componentExecContext holds the per-execution state assembled by prepareComponentExecution.
// It is consumed by executeCommandPipeline without further modification.
type componentExecContext struct {
	componentPath string
	varFile       string
	planFile      string
	workingDir    string
	tenv          *dependencies.ToolchainEnvironment
}

// prepareComponentExecution consolidates all per-component setup into one call:
// path resolution, access checks, toolchain installation, variable file generation,
// OPA/JSON-schema validation, auth pre-hook, config file generation, and env assembly.
// Extracting this reduces ExecuteTerraform's cyclomatic complexity by ~10 decision points.
func prepareComponentExecution(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	shouldProcess bool,
) (*componentExecContext, error) {
	if err := checkTerraformConfig(*atmosConfig); err != nil {
		return nil, err
	}

	componentPath, err := resolveAndProvisionComponentPath(atmosConfig, info)
	if err != nil {
		return nil, err
	}

	if err = checkComponentRestrictions(info); err != nil {
		return nil, err
	}

	var tenv *dependencies.ToolchainEnvironment
	if shouldProcess {
		tenv, err = resolveAndInstallToolchainDeps(atmosConfig, info)
		if err != nil {
			return nil, err
		}
		info.Command = tenv.Resolve(info.Command)
	}

	varFile := constructTerraformComponentVarfileName(info)
	planFile := constructTerraformComponentPlanfileName(info)
	workingDir := constructTerraformComponentWorkingDir(atmosConfig, info)

	if err = printAndWriteVarFiles(atmosConfig, info); err != nil {
		return nil, err
	}

	if err = validateTerraformComponent(atmosConfig, info); err != nil {
		return nil, err
	}

	if err = auth.TerraformPreHook(atmosConfig, info); err != nil {
		log.Error("Error executing 'atmos auth terraform pre-hook'",
			logFieldComponent, info.ComponentFromArg, "error", err)
		// Pre-hook failures terminate execution — this matches the original terraform.go behavior.
		// Authentication setup failures must not silently produce unauthenticated terraform commands.
		return nil, err
	}

	if err = generateConfigFiles(atmosConfig, info, workingDir); err != nil {
		return nil, err
	}

	warnOnConflictingEnvVars()

	if err = assembleComponentEnvVars(atmosConfig, info, tenv); err != nil {
		return nil, err
	}

	return &componentExecContext{
		componentPath: componentPath,
		varFile:       varFile,
		planFile:      planFile,
		workingDir:    workingDir,
		tenv:          tenv,
	}, nil
}

// executeCommandPipeline runs the full terraform command pipeline after the component
// has been prepared: optional init pre-step, argument construction, workspace setup,
// TTY guard, and final command execution + cleanup.
// Extracting this reduces ExecuteTerraform's cyclomatic complexity by ~7 decision points.
func executeCommandPipeline(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	execCtx *componentExecContext,
	opts ...ShellCommandOption,
) error {
	componentPath := execCtx.componentPath

	if shouldRunTerraformInit(atmosConfig, info) {
		var err error
		componentPath, err = executeTerraformInitPhase(atmosConfig, info, componentPath, execCtx.varFile)
		if err != nil {
			return err
		}
	}

	handleDeploySubcommand(atmosConfig, info)
	logTerraformContext(info, execCtx.workingDir)

	allArgsAndFlags, uploadStatusFlag, err := buildTerraformCommandArgs(atmosConfig, info, execCtx.varFile, execCtx.planFile, &componentPath)
	if err != nil {
		return err
	}

	if err = runWorkspaceSetup(atmosConfig, info, componentPath); err != nil {
		return err
	}

	if err = checkTTYRequirement(info); err != nil {
		return err
	}

	addRegionEnvVarForImport(info)

	if err = executeMainTerraformCommand(atmosConfig, info, allArgsAndFlags, componentPath, uploadStatusFlag, opts...); err != nil {
		return err
	}

	cleanupTerraformFiles(atmosConfig, info)
	return nil
}

// runWorkspaceSetup selects (or creates) the Terraform workspace before the main command
// runs.  It is a no-op when: the subcommand is init, the caller is already operating on
// a named workspace (SubCommand2 != ""), the HTTP backend is in use, or the caller has
// set TF_WORKSPACE themselves.
func runWorkspaceSetup(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) error {
	if info.SubCommand == "init" || (info.SubCommand == "workspace" && info.SubCommand2 != "") {
		return nil
	}
	if info.ComponentBackendType == "http" {
		return nil
	}
	if os.Getenv("TF_WORKSPACE") != "" {
		return nil
	}

	// Default: redirect workspace-select stderr to stdout so it is visible.
	workspaceSelectRedirectStdErr := "/dev/stdout"
	if info.RedirectStdErr != "" {
		workspaceSelectRedirectStdErr = info.RedirectStdErr
	}

	// For data-producing subcommands redirect "Switched to workspace…" to stderr
	// so it doesn't pollute captured stdout in $() substitutions.
	var wsOpts []ShellCommandOption
	if info.SubCommand == "output" || info.SubCommand == "show" {
		wsOpts = append(wsOpts, WithStdoutOverride(os.Stderr))
	}

	err := ExecuteShellCommand(
		*atmosConfig,
		info.Command,
		[]string{"workspace", "select", info.TerraformWorkspace},
		componentPath,
		info.ComponentEnvList,
		info.DryRun,
		workspaceSelectRedirectStdErr,
		wsOpts...,
	)
	if err == nil {
		return nil
	}

	// Exit code 1 means the workspace doesn't exist yet; create it.
	var exitCodeErr errUtils.ExitCodeError
	if !errors.As(err, &exitCodeErr) || exitCodeErr.Code != 1 {
		return err
	}

	return ExecuteShellCommand(
		*atmosConfig,
		info.Command,
		[]string{"workspace", "new", info.TerraformWorkspace},
		componentPath,
		info.ComponentEnvList,
		info.DryRun,
		info.RedirectStdErr,
	)
}

// checkTTYRequirement returns an error when `terraform apply` is invoked without
// -auto-approve in a non-interactive environment (stdin is nil).
func checkTTYRequirement(info *schema.ConfigAndStacksInfo) error {
	if os.Stdin != nil {
		return nil
	}
	if info.SubCommand == "apply" && !u.SliceContainsString(info.AdditionalArgsAndFlags, autoApproveFlag) {
		return fmt.Errorf(
			"%w: 'terraform apply' requires a user interaction, but no TTY is attached. "+
				"Use 'terraform apply -auto-approve' or 'terraform deploy' instead",
			errUtils.ErrNoTty,
		)
	}
	return nil
}

// addRegionEnvVarForImport appends AWS_REGION to the component env list when the
// subcommand is `import` and the component has a `region` variable configured.
func addRegionEnvVarForImport(info *schema.ConfigAndStacksInfo) {
	if info.SubCommand != "import" {
		return
	}
	if region, ok := info.ComponentVarsSection["region"].(string); ok {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("AWS_REGION=%s", region))
	}
}

// resolveExitCode extracts the integer exit code from an error returned by
// ExecuteShellCommand.  Returns 0 when err is nil, 1 for generic (non-typed) errors.
func resolveExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ec errUtils.ExitCodeError
	if errors.As(err, &ec) {
		return ec.Code
	}
	var osErr *osexec.ExitError
	if errors.As(err, &osErr) {
		return osErr.ExitCode()
	}
	return 1
}

// executeMainTerraformCommand runs the final terraform sub-command.
// It handles exit-code extraction, plan-status upload (for --upload-status), and
// appropriate error propagation.  A no-op when info.SubCommand is "workspace" with
// no sub-subcommand (workspace listing was already handled by runWorkspaceSetup).
func executeMainTerraformCommand(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	allArgsAndFlags []string,
	componentPath string,
	uploadStatusFlag bool,
	opts ...ShellCommandOption,
) error {
	// Bare `workspace` (no sub-subcommand) was fully handled by runWorkspaceSetup.
	if info.SubCommand == "workspace" && info.SubCommand2 == "" {
		return nil
	}

	err := ExecuteShellCommand(
		*atmosConfig,
		info.Command,
		allArgsAndFlags,
		componentPath,
		info.ComponentEnvList,
		info.DryRun,
		info.RedirectStdErr,
		opts...,
	)

	exitCode := resolveExitCode(err)

	if uploadStatusFlag && shouldUploadStatus(info) {
		client, cerr := pro.NewAtmosProAPIClientFromEnv(atmosConfig)
		if cerr != nil {
			return cerr
		}
		gitRepo := &git.DefaultGitRepo{}
		if uerr := uploadStatus(info, exitCode, client, gitRepo); uerr != nil {
			return uerr
		}
		// Exit codes 0 and 2 are both "success" for plan uploads.
		if exitCode == 0 {
			return nil
		}
		if exitCode == 2 {
			return errUtils.ExitCodeError{Code: 2}
		}
	}

	return err
}

// cleanupTerraformFiles removes ephemeral plan and varfiles that Atmos generates.
// Failures are logged at Trace level and not propagated, since cleanup errors should
// not mask the result of the main command.
func cleanupTerraformFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) {
	if info.SubCommand != "plan" && info.SubCommand != "show" && info.PlanFile == "" {
		planFilePath := constructTerraformComponentPlanfilePath(atmosConfig, info)
		if err := os.Remove(planFilePath); err != nil && !os.IsNotExist(err) {
			log.Trace("Failed to remove plan file during cleanup", "error", err, "file", planFilePath)
		}
	}

	if info.SubCommand == "apply" {
		varFilePath := constructTerraformComponentVarfilePath(atmosConfig, info)
		if err := os.Remove(varFilePath); err != nil && !os.IsNotExist(err) {
			log.Trace("Failed to remove var file during cleanup", "error", err, "file", varFilePath)
		}
	}
}

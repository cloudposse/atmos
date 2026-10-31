package terraform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	auth "github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/policy/planfile"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/autoinit"
	"github.com/cloudposse/atmos/pkg/ui"
)

// componentExecContext holds the per-execution state assembled by prepareComponentExecution.
// It is consumed by executeCommandPipeline without further modification.
type componentExecContext struct {
	preparedConfiguration *schema.PreparedComponentConfiguration
	componentPath         string
	varFile               string
	testVarFile           string
	planFile              string
	workingDir            string
	tenv                  *dependencies.ToolchainEnvironment
	planPolicy            *planfile.Guard
}

// prepareComponentExecution consolidates all per-component setup into one call:
// path resolution, access checks, toolchain installation, variable file generation,
// OPA/JSON-schema validation, auth pre-hook, config file generation, and env assembly.
// Extracting this reduces ExecuteTerraform's cyclomatic complexity by ~10 decision points.
func (e *Executor) prepareComponentExecution(
	ctx context.Context,
	writers provisioner.OutputWriters,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	shouldProcess bool,
) (*componentExecContext, error) {
	if err := checkTerraformConfig(*atmosConfig); err != nil {
		return nil, err
	}

	tenv, err := e.prepareConfigurationEnvironment(ctx, atmosConfig, info, shouldProcess)
	if err != nil {
		return nil, err
	}
	prepared, err := prepareConfiguration(ctx, atmosConfig, info)
	if err != nil {
		return nil, err
	}

	var componentPath string
	if prepared != nil && usesConfigurationPlan(info) {
		// A reviewed plan uses the retained workdir; provisioning could replace its modules.
		componentPath = ConstructTerraformComponentWorkingDir(atmosConfig, info)
	} else {
		componentPath, err = e.resolveAndProvisionComponentPath(ctx, writers, atmosConfig, info)
		if err != nil {
			return nil, err
		}
	}

	if prepared != nil && !info.DryRun {
		if err := configuration.WriteFiles(componentPath, prepared.Files); err != nil {
			return nil, fmt.Errorf("component %s in stack %s, working directory %s: %w", info.ComponentFromArg, info.Stack, componentPath, err)
		}
	}

	if err = checkComponentRestrictions(info); err != nil {
		return nil, err
	}

	if shouldProcess && tenv == nil {
		tenv, err = resolveAndInstallToolchainDeps(atmosConfig, info)
		if err != nil {
			return nil, err
		}
		info.Command = tenv.Resolve(info.Command)
	}

	varFile := ConstructTerraformComponentVarfileName(info)
	testVarFile := ""
	if hasTerraformTestVars(info) {
		testVarFile = ConstructTerraformComponentTestVarfileName(info)
	}
	planFile := ConstructTerraformComponentPlanfileName(info)
	workingDir := ConstructTerraformComponentWorkingDir(atmosConfig, info)

	if err = e.runPreExecutionSteps(atmosConfig, info, workingDir, tenv); err != nil {
		return nil, err
	}

	return &componentExecContext{
		preparedConfiguration: prepared,
		componentPath:         componentPath,
		varFile:               varFile,
		testVarFile:           testVarFile,
		planFile:              planFile,
		workingDir:            workingDir,
		tenv:                  tenv,
	}, nil
}

// runPreExecutionSteps performs validation, auth pre-hook, config file generation,
// env var warnings, and env var assembly. Extracted from prepareComponentExecution
// to keep cyclomatic complexity within bounds.
func (e *Executor) runPreExecutionSteps(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	workingDir string,
	tenv *dependencies.ToolchainEnvironment,
) error {
	// `(computed)` is an inspection-only degradation marker. Never permit it to
	// reach Terraform via a generated tfvars file or TF_VAR_ environment value.
	if err := rejectComputedTerraformVars(info.ComponentVarsSection); err != nil {
		return err
	}

	// Partition variables into disk-safe vs. secret-bearing BEFORE writing the varfile or
	// assembling env vars (and before the auth pre-hook registers credentials with the
	// masker), so secrets are kept off disk and injected as TF_VAR_* env vars instead.
	ComputeTerraformSecretVarKeys(info)

	if err := printAndWriteVarFiles(atmosConfig, info); err != nil {
		return err
	}

	if err := e.validateTerraformComponent(atmosConfig, info); err != nil {
		return err
	}

	if _, owned := info.ComponentSettingsSection["configuration"]; !owned || info.DryRun {
		if err := auth.TerraformPreHook(atmosConfig, info); err != nil {
			// The error is returned and rendered once by the command boundary, so it is only logged
			// at debug level here to avoid printing the same failure twice.
			log.Debug("Error executing 'atmos auth terraform pre-hook'",
				logFieldComponent, info.ComponentFromArg, "error", err)
			// Pre-hook failures terminate execution — this matches the original terraform.go behavior.
			// Authentication setup failures must not silently produce unauthenticated terraform commands.
			return err
		}
		disableTerraformPluginCacheForExecution(atmosConfig, info)
	}

	// Generate backend and provider-override files. When AutoGenerateFiles=true,
	// GenerateFilesForComponent was already called inside resolveAndProvisionComponentPath
	// for generate: section files. This call handles the distinct backend/provider-override
	// responsibility.
	if err := e.GenerateConfigFiles(atmosConfig, info, workingDir); err != nil {
		return err
	}

	warnOnConflictingEnvVars()

	if _, owned := info.ComponentSettingsSection["configuration"]; owned && !info.DryRun {
		// Inputs have now been evaluated; append their secret variables after masking.
		return appendTerraformSecretEnv(info)
	}
	return assembleComponentEnvVars(atmosConfig, info, tenv)
}

// executeCommandPipeline runs the full terraform command pipeline after the component
// has been prepared: optional init pre-step, argument construction, workspace setup,
// TTY guard, and final command execution + cleanup.
// Extracting this reduces ExecuteTerraform's cyclomatic complexity by ~7 decision points.
// This builds the CI log-group label for a terraform/tofu phase,
// e.g. "terraform init" or "tofu plan". The command is the resolved executable
// (often an absolute toolchain path), so it is reduced to its base name.
func terraformPhaseLabel(info *schema.ConfigAndStacksInfo, phase string) string {
	tool := filepath.Base(strings.TrimSpace(info.Command))
	if tool == "" || tool == "." {
		tool = "terraform"
	}
	return strings.TrimSpace(tool + " " + phase)
}

func (e *Executor) executeCommandPipeline(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	execCtx *componentExecContext,
	opts ...component.ExecutionOption,
) error {
	componentPath := execCtx.componentPath

	if shouldRunTerraformInit(atmosConfig, info) {
		// Phase-level CI log grouping (Dimension "phase"): fold the `init` phase
		// into its own collapsible group. A no-op unless ci.groups.mode is "auto"
		// and this is the outermost Atmos invocation (a terraform run nested inside
		// a workflow step stays flat inside the step's group).
		err := ci.Group(atmosConfig, ci.DimensionPhase, terraformPhaseLabel(info, subcommandInit), func() error {
			var phaseErr error
			componentPath, phaseErr = e.executeTerraformInitPhase(atmosConfig, info, componentPath, execCtx.varFile, opts...)
			return phaseErr
		})
		if err != nil {
			return err
		}
	}

	handleDeploySubcommand(atmosConfig, info)
	logTerraformContext(info, execCtx.workingDir)
	addTerraformTestVarfileArg(info, execCtx.testVarFile)

	allArgsAndFlags, uploadStatusFlag, err := buildTerraformCommandArgs(atmosConfig, info, execCtx.varFile, execCtx.planFile, &componentPath, opts...)
	if err != nil {
		return err
	}

	if err = e.runWorkspaceSetupPhase(atmosConfig, info, componentPath, opts...); err != nil {
		return err
	}

	if err = checkTTYRequirement(info); err != nil {
		return err
	}

	addRegionEnvVarForImport(info)

	if shouldRunMainTerraformCommand(info) {
		if err = e.runMainCommandPhase(atmosConfig, info, execCtx, allArgsAndFlags, componentPath, uploadStatusFlag, opts...); err != nil {
			return err
		}
	}

	cleanupTerraformFiles(atmosConfig, info)
	return nil
}

// runMainCommandPhase runs the main plan/apply/destroy/… subcommand under its own CI phase
// group, capturing scoped exec-metadata output (FR-006f), and — on failure — gives smart init
// exactly one chance to recover via recoverFromInitRequired before propagating the error.
func (e *Executor) runMainCommandPhase( //nolint:revive // argument-limit: every parameter is load-bearing context passed straight through to recoverFromInitRequired.
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	execCtx *componentExecContext,
	allArgsAndFlags []string,
	componentPath string,
	uploadStatusFlag bool,
	opts ...component.ExecutionOption,
) error {
	// FR-006f: capture this invocation's own stdout/stderr into a buffer pair
	// scoped to ONLY the main command, in addition to (not instead of) whatever
	// WithStdoutCapture/WithStderrCapture already accumulate across the whole
	// init+workspace-select+main pipeline for other consumers (e.g.
	// cmd/terraform's capturedPlanOutput, used by CI job-summary hooks). Without
	// this, incidental init/workspace-select output (e.g. a stray "No changes."
	// lookalike) can poison the exec-metadata parser's extraction even though the
	// real plan/apply's own output is correct (research.md Decision 32).
	var execMetadataStdoutBuf, execMetadataStderrBuf bytes.Buffer
	mainOpts := append(slices.Clone(opts), component.WithMetadataOutputCapture(&execMetadataStdoutBuf, &execMetadataStderrBuf))

	// Phase-level CI log grouping (Dimension "phase"): fold the main subcommand
	// (plan/apply/destroy/…) into its own collapsible group, separate from init
	// and workspace setup.
	mainErr := ci.Group(atmosConfig, ci.DimensionPhase, terraformPhaseLabel(info, info.SubCommand), func() error {
		return e.executeGovernedTerraformCommand(atmosConfig, info, allArgsAndFlags, componentPath, uploadStatusFlag, execCtx.planPolicy, mainOpts...)
	})
	info.ExecMetadataRawOutput = combineExecMetadataOutput(&execMetadataStdoutBuf, &execMetadataStderrBuf)
	if mainErr == nil {
		return recordConfigurationPlan(component.ContextFromOptions(opts...), atmosConfig, info, execCtx, allArgsAndFlags)
	}

	return e.recoverFromInitRequired(atmosConfig, info, execCtx, allArgsAndFlags, componentPath, uploadStatusFlag,
		mainErr, &execMetadataStdoutBuf, &execMetadataStderrBuf, opts...)
}

// recoverFromInitRequired is smart init's plan/apply-time safety net: when the main command
// just failed, it classifies the captured output for a known "init is required" diagnostic
// (autoinit.Classify) and, if Atmos's own recovery policy says to (autoinit.ShouldRecover),
// forces exactly one `terraform init` re-run with the flag(s) the diagnostic asked for, then
// retries the main command exactly once more. This is the correction for a smart-init bet gone
// wrong: skipping init looked safe based on the fingerprint, but terraform/tofu itself now
// disagrees.
//
// No recovery is attempted for the init subcommand itself (there is no "init required" fallback
// for init) or when ShouldRecover reports a policy error (e.g. the user explicitly disabled
// implicit init) — that error is joined with the original failure and returned as-is.
func (e *Executor) recoverFromInitRequired( //nolint:revive // argument-limit: every parameter is load-bearing context from runMainCommandPhase.
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	execCtx *componentExecContext,
	allArgsAndFlags []string,
	componentPath string,
	uploadStatusFlag bool,
	mainErr error,
	stdoutBuf, stderrBuf *bytes.Buffer,
	opts ...component.ExecutionOption,
) error {
	if info.SubCommand == subcommandInit {
		return mainErr
	}

	combined := stdoutBuf.String() + "\n" + stderrBuf.String() + "\n" + mainErr.Error()
	rec, policyErr := classifyInitRecovery(atmosConfig, info, combined)
	if policyErr != nil {
		return errors.Join(mainErr, policyErr)
	}
	if !rec.Run {
		return mainErr
	}

	ui.Warning(fmt.Sprintf("Terraform requires initialization; running init and retrying '%s'", info.SubCommand))

	initErr := ci.Group(atmosConfig, ci.DimensionPhase, terraformPhaseLabel(info, subcommandInit), func() error {
		return e.executeTerraformInitForcedFn(atmosConfig, info, componentPath, execCtx.varFile, rec.WithReconfigure, rec.WithUpgrade, opts...)
	})
	if initErr != nil {
		return errors.Join(mainErr, initErr)
	}

	return e.retryMainCommand(atmosConfig, info, allArgsAndFlags, componentPath, uploadStatusFlag, execCtx.planPolicy, opts...)
}

// retryMainCommand re-runs the main subcommand once after a forced init recovery, resetting the
// exec-metadata capture so info.ExecMetadataRawOutput reflects only the retry's own output.
func (e *Executor) retryMainCommand( //nolint:revive // argument-limit: every parameter is load-bearing context passed straight through to executeMainTerraformCommandFn.
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	allArgsAndFlags []string,
	componentPath string,
	uploadStatusFlag bool,
	gate *planfile.Guard,
	opts ...component.ExecutionOption,
) error {
	var execMetadataStdoutBuf, execMetadataStderrBuf bytes.Buffer
	mainOpts := append(slices.Clone(opts), component.WithMetadataOutputCapture(&execMetadataStdoutBuf, &execMetadataStderrBuf))

	err := ci.Group(atmosConfig, ci.DimensionPhase, terraformPhaseLabel(info, info.SubCommand), func() error {
		if gate != nil {
			return e.executeGovernedTerraformCommand(atmosConfig, info, allArgsAndFlags, componentPath, uploadStatusFlag, gate, mainOpts...)
		}
		return e.executeMainTerraformCommandFn(atmosConfig, info, allArgsAndFlags, componentPath, uploadStatusFlag, mainOpts...)
	})
	info.ExecMetadataRawOutput = combineExecMetadataOutput(&execMetadataStdoutBuf, &execMetadataStderrBuf)
	return err
}

// runWorkspaceSetupPhase selects or creates the Terraform workspace under its own
// phase-level CI log group. The group is emitted only when workspace setup will
// actually run, avoiding empty groups for backends/subcommands that skip setup.
func (e *Executor) runWorkspaceSetupPhase(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string, opts ...component.ExecutionOption) error {
	if shouldSkipWorkspaceSetup(info) {
		return nil
	}
	var stdout, stderr bytes.Buffer
	captureOpts := append(slices.Clone(opts), component.WithMetadataOutputCapture(&stdout, &stderr))
	err := ci.Group(atmosConfig, ci.DimensionPhase, terraformPhaseLabel(info, subcommandWorkspace), func() error {
		return e.runWorkspaceSetup(atmosConfig, info, componentPath, captureOpts...)
	})
	if err == nil {
		return nil
	}
	// Workspace selection can load providers before plan ever runs. Apply the same narrow,
	// bounded recovery and opt-out policy here, preserving the originally requested workspace.
	tf := &atmosConfig.Components.Terraform
	return autoinit.Recover(autoinit.RecoverParams{
		Err: err, Output: combineExecMetadataOutput(&stdout, &stderr) + "\n" + err.Error(),
		Mode: tf.EffectiveInitMode(), Reconfigure: tf.EffectiveInitReconfigure(), Upgrade: tf.EffectiveInitUpgrade(),
		OptedOut: autoinit.OptedOut(atmosConfig, info),
		RunInit: func(rec autoinit.Recovery) error {
			return e.executeTerraformInitForcedFn(atmosConfig, info, componentPath,
				ConstructTerraformComponentVarfileName(info), rec.WithReconfigure, rec.WithUpgrade, opts...)
		},
		Retry: func() error { return e.runWorkspaceSetup(atmosConfig, info, componentPath, opts...) },
	})
}

func shouldRunMainTerraformCommand(info *schema.ConfigAndStacksInfo) bool {
	return info.SubCommand != subcommandWorkspace || info.SubCommand2 != ""
}

// combineExecMetadataOutput concatenates the main command's scoped stdout/stderr
// capture for the exec-metadata parser (FR-006f), mirroring the stdout+"\n"+stderr
// convention cmd/terraform's terraformExecMetadataParserFunc previously applied
// when it owned buffer construction directly.
func combineExecMetadataOutput(stdoutBuf, stderrBuf *bytes.Buffer) string {
	combined := stdoutBuf.String()
	if errOut := stderrBuf.String(); errOut != "" {
		combined += "\n" + errOut
	}
	return combined
}

func addTerraformTestVarfileArg(info *schema.ConfigAndStacksInfo, testVarFile string) {
	if info.SubCommand != "test" || testVarFile == "" {
		return
	}
	info.AdditionalArgsAndFlags = append([]string{varFileFlag, testVarFile}, info.AdditionalArgsAndFlags...)
}

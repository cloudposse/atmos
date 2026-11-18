package terraform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/component"
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
	// planSnapshots observes the configuration around the main command for plan binding.
	planSnapshots *planSnapshotter
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

	componentPath, err := e.resolvePreparedComponentPath(ctx, writers, atmosConfig, info, prepared)
	if err != nil {
		return nil, err
	}

	if err := writePreparedConfigurationFiles(info, prepared, componentPath); err != nil {
		return nil, err
	}

	if err = checkComponentRestrictions(info); err != nil {
		return nil, err
	}

	tenv, err = resolveComponentToolchain(atmosConfig, info, shouldProcess, tenv)
	if err != nil {
		return nil, err
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

	if !schema.ConfigurationOwned(info.ComponentSettingsSection) || info.DryRun {
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

	return assemblePreExecutionEnvVars(atmosConfig, info, tenv)
}

// assemblePreExecutionEnvVars keeps the prepared environment for configuration-owned
// execution; ordinary components and dry runs still assemble their full environment.
func assemblePreExecutionEnvVars(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, tenv *dependencies.ToolchainEnvironment) error {
	if schema.ConfigurationOwned(info.ComponentSettingsSection) && !info.DryRun {
		if err := checkConfigurationWorkspace(info); err != nil {
			return err
		}
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
	warnConfigurationDryRun(info)

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
			return errUtils.AsPrerequisiteError(err)
		}
	}

	handleDeploySubcommand(atmosConfig, info)
	logTerraformContext(info, execCtx.workingDir)
	addTerraformTestVarfileArg(info, execCtx.testVarFile)

	allArgsAndFlags, uploadStatusFlag, err := buildTerraformCommandArgs(atmosConfig, info, execCtx.varFile, execCtx.planFile, &componentPath, opts...)
	if err != nil {
		return err
	}

	// A missing required variable file must fail before workspace selection runs a subprocess.
	if err = preflightRequiredVarFiles(info, componentPath); err != nil {
		return err
	}

	if err = e.runWorkspaceSetupPhase(atmosConfig, info, componentPath, opts...); err != nil {
		return errUtils.AsPrerequisiteError(err)
	}

	if err = checkTTYRequirement(info, opts...); err != nil {
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
	// Init/provisioning can relocate the execution directory after preparation.
	execCtx.componentPath, execCtx.workingDir = componentPath, componentPath
	if err := validateEnvironmentPlanOutput(atmosConfig, info, execCtx); err != nil {
		return err
	}
	allArgsAndFlags, restore, err := expandPolicyPlanArguments(info, execCtx, allArgsAndFlags, componentPath)
	if err != nil {
		return err
	}
	defer restore()
	var execMetadataStdoutBuf, execMetadataStderrBuf bytes.Buffer
	var effectiveArgs []string
	captureCtx := context.WithValue(component.ContextFromOptions(opts...), mainCommandArgsKey{}, &effectiveArgs)
	captureCtx = execCtx.observePlanSnapshots(captureCtx, atmosConfig, info, allArgsAndFlags)
	captureArgs := component.WithProcessContext(captureCtx)
	// A component with provider-supplied hooks recovers inside its main-command lifecycle, so
	// the retry runs between the same before and after hooks instead of repeating them.
	recovery := &mainCommandRecovery{recover: func(mainErr error, retry func() error) error {
		return e.recoverWithinLifecycle(atmosConfig, info, execCtx, componentPath, mainErr,
			&execMetadataStdoutBuf, &execMetadataStderrBuf, retry, append(slices.Clone(opts), captureArgs)...)
	}}
	captureArgs = component.WithProcessContext(context.WithValue(captureCtx, mainCommandRecoveryKey{}, recovery))
	mainOpts := append(slices.Clone(opts), component.WithMetadataOutputCapture(&execMetadataStdoutBuf, &execMetadataStderrBuf), captureArgs)

	// Phase-level CI log grouping (Dimension "phase"): fold the main subcommand
	// (plan/apply/destroy/…) into its own collapsible group, separate from init
	// and workspace setup.
	mainErr := ci.Group(atmosConfig, ci.DimensionPhase, terraformPhaseLabel(info, info.SubCommand), func() error {
		return e.executeGovernedTerraformCommand(atmosConfig, info, allArgsAndFlags, componentPath, uploadStatusFlag, execCtx.planPolicy, mainOpts...)
	})
	info.ExecMetadataRawOutput = combineExecMetadataOutput(&execMetadataStdoutBuf, &execMetadataStderrBuf)
	if mainErr != nil && !lifecycleOwnsRecovery(info) {
		mainErr = e.recoverFromInitRequired(atmosConfig, info, execCtx, allArgsAndFlags, componentPath, uploadStatusFlag,
			mainErr, &execMetadataStdoutBuf, &execMetadataStderrBuf, append(slices.Clone(opts), captureArgs)...)
	}
	// An explicit -detailed-exitcode plan reports changes with status 2 after writing the
	// plan, so the reviewed plan is recorded before that status is returned.
	if mainErr != nil && !isExplicitDetailedPlanChanges(info, effectiveArgs, mainErr) {
		return mainErr
	}
	// Policy plans run against a private artifact that is removed after the guard
	// publishes it to the original output path. The effective arguments were
	// validated before execution; bind the published destination instead.
	if execCtx.planPolicy != nil && info.SubCommand == "plan" && !info.DryRun {
		effectiveArgs = allArgsAndFlags
	}
	if err := recordConfigurationPlan(component.ContextFromOptions(opts...), atmosConfig, info, execCtx, effectiveArgs); err != nil {
		return err
	}
	return mainErr
}

// planSnapshot is the configuration a saved plan is bound to: the provider identity and the
// digest of the retained working directory, excluding the plan and the state Terraform writes.
type planSnapshot struct {
	path, identity, workingDigest string
	// engineDigest is the digest of the Terraform executable observed around the subprocess, so a hook
	// that replaces the binary afterwards cannot move the binding onto the replacement. It is empty when
	// the executable could not be read, and the binding then falls back to its own observation.
	engineDigest string
	// planDigest is the digest of the plan file Terraform wrote, taken before any after hook runs. It is
	// empty when no digest was observed, and it is not part of the before-and-after comparison.
	planDigest string
}

// planSnapshotKey carries the plan snapshotter to the main command's subprocess.
type planSnapshotKey struct{}

// planSnapshotter observes the configuration around the Terraform subprocess, after the before
// hooks and before the after hooks. A hook that rewrites the module after Terraform read it then
// cannot move the binding onto the rewritten bytes.
type planSnapshotter struct {
	ctx      context.Context
	config   *schema.AtmosConfiguration
	info     *schema.ConfigAndStacksInfo
	execCtx  *componentExecContext
	pathFrom func(phaseArgs []string) string
	result   *planSnapshot
	err      error
}

// takePlanSnapshot fingerprints the configuration with the inputs the binding record uses.
func takePlanSnapshot(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, execCtx *componentExecContext, path string) (*planSnapshot, error) {
	runtime, err := planRuntime(config)
	if err != nil {
		return nil, err
	}
	identity, err := runtime.PlanIdentity(ctx, info, execCtx.preparedConfiguration.Sources)
	if err != nil {
		return nil, err
	}
	digest, err := configurationWorkdirDigest(execCtx.workingDir, path)
	if err != nil {
		return nil, err
	}
	engineDigest, _ := fileDigest(info.Command)
	return &planSnapshot{path: path, identity: identity, workingDigest: digest, engineDigest: engineDigest}, nil
}

// around wraps one run of the Terraform subprocess. The configuration must be the same before and
// after it; the unchanged snapshot is kept for the binding, and a changed one rejects the plan.
func (s *planSnapshotter) around(phaseArgs []string, run func() error) func() error {
	return func() error {
		s.result, s.err = nil, nil
		path := s.pathFrom(phaseArgs)
		if path == "" {
			return run()
		}
		before, err := takePlanSnapshot(s.ctx, s.config, s.info, s.execCtx, path)
		if err != nil {
			return err
		}
		runErr := run()
		after, err := takePlanSnapshot(s.ctx, s.config, s.info, s.execCtx, path)
		switch {
		case err != nil:
			s.err = err
		case *after != *before:
			s.err = fmt.Errorf("%w: configuration changed while Terraform ran", errUtils.ErrPlanVerificationFailed)
		default:
			// Hash the engine's actual -out artifact before after hooks, even when policy
			// later publishes it at a different path retained by the companion.
			writtenPath := path
			if s.execCtx.planPolicy != nil {
				writtenPath = plannedOutputPath(s.execCtx.componentPath, phaseArgs)
			}
			if digest, digestErr := fileDigest(writtenPath); digestErr == nil {
				after.planDigest = digest
			}
			s.result = after
		}
		return runErr
	}
}

// reviewedSnapshot returns the configuration Terraform read, or takes a fresh snapshot when no
// observed run recorded one for this plan.
func (c *componentExecContext) reviewedSnapshot(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, path string) (*planSnapshot, error) {
	if s := c.planSnapshots; s != nil {
		if s.err != nil {
			return nil, s.err
		}
		if s.result != nil && s.result.path == path {
			return s.result, nil
		}
	}
	return takePlanSnapshot(ctx, config, info, c, path)
}

// observePlanSnapshots returns ctx carrying a snapshotter for a plan this invocation binds. Policy
// plans publish the artifact at the original output path, so that path is the one excluded from the
// digest. The returned context is unchanged when no plan is bound.
func (c *componentExecContext) observePlanSnapshots(ctx context.Context, config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, policyArgs []string) context.Context {
	c.planSnapshots = nil
	if !bindsPlan(info, c) {
		return ctx
	}
	c.planSnapshots = &planSnapshotter{ctx: ctx, config: config, info: info, execCtx: c, pathFrom: func(phaseArgs []string) string {
		if c.planPolicy != nil {
			phaseArgs = policyArgs
		}
		return plannedOutputPath(c.componentPath, phaseArgs)
	}}
	return context.WithValue(ctx, planSnapshotKey{}, c.planSnapshots)
}

// isExplicitDetailedPlanChanges reports plan's "changes present" status from -detailed-exitcode.
// A failed hook is never that status, even when the hook itself exited with 2.
func isExplicitDetailedPlanChanges(info *schema.ConfigAndStacksInfo, args []string, err error) bool {
	if errors.Is(err, errUtils.ErrComponentHookFailed) {
		return false
	}
	return info.SubCommand == subcommandPlan && detailedExitCodeEnabled(args) && resolveExitCode(err) == detailedExitCodeChangesDetected
}

// detailedExitCodeEnabled reports whether args enable -detailed-exitcode. It accepts the bare flag
// and the explicit boolean forms (-detailed-exitcode=true, --detailed-exitcode=false), and the last
// occurrence wins, matching Go flag semantics. An unparsable value counts as not enabled.
func detailedExitCodeEnabled(args []string) bool {
	enabled := false
	for _, arg := range args {
		name, value, hasValue := strings.Cut(arg, "=")
		if name != detailedExitCodeFlag && name != "-"+detailedExitCodeFlag {
			continue
		}
		if !hasValue {
			enabled = true
			continue
		}
		parsed, err := strconv.ParseBool(value)
		enabled = err == nil && parsed
	}
	return enabled
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
	retry, err := e.runInitRecovery(atmosConfig, info, execCtx, componentPath, mainErr, stdoutBuf, stderrBuf, opts...)
	if !retry {
		return err
	}
	return e.retryMainCommand(atmosConfig, info, allArgsAndFlags, componentPath, uploadStatusFlag, execCtx.planPolicy, opts...)
}

// runInitRecovery classifies a failed main command and, when smart init's recovery policy
// allows it, runs the forced init the diagnostic asked for. It reports whether the main command
// should now be retried; otherwise it returns the error to propagate.
func (e *Executor) runInitRecovery( //nolint:revive // argument-limit: every parameter is load-bearing context from the recovering caller.
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	execCtx *componentExecContext,
	componentPath string,
	mainErr error,
	stdoutBuf, stderrBuf *bytes.Buffer,
	opts ...component.ExecutionOption,
) (bool, error) {
	if info.SubCommand == subcommandInit {
		return false, mainErr
	}

	combined := stdoutBuf.String() + "\n" + stderrBuf.String() + "\n" + mainErr.Error()
	rec, policyErr := classifyInitRecovery(atmosConfig, info, combined)
	if policyErr != nil {
		return false, errors.Join(mainErr, policyErr)
	}
	if !rec.Run {
		return false, mainErr
	}

	ui.Warning(fmt.Sprintf("Terraform requires initialization; running init and retrying '%s'", info.SubCommand))

	initErr := ci.Group(atmosConfig, ci.DimensionPhase, terraformPhaseLabel(info, subcommandInit), func() error {
		return e.executeTerraformInitForcedFn(atmosConfig, info, componentPath, execCtx.varFile, rec.WithReconfigure, rec.WithUpgrade, opts...)
	})
	if initErr != nil {
		return false, errors.Join(mainErr, initErr)
	}
	return true, nil
}

// recoverWithinLifecycle is recoverFromInitRequired for a main command wrapped by provider
// hooks. It runs inside that command's lifecycle and retries only the Terraform subprocess, so
// before and after hooks run once for the user command. The exec-metadata capture is reset so
// it reflects only the retry's own output, as retryMainCommand does.
func (e *Executor) recoverWithinLifecycle( //nolint:revive // argument-limit: every parameter is load-bearing context from runMainCommandPhase.
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	execCtx *componentExecContext,
	componentPath string,
	mainErr error,
	stdoutBuf, stderrBuf *bytes.Buffer,
	retry func() error,
	opts ...component.ExecutionOption,
) error {
	again, err := e.runInitRecovery(atmosConfig, info, execCtx, componentPath, mainErr, stdoutBuf, stderrBuf, opts...)
	if !again {
		return err
	}
	stdoutBuf.Reset()
	stderrBuf.Reset()
	return retry()
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

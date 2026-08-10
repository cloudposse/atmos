package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"mvdan.cc/sh/v3/shell"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	w "github.com/cloudposse/atmos/internal/tui/workflow"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/background"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/dependencies"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/runner/freshness"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/taskgraph"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
	workflowPkg "github.com/cloudposse/atmos/pkg/workflow"
)

// Workflow error title for formatted output.
const WorkflowErrTitle = "Workflow Error"

// workflowTemplatePasses is the number of template render passes the workflow
// step executor uses, matching the custom command step path (cmd_utils.go) so
// multi-level templates resolve identically in both.
const workflowTemplatePasses = 3

// bgRunIDLen is the length of the short per-run id used to scope background container
// instance names when no explicit `--stack` is given.
const bgRunIDLen = 8

// logKeyStep is the structured-log field key used for a workflow step's identity.
const logKeyStep = "step"

// Local errors not in shared package (workflow-specific internal errors).
var (
	ErrNoWorkflowFilesToSelect = errors.New("no workflow files to select from")
	ErrNonTTYWorkflowSelection = errors.New("interactive workflow selection not available in non-TTY or CI environments")
)

// KnownWorkflowErrors contains all known workflow sentinel errors for error handling.
var KnownWorkflowErrors = []error{
	errUtils.ErrWorkflowNoSteps,
	errUtils.ErrInvalidWorkflowStepType,
	errUtils.ErrInvalidFromStep,
	errUtils.ErrWorkflowStepFailed,
	errUtils.ErrWorkflowNoWorkflow,
	errUtils.ErrWorkflowFileNotFound,
	errUtils.ErrInvalidWorkflowManifest,
}

// workflowStepErrorContext contains context needed to build workflow step errors.
type workflowStepErrorContext struct {
	WorkflowPath     string
	WorkflowBasePath string
	Workflow         string
	StepName         string
	Command          string
	CommandType      string
	FinalStack       string
}

// buildWorkflowStepError builds an error with resume hints when a workflow step fails.
func buildWorkflowStepError(err error, ctx *workflowStepErrorContext) error {
	log.Debug("Workflow failed", "error", err)

	// Remove the workflow base path, stacks/workflows.
	workflowFileName := strings.TrimPrefix(filepath.ToSlash(ctx.WorkflowPath), filepath.ToSlash(ctx.WorkflowBasePath))
	// Remove the leading slash.
	workflowFileName = strings.TrimPrefix(workflowFileName, "/")
	// Remove the file extension.
	workflowFileName = strings.TrimSuffix(workflowFileName, filepath.Ext(workflowFileName))

	resumeCommand := fmt.Sprintf(
		"%s workflow %s -f %s --from-step '%s'",
		config.AtmosCommand,
		ctx.Workflow,
		workflowFileName,
		ctx.StepName,
	)

	// Add stack parameter to resume command if a stack was used.
	if ctx.FinalStack != "" {
		resumeCommand = fmt.Sprintf("%s -s '%s'", resumeCommand, ctx.FinalStack)
	}

	failedCmd := ctx.Command
	if ctx.CommandType == config.AtmosCommand {
		failedCmd = config.AtmosCommand + " " + ctx.Command
		// Add stack parameter to failed command if a stack was used.
		if ctx.FinalStack != "" {
			failedCmd = fmt.Sprintf("%s -s '%s'", failedCmd, ctx.FinalStack)
		}
	}

	// Build error with context about the failed command.
	// Use fmt.Errorf with %w to wrap the underlying error while adding ErrWorkflowStepFailed to the chain.
	// This preserves both the error sentinel for errors.Is() checks and the underlying error's exit code.
	wrappedErr := fmt.Errorf("%w: %w", errUtils.ErrWorkflowStepFailed, err)

	// Now build the error with explanation and hints using the wrapped error.
	// This preserves the error chain while adding formatted context.
	// Commands are wrapped in code fences for proper formatting and copy-paste.
	// Single quotes are used for shell safety (step names and stacks can contain spaces).
	builder := errUtils.Build(wrappedErr).
		WithTitle("Workflow Error").
		WithExplanationf("The following command failed to execute:\n\n```shell\n%s\n```", failedCmd).
		WithHintf("To resume the workflow from this step, run:\n\n```shell\n%s\n```", resumeCommand)

	// Extract exit code from the underlying error if available.
	if exitCode := errUtils.GetExitCode(err); exitCode != 0 {
		builder = builder.WithExitCode(exitCode)
	}

	return builder.Err()
}

// prepareStepEnvironment prepares environment variables for a workflow step.
// baseEnv should already contain system env + global env + toolchain PATH.
// This function merges workflow env, persistent env-step values, and step env on
// top, then handles auth if needed.
// Returns the environment variables to use for the step.
func prepareStepEnvironment(
	baseEnv []string,
	stepIdentity string,
	stepName string,
	authManager auth.AuthManager,
	workflowEnvMap map[string]string,
	persistentEnvMap map[string]string,
	stepEnvMap map[string]string,
) ([]string, error) {
	// Make a copy of baseEnv to avoid modifying the caller's slice.
	stepEnv := make([]string, len(baseEnv))
	copy(stepEnv, baseEnv)

	// Merge workflow, persistent env-step, and step env vars into a single map.
	// Later layers take precedence, so a current step's env can override a value
	// established by an earlier env step.
	// This ensures duplicate keys are resolved before adding to the environment.
	mergedEnv := make(map[string]string, len(workflowEnvMap)+len(persistentEnvMap)+len(stepEnvMap))
	for k, v := range workflowEnvMap {
		mergedEnv[k] = v
	}
	for k, v := range persistentEnvMap {
		mergedEnv[k] = v
	}
	for k, v := range stepEnvMap {
		mergedEnv[k] = v
	}
	if pathOverride, ok := mergedEnv["PATH"]; ok {
		// Workflow templates commonly extend PATH with the process PATH, for example
		// `PATH: /workspace/.context/bin:{{ env "PATH" }}`. At this point baseEnv
		// has already added the workflow toolchain directories, so replacing PATH
		// would otherwise make declared tools unavailable to the step.
		mergedEnv["PATH"] = mergeWorkflowPath(pathOverride, lastEnvironmentValue(baseEnv, "PATH"))
	}
	if len(mergedEnv) > 0 {
		stepEnv = append(stepEnv, envpkg.ConvertMapToSlice(mergedEnv)...)
	}

	// No identity specified, use base environment (system + global + toolchain + workflow + persistent + step env).
	if stepIdentity == "" {
		return stepEnv, nil
	}

	if authManager == nil {
		return nil, errUtils.Build(errUtils.ErrAuthManager).
			WithExplanation("auth manager is not initialized").
			WithContext("identity", stepIdentity).
			WithContext(logKeyStep, stepName).
			Err()
	}

	ctx := context.Background()

	// Try to use cached credentials first (passive check, no prompts).
	// Only authenticate if cached credentials are not available or expired.
	if _, err := authManager.GetCachedCredentials(ctx, stepIdentity); err != nil {
		log.Debug("No valid cached credentials found, authenticating", "identity", stepIdentity, "error", err)
		// No valid cached credentials - perform full authentication.
		if _, err = authManager.Authenticate(ctx, stepIdentity); err != nil {
			// Check for user cancellation - return clean error without wrapping.
			if errors.Is(err, errUtils.ErrUserAborted) {
				return nil, errUtils.ErrUserAborted
			}
			return nil, fmt.Errorf("%w for identity %q in step %q: %w", errUtils.ErrAuthenticationFailed, stepIdentity, stepName, err)
		}
	}

	// Prepare shell environment with authentication credentials.
	// Pass stepEnv (system + global + toolchain + workflow + step) to let auth configure credentials.
	authEnv, authErr := authManager.PrepareShellEnvironment(ctx, stepIdentity, stepEnv)
	if authErr != nil {
		return nil, fmt.Errorf("%w: failed to prepare shell environment for identity %q in step %q: %w", errUtils.ErrAuthenticationFailed, stepIdentity, stepName, authErr)
	}
	stepEnv = authEnv

	log.Debug("Prepared environment with identity", "identity", stepIdentity, logKeyStep, stepName)
	return stepEnv, nil
}

// lastEnvironmentValue returns the effective value for key in env, where later
// entries take precedence. This matches the environment semantics used for
// subprocesses and lets a toolchain PATH override the inherited system PATH.
func lastEnvironmentValue(env []string, key string) string {
	var value string
	for _, entry := range env {
		entryKey, entryValue, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(entryKey, key) {
			value = entryValue
		}
	}
	return value
}

// mergeWorkflowPath combines a workflow PATH override with the already
// toolchain-augmented PATH. When both values share a suffix, that suffix is
// retained once and toolchain directories are placed after the custom prefix.
func mergeWorkflowPath(overridePath string, toolchainPath string) string {
	if overridePath == "" || toolchainPath == "" {
		return overridePath
	}

	separator := string(os.PathListSeparator)
	overrideEntries := strings.Split(overridePath, separator)
	toolchainEntries := strings.Split(toolchainPath, separator)

	commonSuffix := 0
	for commonSuffix < len(overrideEntries) && commonSuffix < len(toolchainEntries) {
		overrideEntry := overrideEntries[len(overrideEntries)-1-commonSuffix]
		toolchainEntry := toolchainEntries[len(toolchainEntries)-1-commonSuffix]
		if overrideEntry != toolchainEntry {
			break
		}
		commonSuffix++
	}

	merged := make([]string, 0, len(overrideEntries)+len(toolchainEntries)-commonSuffix)
	merged = append(merged, overrideEntries[:len(overrideEntries)-commonSuffix]...)
	merged = append(merged, toolchainEntries[:len(toolchainEntries)-commonSuffix]...)
	merged = append(merged, overrideEntries[len(overrideEntries)-commonSuffix:]...)
	return strings.Join(merged, separator)
}

// IsKnownWorkflowError returns true if the error matches any known workflow error.
// This includes ExitCodeError which indicates a subcommand failure that's already been reported.
func IsKnownWorkflowError(err error) bool {
	// Check if it's an ExitCodeError - these are already reported by the subcommand
	var exitCodeErr errUtils.ExitCodeError
	if errors.As(err, &exitCodeErr) {
		return true
	}

	// Check known workflow errors
	for _, knownErr := range KnownWorkflowErrors {
		if errors.Is(err, knownErr) {
			return true
		}
	}
	return false
}

// checkAndMergeDefaultIdentity checks if there's a default identity configured in atmos.yaml or stack configs.
// If a default identity is found in stack configs, it merges it into atmosConfig.Auth.
// Stack defaults take precedence over atmos.yaml defaults (following Atmos inheritance model).
// Returns true if a default identity exists after merging.
func checkAndMergeDefaultIdentity(atmosConfig *schema.AtmosConfiguration) bool {
	if len(atmosConfig.Auth.Identities) == 0 {
		return false
	}

	// Always load stack configs - stack defaults take precedence over atmos.yaml.
	stackDefaults, err := config.LoadStackAuthDefaults(atmosConfig)
	if err != nil {
		// On error, fall back to checking atmos.yaml defaults.
		for _, identity := range atmosConfig.Auth.Identities {
			if identity.Default {
				return true
			}
		}
		return false
	}

	// Merge stack defaults into auth config (stack takes precedence).
	if len(stackDefaults) > 0 {
		config.MergeStackAuthDefaults(&atmosConfig.Auth, stackDefaults)
	}

	// Check if we have a default after merging.
	for _, identity := range atmosConfig.Auth.Identities {
		if identity.Default {
			return true
		}
	}

	return false
}

// workflowCommandFilters carries optional, out-of-band ExecuteWorkflow invocation parameters
// that don't warrant their own required arguments: tags/labels filtering, and (see
// dependenciesResolved) whether this invocation is itself a dependency dispatch.
type workflowCommandFilters struct {
	tags   []string
	labels string
	// dependenciesResolved marks this ExecuteWorkflow call as already running inside another
	// workflow's dependency graph (see WorkflowRunner), so ExecuteWorkflow must skip resolving
	// and running its OWN dependencies.workflows/dependencies.commands again. Without this, a
	// workflow depended on by another workflow would have its dependency graph built and
	// executed twice: once by the parent's taskgraph.Run (which already discovers and runs the
	// full transitive closure through WorkflowLookup), and again here, redundantly -- and a
	// cycle reachable only through this workflow's own dependencies would be checked against a
	// second, independent graph the parent's cycle detection never sees, rather than being
	// caught once, up front, as part of the parent's single full-closure graph build.
	dependenciesResolved bool
}

// ExecuteWorkflow executes an Atmos workflow.
func ExecuteWorkflow(
	atmosConfig schema.AtmosConfiguration,
	workflow string,
	workflowPath string,
	workflowDefinition *schema.WorkflowDefinition,
	dryRun bool,
	commandLineStack string,
	fromStep string,
	commandLineIdentity string,
	commandLineFilters ...workflowCommandFilters,
) (retErr error) {
	defer perf.Track(&atmosConfig, "exec.ExecuteWorkflow")()
	commandFilters := workflowCommandFilters{}
	if len(commandLineFilters) > 0 {
		commandFilters = commandLineFilters[0]
	}
	var activeContainer *workflowPkg.ContainerSession
	defer func() {
		if activeContainer == nil {
			return
		}
		if cleanupErr := activeContainer.Cleanup(retErr == nil); cleanupErr != nil && retErr == nil {
			retErr = cleanupErr
		}
	}()

	// Reset step executor state at the start of each workflow to ensure clean variable scope.
	ResetStepExecutorState()

	// Initialize step executor with stage count for stage step type.
	initStepExecutorWithStages(workflowDefinition)

	// Resolve inline workflow step commands and env through the same template
	// engine custom command steps use (full Atmos renderer + multi-pass), so
	// {{ .steps.* }} / {{ .env.* }} / {{ .flags.* }} and template functions
	// behave identically in both. Flags are protected so a flag value containing
	// template markers is not re-evaluated on later passes (mirrors cmd_utils).
	workflowVars := stepExecutorState.Variables()
	workflowVars.SetTemplateRenderer(func(name, input string, data any) (string, error) {
		return ProcessTmpl(&atmosConfig, name, input, data, false)
	})
	workflowVars.SetTemplatePasses(workflowTemplatePasses)
	workflowVars.ProtectTemplateRoots("Flags", "flags")

	// Evaluate value-producing YAML functions (!env, !exec) in interactive step
	// fields (default/prompt/placeholder/options). Workflow manifests are parsed
	// with UnmarshalYAML, which leaves these as literal "!env ..." strings; this
	// lets interactive steps source defaults from the environment in CI.
	if err := resolveWorkflowStepFunctions(&atmosConfig, workflowDefinition); err != nil {
		return err
	}

	steps := workflowDefinition.Steps

	if len(steps) == 0 {
		return errUtils.Build(errUtils.ErrWorkflowNoSteps).
			WithTitle(WorkflowErrTitle).
			WithExplanationf("Workflow `%s` is empty and requires at least one step to execute.", workflow).
			WithContext("workflow", workflow).
			WithExitCode(1).
			Err()
	}

	// Check if the workflow steps have the `name` attribute
	checkAndGenerateWorkflowStepNames(workflowDefinition)

	// Background container services started by `background: true` steps are tracked in
	// a run-scoped registry. runCtx propagates cancellation (Ctrl-C / step failure) to
	// readiness waits and teardown. Any service still running when the workflow ends —
	// or when it exits early on error — is auto-torn-down here (implicit, since a service
	// never exits on its own); an explicit `cancel` step removes it from the registry first.
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	bgRegistry := background.NewRegistry()
	// bgGated records background steps that have already passed their readiness gate,
	// so the implicit gate before each foreground step does not re-probe them.
	bgGated := map[string]bool{}
	// Scope background container instance names per run. An explicit `--stack` is honored
	// verbatim (override path); otherwise use a run-specific id rather than the shared
	// workflow/stack name, so concurrent executions of the same workflow do not collide
	// on the same container.
	bgStack := commandLineStack
	if bgStack == "" {
		bgStack = "run-" + uuid.NewString()[:bgRunIDLen]
	}
	bgRunner := &workflowPkg.ContainerRunner{Stack: bgStack, DryRun: dryRun}
	defer func() {
		if stopErr := bgRegistry.StopAll(runCtx); stopErr != nil {
			retErr = errors.Join(retErr, stopErr)
		}
	}()

	// Validate exec steps before executing anything: an exec step replaces
	// the Atmos process, so it must be the final step and must not set
	// supervisor-only fields (tty, interactive, retry, timeout, output).
	if err := schema.ValidateWorkflowSteps(workflowDefinition.Steps); err != nil {
		return errUtils.Build(err).
			WithTitle(WorkflowErrTitle).
			WithHint("Check workflow step type, nested steps, needs dependencies, and control-step output/fail configuration").
			WithContext("workflow", workflow).
			WithExitCode(1).
			Err()
	}

	log.Debug("Executing workflow", "workflow", workflow, "path", workflowPath)

	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		err := u.PrintAsYAMLToFileDescriptor(&atmosConfig, workflowDefinition)
		if err != nil {
			return err
		}
	}

	// If `--from-step` is specified, skip all the previous steps
	if fromStep != "" {
		steps = lo.DropWhile[schema.WorkflowStep](steps, func(step schema.WorkflowStep) bool {
			return step.Name != fromStep
		})

		if len(steps) == 0 {
			stepNames := lo.Map(workflowDefinition.Steps, func(step schema.WorkflowStep, _ int) string { return step.Name })
			return errUtils.Build(errUtils.ErrInvalidFromStep).
				WithTitle(WorkflowErrTitle).
				WithExplanationf("The `--from-step` flag was set to `%s`, but this step does not exist in workflow `%s`.\n\n### Available steps:\n\n%s", fromStep, workflow, u.FormatList(stepNames)).
				WithContext("from_step", fromStep).
				WithContext("workflow", workflow).
				WithExitCode(1).
				Err()
		}
	}

	// Ensure toolchain dependencies are installed and build PATH for workflow steps.
	tenv, err := dependencies.ForWorkflow(&atmosConfig, workflowDefinition)
	if err != nil {
		return err
	}

	// Create auth manager if any runnable step has an identity or if command-line identity is specified.
	// We check once upfront to avoid repeated initialization.
	var authManager auth.AuthManager
	var authStackInfo *schema.ConfigAndStacksInfo
	needsAuth := false
	for i := range steps {
		step := &steps[i]
		if err := schema.ValidateStepCondition(step.When); err != nil {
			return err
		}
		// A step whose effective `when:` references a freshness fact can't be decided here: no
		// freshness.Checker has run yet, so those facts are all zero-value (always "unchanged"),
		// even on a step's very first-ever run. Treat it as possibly-runnable instead of skipping
		// it via `continue`, matching cmd.executeCustomCommand's identical fix for the same
		// empty-Context short-circuit.
		declared := freshness.StepDeclarations{Inputs: step.Inputs, Artifacts: step.Artifacts, Preconditions: step.Preconditions}
		effective := freshness.EffectiveWhen(step.When, declared)
		runs := freshness.MentionsAnyFreshnessFact(effective)
		if !runs {
			var err error
			runs, err = step.When.EvaluateWithImplicitSuccessE(workflowPkg.BuildConditionContext(workflow, workflowDefinition, step, commandLineStack, workflowDefinition.Env))
			if err != nil {
				return err
			}
		}
		if !runs {
			continue
		}
		if commandLineIdentity != "" || strings.TrimSpace(step.Identity) != "" {
			needsAuth = true
			break
		}
	}
	if needsAuth {
		// Create a ConfigAndStacksInfo for the auth manager to populate with AuthContext.
		// This enables YAML template functions to access authenticated credentials.
		authStackInfo = &schema.ConfigAndStacksInfo{
			AuthContext: &schema.AuthContext{},
		}

		credStore := credentials.NewCredentialStoreWithConfig(&atmosConfig.Auth)
		validator := validation.NewValidator()
		var err error
		authManager, err = auth.NewAuthManager(&atmosConfig.Auth, credStore, validator, authStackInfo, atmosConfig.CliConfigPath)
		if err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err)
		}
	}

	// Resolve and run dependencies.commands/dependencies.workflows before any of this
	// workflow's own steps, concurrently by default via pkg/taskgraph's DAG scheduler. Skipped
	// when this workflow is itself being invoked as someone else's dependency (see
	// workflowCommandFilters.dependenciesResolved) -- the caller's taskgraph run already
	// discovered and satisfied these as part of its own full-transitive-closure graph.
	if direct := taskgraph.RefsFromDependencies(workflowDefinition.Dependencies.OrEmpty()); len(direct) > 0 && !commandFilters.dependenciesResolved {
		if err := taskgraph.Run(
			context.Background(), direct,
			taskgraph.WithWorkflowRunner(WorkflowRunner(&atmosConfig, workflowPath, dryRun, commandLineIdentity)),
			taskgraph.WithWorkflowLookup(WorkflowLookup(&atmosConfig, workflowPath)),
			taskgraph.WithCommandRunner(commandRunnerViaSubprocess(&atmosConfig)),
			taskgraph.WithCommandLookup(CommandLookup(&atmosConfig)),
		); err != nil {
			return err
		}
	}

	// Construct base environment once: system env + global env + toolchain PATH.
	// This is reused for all steps, with workflow/step env vars merged on top per step.
	baseEnv := envpkg.MergeGlobalEnv(os.Environ(), atmosConfig.Env)
	baseEnv = append(baseEnv, tenv.EnvVars()...)
	persistentEnv := make(map[string]string)

	// Freshness checker for steps' `inputs:` (sources/generates/check), shared across all
	// steps in this workflow run. State persists under a project-relative directory so it
	// composes with ci.cache.includes: for cross-CI-run persistence.
	freshnessChecker := freshness.NewChecker()
	freshnessStateDir := freshness.StateDir(atmosConfig.BasePath)
	freshnessScope := "workflow:" + workflowPath + ":" + workflow

	// Initialize show renderer for header/flags display.
	showRenderer := workflowPkg.NewShowRenderer()

	// Build flags map for header display.
	flags := buildWorkflowFlagsMap(commandLineStack, commandLineIdentity, dryRun, fromStep)

	// Initialize progress renderer if enabled.
	totalSteps := len(steps)
	progressRenderer := workflowPkg.NewProgressRenderer(workflowDefinition, totalSteps)

	// Render header before first step (if enabled).
	showRenderer.RenderHeaderIfNeeded(workflowDefinition, workflow, flags)

	var workflowErr error
	conditionStatus := schema.ConditionPredicateSuccess
	for stepIdx, step := range steps {
		// Resolved ahead of the when: check (rather than where it's used further below) since
		// the freshness checker needs it to resolve inputs.sources/artifacts.paths relative to
		// the step's own working directory, not process CWD.
		stepWorkDir := workflowPkg.CalculateWorkingDirectory(workflowDefinition, &step, atmosConfig.BasePath)
		if stepWorkDir == "" {
			stepWorkDir = "."
		}

		conditionContext := workflowPkg.BuildConditionContext(workflow, workflowDefinition, &step, commandLineStack, workflowDefinition.Env)
		conditionContext.Status = conditionStatus
		declared := freshness.StepDeclarations{Inputs: step.Inputs, Artifacts: step.Artifacts, Preconditions: step.Preconditions}
		effectiveWhen := freshness.EffectiveWhen(step.When, declared)
		if step.Inputs != nil || step.Artifacts != nil || step.Preconditions != nil {
			id := freshness.StepIdentity{BaseDir: stepWorkDir, StateDir: freshnessStateDir, Scope: freshnessScope, StepName: step.Name}
			facts, factsErr := freshnessChecker.Compute(effectiveWhen, declared, id)
			if factsErr != nil {
				return factsErr
			}
			conditionContext.ChecksumChanged = facts.ChecksumChanged
			conditionContext.TimestampChanged = facts.TimestampChanged
			conditionContext.PreconditionsSuccess = facts.PreconditionsSuccess
			conditionContext.Sources = facts.Sources
			conditionContext.Artifacts = facts.Artifacts
		}
		runs, err := effectiveWhen.EvaluateWithImplicitSuccessE(conditionContext)
		if err != nil {
			return err
		}
		if !runs {
			log.Debug("Skipping workflow step, `when` condition did not match", logKeyStep, step.Name)
			continue
		}
		// Render step label with optional count prefix and progress bar.
		// When progress is enabled, combine label + progress on a single line (no newline).
		// When progress is disabled, only show the label if show.count is enabled; otherwise
		// emit nothing so default output stays backward compatible (show features are opt-in).
		showCfg := stepPkg.GetShowConfig(&step, workflowDefinition)
		label := stepPkg.FormatStepLabel(&step, workflowDefinition, stepIdx, totalSteps)
		if progressRenderer.IsEnabled() {
			progressRenderer.Update(stepIdx+1, step.Name)
			progressRenderer.RenderWithLabel(label) // No newline - will be cleared.
		} else if stepPkg.ShowCount(showCfg) {
			ui.Writeln(label)
		}

		command := strings.TrimSpace(step.Command)
		commandType := strings.TrimSpace(step.Type)
		stepIdentity := strings.TrimSpace(step.Identity)
		workflowStack := strings.TrimSpace(workflowDefinition.Stack)
		stepStack := strings.TrimSpace(step.Stack)
		finalStack := ""

		// The workflow `stack` attribute overrides the stack in the `command` (if specified).
		// The step `stack` attribute overrides the stack in the `command` and the workflow `stack` attribute.
		// The stack defined on the command line has the highest priority.
		if workflowStack != "" {
			finalStack = workflowStack
		}
		if stepStack != "" {
			finalStack = stepStack
		}
		if commandLineStack != "" {
			finalStack = commandLineStack
		}

		// If step doesn't specify identity, use command-line identity (if provided).
		if stepIdentity == "" && commandLineIdentity != "" {
			stepIdentity = commandLineIdentity
		}

		log.Debug("Executing workflow step", logKeyStep, stepIdx, "name", step.Name, "command", command)

		if commandType == "" {
			commandType = "atmos"
		}

		// Resolve step-variable templates in workflow/step env values (parity with
		// custom command steps) so a value like `X: "{{ .steps.select.value }}"`
		// is populated before it reaches the subprocess.
		resolvedWorkflowEnv, resolvedStepEnv, err := resolveWorkflowStepEnvs(workflowDefinition.Env, step.Env, baseEnv)
		if err != nil {
			if workflowErr == nil {
				workflowErr = err
			} else {
				workflowErr = errors.Join(workflowErr, err)
			}
			conditionStatus = schema.ConditionPredicateFailure
			continue
		}

		// Prepare environment variables: start with baseEnv (system + global + toolchain).
		// Then merge workflow-level, persistent env-step, and step-level env vars.
		// If identity is specified, also authenticate and add credentials.
		stepEnv, err := prepareStepEnvironment(baseEnv, stepIdentity, step.Name, authManager, resolvedWorkflowEnv, persistentEnv, resolvedStepEnv)
		if err != nil {
			if workflowErr == nil {
				workflowErr = err
			} else {
				workflowErr = errors.Join(workflowErr, err)
			}
			conditionStatus = schema.ConditionPredicateFailure
			continue
		}
		workDir := stepWorkDir

		// Clear progress line and re-render as permanent record before step execution.
		// This ensures progress line appears as header, then step output below it.
		if progressRenderer.IsEnabled() {
			ui.ClearLine()
			progressRenderer.RenderPermanent(label)
		}

		// Reject unknown step types before opening a log group so the precise
		// validation error is returned directly (not wrapped as a step failure).
		if commandType != "shell" &&
			commandType != schema.TaskTypeExec &&
			commandType != "atmos" &&
			commandType != schema.TaskTypeWait &&
			commandType != schema.TaskTypeWaitAll &&
			commandType != schema.TaskTypeCancel &&
			commandType != schema.TaskTypeParallel &&
			commandType != schema.TaskTypeMatrix &&
			!stepPkg.IsExtendedStepType(commandType) {
			return errUtils.Build(errUtils.ErrInvalidWorkflowStepType).
				WithTitle(WorkflowErrTitle).
				WithExplanationf("Workflow `%s` step `%s` uses unsupported type `%s`.", workflow, step.Name, commandType).
				WithContext("workflow", workflow).
				WithContext(logKeyStep, step.Name).
				WithHintf("Step type '%s' is not supported", commandType).
				WithHint("Each step must specify a valid type: 'atmos', 'shell', 'script', 'exec', or an interactive type like 'input', 'confirm', 'choose'").
				WithExitCode(1).
				Err()
		}

		// Resolve step-variable templates ({{ .steps.* }} / {{ .env.* }} /
		// {{ .flags.* }}) in inline command-bearing steps (shell/atmos/exec) so a
		// value captured by an earlier step reaches the command — parity with
		// custom command steps.
		if workflowCommandSupportsTemplating(commandType) {
			resolvedCommand, resolveErr := resolveWorkflowStepCommand(command, stepEnv)
			if resolveErr != nil {
				// errors.Join ignores a nil left operand, so this both starts and
				// accumulates the workflow error without an extra nil check.
				workflowErr = errors.Join(workflowErr, resolveErr)
				conditionStatus = schema.ConditionPredicateFailure
				continue
			}
			command = resolvedCommand
		}

		// If this step will be enclosed in a CI log group, mark the subprocess
		// environment so a nested `atmos` invocation skips unsupported nested
		// grouping.
		if commandType != schema.TaskTypeExec && ci.ShouldPropagateLogGroupSentinel(&atmosConfig, ci.DimensionStep) {
			stepEnv = append(stepEnv, ci.LogGroupSentinelEnv())
		}

		var commandResult *stepPkg.StepResult
		runCommandStep := func(run func(stdout, stderr io.Writer) error) error {
			var runErr error
			commandResult, runErr = stepPkg.ExecuteCommandResult(step.Name, run)
			return runErr
		}
		executeStep := func() error {
			// Background steps (start/wait/wait-all/cancel) are coordinated by the
			// run-scoped registry; everything else falls through to the normal switch.
			handled := true
			switch {
			case step.BackgroundAsync:
				// Start the container service detached (non-blocking): consecutive background
				// steps come up concurrently. Readiness is enforced by the implicit gate before
				// the next foreground step (and by `wait`/`wait-all`).
				err = workflowPkg.StartBackground(runCtx, bgRegistry, bgRunner, &steps[stepIdx], stepEnv)
			case commandType == schema.TaskTypeWait:
				err = workflowPkg.WaitBackground(runCtx, bgRegistry, step.For)
				if err == nil {
					for _, name := range step.For {
						bgGated[name] = true
					}
				}
			case commandType == schema.TaskTypeWaitAll:
				err = workflowPkg.WaitAllBackground(runCtx, bgRegistry)
				if err == nil {
					for _, name := range bgRegistry.Names() {
						bgGated[name] = true
					}
				}
			case commandType == schema.TaskTypeCancel:
				err = workflowPkg.CancelBackground(runCtx, bgRegistry, step.For)
				for _, name := range step.For {
					delete(bgGated, name)
				}
			default:
				handled = false
			}

			// Implicit readiness gate: before running a foreground step, block until every
			// background service started so far is healthy. Already-gated services are skipped.
			if !handled && err == nil {
				err = workflowPkg.GatePendingBackground(runCtx, bgRegistry, bgGated)
			}

			switch {
			case handled:
				// already executed above
			case err != nil:
				// A failed readiness gate skips this step's foreground work; the error
				// handler below reports it.
			case commandType == schema.TaskTypeParallel, commandType == schema.TaskTypeMatrix:
				err = executeWorkflowControlStep(context.Background(), &workflowControlContext{
					atmosConfig:         atmosConfig,
					workflowDefinition:  workflowDefinition,
					dryRun:              dryRun,
					commandLineStack:    commandLineStack,
					commandLineTags:     commandFilters.tags,
					commandLineLabels:   commandFilters.labels,
					commandLineIdentity: stepIdentity,
					baseEnv:             baseEnv,
					persistentEnv:       persistentEnv,
					authManager:         authManager,
				}, &steps[stepIdx])
			case commandType == "shell":
				// Render command before execution if show.command is enabled.
				// Steps with tty/interactive attach the user's terminal; plain
				// steps keep the existing masked shell-interpreter behavior.
				stepPkg.RenderCommand(&step, workflowDefinition, command)
				commandName := fmt.Sprintf("%s-step-%d", workflow, stepIdx)
				switch {
				case workflowPkg.StepContainerOverride(&step):
					err = retry.Do(context.Background(), step.Retry, func() error {
						return runCommandStep(func(stdout, stderr io.Writer) error {
							return workflowPkg.RunStepContainerOverride(context.Background(), &workflowPkg.ContainerStepParams{
								Workflow:      workflow,
								WorkflowPath:  workflowPath,
								BasePath:      atmosConfig.BasePath,
								WorkflowDef:   workflowDefinition,
								Step:          &step,
								HostWorkDir:   workDir,
								Command:       command,
								StepEnv:       stepEnv,
								RuntimeEnv:    stepEnv,
								DryRun:        dryRun,
								StdoutCapture: stdout,
								StderrCapture: stderr,
							})
						})
					})
				case workflowDefinition.Container != nil && workflowDefinition.Container.IsEnabled() && !workflowPkg.StepContainerDisabled(&step):
					if activeContainer == nil {
						activeContainer, err = workflowPkg.StartWorkflowContainer(context.Background(), &workflowPkg.ContainerStepParams{
							Workflow:     workflow,
							WorkflowPath: workflowPath,
							BasePath:     atmosConfig.BasePath,
							WorkflowDef:  workflowDefinition,
							RuntimeEnv:   stepEnv,
							DryRun:       dryRun,
						})
						if err != nil {
							break
						}
					}
					err = retry.Do(context.Background(), step.Retry, func() error {
						return runCommandStep(func(stdout, stderr io.Writer) error {
							return activeContainer.ExecShell(context.Background(), &workflowPkg.ContainerStepParams{
								Step:          &step,
								WorkflowDef:   workflowDefinition,
								HostWorkDir:   workDir,
								Command:       command,
								StepEnv:       stepEnv,
								StdoutCapture: stdout,
								StderrCapture: stderr,
							})
						})
					})
				default:
					err = retry.Do(context.Background(), step.Retry, func() error {
						return runCommandStep(func(stdoutCapture, stderrCapture io.Writer) error {
							return process.RunShellStep(context.Background(), &process.ShellSessionSpec{
								Command:     command,
								Name:        commandName,
								Dir:         workDir,
								Env:         stepEnv,
								TTY:         step.Tty,
								Interactive: step.Interactive,
								DryRun:      dryRun,
							}, func() error {
								return ExecuteShellWithWriters(&ExecuteShellSpec{
									Command: command,
									Name:    commandName,
									Dir:     workDir,
									EnvVars: stepEnv,
									DryRun:  dryRun,
									Stdout:  io.MultiWriter(ioLayer.MaskWriter(os.Stdout), stdoutCapture),
									Stderr:  io.MultiWriter(ioLayer.MaskWriter(os.Stderr), stderrCapture),
								})
							})
						})
					})
				}
			case commandType == schema.TaskTypeExec:
				// Replace the Atmos process with the command (shell exec semantics).
				// Validated earlier to be the final step; no retry wrapper (the
				// process is replaced, so a retry could never run).
				stepPkg.RenderCommand(&step, workflowDefinition, command)
				err = process.ReplaceShellSession(&process.ExecSpec{
					Command: command,
					Name:    fmt.Sprintf("%s-step-%d", workflow, stepIdx),
					Dir:     ".",
					Env:     stepEnv,
					DryRun:  dryRun,
				})
			case commandType == "atmos":
				// Parse command using shell.Fields for proper quote handling.
				// This correctly handles arguments like -var="foo=bar" by stripping quotes.
				args, parseErr := shell.Fields(command, nil)
				if parseErr != nil {
					log.Debug("Shell parsing failed, falling back to strings.Fields", "error", parseErr, "command", command)
					args = strings.Fields(command)
				}

				args = workflowPkg.AppendAtmosStepFlags(args, workflowPkg.AtmosStepFlags{
					Stack:  finalStack,
					Tags:   commandFilters.tags,
					Labels: commandFilters.labels,
				})
				if finalStack != "" {
					log.Debug("Using stack", "stack", finalStack)
				}

				// Build display command from the final arguments so it matches execution.
				displayCmd := "atmos " + strings.Join(args, " ")
				// Render command before execution if show.command is enabled.
				stepPkg.RenderCommand(&step, workflowDefinition, displayCmd)

				ui.Infof("Executing command: `atmos %s`", command)
				err = retry.Do(context.Background(), step.Retry, func() error {
					return runCommandStep(func(stdout, stderr io.Writer) error {
						return ExecuteShellCommand(
							atmosConfig,
							"atmos",
							args,
							".",
							stepEnv,
							dryRun,
							"",
							WithStdoutCapture(stdout),
							WithStderrCapture(stderr),
						)
					})
				})
			default:
				// Check if this is an extended step type (input, confirm, choose, etc.).
				if !stepPkg.IsExtendedStepType(commandType) {
					return errUtils.Build(errUtils.ErrInvalidWorkflowStepType).
						WithTitle(WorkflowErrTitle).
						WithExplanationf("Workflow `%s` step `%s` uses unsupported type `%s`.", workflow, step.Name, commandType).
						WithContext("workflow", workflow).
						WithContext(logKeyStep, step.Name).
						WithHintf("Step type '%s' is not supported", commandType).
						WithHint("Each step must specify a valid type: 'atmos', 'shell', 'script', 'exec', or an interactive type like 'input', 'confirm', 'choose'").
						WithExitCode(1).
						Err()
				}
				if commandType == schema.TaskTypeScript {
					stepPkg.RenderCommand(&step, workflowDefinition, process.FormatScriptDisplay(step.Interpreter, step.Script))
					switch {
					case workflowPkg.StepContainerOverride(&step):
						err = retry.Do(context.Background(), step.Retry, func() error {
							return workflowPkg.RunStepContainerOverride(context.Background(), &workflowPkg.ContainerStepParams{
								Workflow:     workflow,
								WorkflowPath: workflowPath,
								BasePath:     atmosConfig.BasePath,
								WorkflowDef:  workflowDefinition,
								Step:         &step,
								HostWorkDir:  workDir,
								Command:      process.FormatScriptDisplay(step.Interpreter, step.Script),
								StepEnv:      stepEnv,
								RuntimeEnv:   stepEnv,
								DryRun:       dryRun,
							})
						})
					case workflowDefinition.Container != nil && workflowDefinition.Container.IsEnabled() && !workflowPkg.StepContainerDisabled(&step):
						if activeContainer == nil {
							activeContainer, err = workflowPkg.StartWorkflowContainer(context.Background(), &workflowPkg.ContainerStepParams{
								Workflow:     workflow,
								WorkflowPath: workflowPath,
								BasePath:     atmosConfig.BasePath,
								WorkflowDef:  workflowDefinition,
								RuntimeEnv:   stepEnv,
								DryRun:       dryRun,
							})
							if err != nil {
								break
							}
						}
						err = retry.Do(context.Background(), step.Retry, func() error {
							return activeContainer.ExecShell(context.Background(), &workflowPkg.ContainerStepParams{
								Step:        &step,
								WorkflowDef: workflowDefinition,
								HostWorkDir: workDir,
								Command:     process.FormatScriptDisplay(step.Interpreter, step.Script),
								StepEnv:     stepEnv,
							})
						})
					default:
						err = executeExtendedStep(context.Background(), &steps[stepIdx], workflowDefinition, stepEnv, extendedStepOptions{
							DryRun:      dryRun,
							FinalStack:  finalStack,
							AtmosConfig: &atmosConfig,
						})
					}
					break
				}
				err = executeExtendedStep(context.Background(), &steps[stepIdx], workflowDefinition, stepEnv, extendedStepOptions{
					DryRun:        dryRun,
					FinalStack:    finalStack,
					AtmosConfig:   &atmosConfig,
					ToolchainPATH: tenv.PATH(),
					AuthManager:   authManager,
				})
			}
			if err != nil {
				return err
			}
			return stepPkg.StoreCommandResult(workflowVars, step.Name, step.Outputs, commandResult)
		}

		// Wrap each step's output in a collapsible CI log group when grouping is
		// active. Exec steps run bare because a successful Unix exec never returns
		// to close a deferred group.
		err = stepPkg.RunGroupedForType(&atmosConfig, step.Name, command, commandType, executeStep)
		if err != nil {
			// Terminal-handoff steps (tty/interactive/exec) that exit non-zero
			// propagate the code silently, like a shell - don't wrap them in a
			// themed workflow error (which would query the terminal post-session).
			var silentExit errUtils.ExitCodeError
			if errors.As(err, &silentExit) && silentExit.Silent {
				return err
			}
			stepErr := err
			if !errors.Is(err, errUtils.ErrInvalidWorkflowStepType) {
				stepErr = buildWorkflowStepError(err, &workflowStepErrorContext{
					WorkflowPath:     workflowPath,
					WorkflowBasePath: atmosConfig.Workflows.BasePath,
					Workflow:         workflow,
					StepName:         step.Name,
					Command:          command,
					CommandType:      commandType,
					FinalStack:       finalStack,
				})
			}

			// A `continue:` condition that matches this step's own failure forgives it:
			// subsequent steps still run and the overall workflow status is unaffected,
			// mirroring GitHub Actions' continue-on-error. Malformed `continue:` CEL is a
			// hard failure, never silently forgiven.
			continueCtx := workflowPkg.BuildConditionContext(workflow, workflowDefinition, &step, commandLineStack, workflowDefinition.Env)
			continueCtx.Status = schema.ConditionPredicateFailure
			forgiven, continueErr := step.Continue.EvaluateContinueE(continueCtx)
			if continueErr != nil {
				return fmt.Errorf("%w: %w", errUtils.ErrInvalidContinueCondition, continueErr)
			}
			if forgiven {
				log.Warn("Workflow step failed but 'continue' matched; continuing", "workflow", workflow, logKeyStep, step.Name, "error", stepErr)
				continue
			}

			if workflowErr == nil {
				workflowErr = stepErr
			} else {
				workflowErr = errors.Join(workflowErr, stepErr)
			}
			conditionStatus = schema.ConditionPredicateFailure
		} else if step.Inputs != nil || step.Artifacts != nil {
			// Record the new sources checksum only after a successful Execute() -- a failed
			// step must never falsely mark itself up to date. Recording failure itself is
			// logged, not fatal: it must not fail an otherwise-successful step. Gated on
			// Artifacts too (not just Inputs): an artifacts-only step still needs a recorded
			// (empty) sources hash, or it reruns forever -- see the RecordSuccess doc comment.
			if recErr := freshnessChecker.RecordSuccess(step.Inputs, stepWorkDir, freshnessStateDir, freshnessScope, step.Name); recErr != nil {
				log.Debug("Failed to record freshness state for workflow step", "workflow", workflow, logKeyStep, step.Name, "error", recErr)
			}
		}

		if err == nil && commandType == "env" && (step.Export == nil || *step.Export) {
			for key := range step.Vars {
				if value, ok := workflowVars.Env[key]; ok {
					persistentEnv[key] = value
				}
			}
		}
	}

	// Mark progress as done.
	if progressRenderer.IsEnabled() {
		progressRenderer.Done()
	}

	return workflowErr
}

// stepExecutorState holds persistent state for extended step execution within a workflow.
// This allows step results to be passed between steps for variable templating.
//
// KNOWN LIMITATION: this is a single process-wide global, but dependencies.workflows (see
// taskgraph.Run at this file's ExecuteWorkflow call site) can dispatch multiple sibling
// workflow-kind dependency nodes CONCURRENTLY when they share no edge between them. Two such
// ExecuteWorkflow calls running at once both reset/read/write this same global, which can mix
// template results across the concurrently-running workflows. A real per-invocation fix means
// threading a *stepPkg.StepExecutor as an explicit parameter through ExecuteWorkflow and every
// helper that currently reads this global directly (executeExtendedStep,
// workflow_command_templating.go's resolvers, workflow_control_adapter.go's TemplateData/
// StoreResult callbacks) instead of reaching for the package-level var -- removing the global
// entirely. A quick mutex around this var is NOT a safe substitute: wrapping ExecuteWorkflow's
// whole body deadlocks on a multi-level dependency chain (a dependency dispatched while holding
// the lock recursively calls taskgraph.Run for ITS OWN siblings, whose dispatch goroutines then
// block forever trying to acquire the same non-reentrant lock the parent is holding), and a
// narrower lock/unlock/relock around just the dependency-resolution call leaves a stale-pointer
// race window (a captured `workflowVars := stepExecutorState.Variables()` reference can diverge
// from what the global points to after a concurrent sibling's reset, so some of a workflow's own
// step code reads the old instance while other code re-reads the global and gets a different
// one). Command-kind dependencies do not have this problem: dependencies.commands dispatches
// in-process against Cobra's own per-command flag/context state (see
// pkg/taskgraph/adapters/cobra_command.go), not this shared executor.
var stepExecutorState *stepPkg.StepExecutor

type extendedStepOptions struct {
	DryRun        bool
	FinalStack    string
	AtmosConfig   *schema.AtmosConfiguration
	ToolchainPATH string
	AuthManager   auth.AuthManager
}

// executeExtendedStep runs an extended step type (input, confirm, choose, etc.).
func executeExtendedStep(ctx context.Context, workflowStep *schema.WorkflowStep, workflow *schema.WorkflowDefinition, envVars []string, opts extendedStepOptions) error {
	// Initialize or reuse step executor.
	if stepExecutorState == nil {
		stepExecutorState = stepPkg.NewStepExecutor()
	}

	// Set workflow context for output mode inheritance.
	stepExecutorState.SetWorkflow(workflow)
	configureStepScannerContext(stepExecutorState.Variables(), opts.AtmosConfig, opts.ToolchainPATH, opts.AuthManager)

	// Add environment variables to the executor.
	for _, env := range envVars {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			stepExecutorState.SetEnv(parts[0], parts[1])
		}
	}
	// Always set (or clear) the stack flag so a stackless step does not
	// inherit a stale value from a prior step in the same workflow.
	stepExecutorState.SetFlag("stack", opts.FinalStack)

	// Execute the step.
	stepCopy := *workflowStep
	stepCopy.DryRun = opts.DryRun
	stepCopy.Stack = opts.FinalStack
	_, err := stepExecutorState.Execute(ctx, &stepCopy)
	return err
}

func configureStepScannerContext(vars *stepPkg.Variables, atmosConfig *schema.AtmosConfiguration, toolchainPATH string, authManager auth.AuthManager) {
	if vars == nil {
		return
	}
	vars.SetAtmosConfig(atmosConfig)
	vars.SetToolchainPATH(toolchainPATH)
	vars.SetComponentInfoResolver(func(_ context.Context, component, stack, componentType string) (*schema.ConfigAndStacksInfo, error) {
		info := schema.ConfigAndStacksInfo{
			ComponentFromArg: component,
			ComponentType:    componentType,
			StackFromArg:     stack,
			Stack:            stack,
		}
		stackConfig, err := config.InitCliConfig(info, true)
		if err != nil {
			return nil, err
		}
		var authForStack auth.AuthManager
		if stackConfig.CliConfigPath == atmosConfig.CliConfigPath {
			authForStack = authManager
		}
		resolved, err := ProcessStacks(&stackConfig, info, true, true, false, nil, authForStack)
		if err != nil {
			return nil, err
		}
		return &resolved, nil
	})
}

// ResetStepExecutorState resets the step executor state.
// This should be called at the start of a new workflow execution.
func ResetStepExecutorState() {
	stepExecutorState = nil
}

// initStepExecutorWithStages initializes the step executor with stage count.
// This must be called before executing any steps so stage steps know the total.
func initStepExecutorWithStages(workflow *schema.WorkflowDefinition) {
	if stepExecutorState == nil {
		stepExecutorState = stepPkg.NewStepExecutor()
	}
	totalStages := stepPkg.CountStages(workflow)
	stepExecutorState.Variables().SetTotalStages(totalStages)
}

// buildWorkflowFlagsMap builds a map of workflow flags for display in the header.
func buildWorkflowFlagsMap(stack, identity string, dryRun bool, fromStep string) map[string]string {
	flags := make(map[string]string)
	if stack != "" {
		flags["stack"] = stack
	}
	if identity != "" {
		flags["identity"] = identity
	}
	if dryRun {
		flags["dry-run"] = "true"
	}
	if fromStep != "" {
		flags["from-step"] = fromStep
	}
	return flags
}

// ExecuteDescribeWorkflows executes `atmos describe workflows` command.
func ExecuteDescribeWorkflows(
	atmosConfig schema.AtmosConfiguration,
) ([]schema.DescribeWorkflowsItem, map[string][]string, map[string]schema.WorkflowManifest, error) {
	defer perf.Track(&atmosConfig, "exec.ExecuteDescribeWorkflows")()

	listResult := []schema.DescribeWorkflowsItem{}
	mapResult := make(map[string][]string)
	allResult := make(map[string]schema.WorkflowManifest)

	if atmosConfig.Workflows.BasePath == "" {
		return nil, nil, nil, errUtils.ErrWorkflowBasePathNotConfigured
	}

	// If `workflows.base_path` is a relative path, join it with `stacks.base_path`
	var workflowsDir string
	if u.IsPathAbsolute(atmosConfig.Workflows.BasePath) {
		workflowsDir = atmosConfig.Workflows.BasePath
	} else {
		workflowsDir = filepath.Join(atmosConfig.BasePath, atmosConfig.Workflows.BasePath)
	}

	isDirectory, err := u.IsDirectory(workflowsDir)
	if err != nil || !isDirectory {
		return nil, nil, nil, fmt.Errorf("the workflow directory '%s' does not exist. Review 'workflows.base_path' in 'atmos.yaml'", workflowsDir)
	}

	files, err := u.GetAllYamlFilesInDir(workflowsDir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error reading the directory '%s' defined in 'workflows.base_path' in 'atmos.yaml': %v",
			atmosConfig.Workflows.BasePath, err)
	}

	for _, f := range files {
		var workflowPath string
		if u.IsPathAbsolute(atmosConfig.Workflows.BasePath) {
			workflowPath = filepath.Join(atmosConfig.Workflows.BasePath, f)
		} else {
			workflowPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Workflows.BasePath, f)
		}

		fileContent, err := os.ReadFile(workflowPath)
		if err != nil {
			// Skip files that can't be read (permission issues, etc.).
			log.Warn("Skipping workflow file", "file", f, "error", err)
			continue
		}

		workflowManifest, err := u.UnmarshalYAML[schema.WorkflowManifest](string(fileContent))
		if err != nil {
			// Skip files that can't be parsed as YAML.
			log.Warn("Skipping invalid workflow file", "file", f, "error", err)
			continue
		}

		if workflowManifest.Workflows == nil {
			// Skip files without the workflows key.
			log.Warn("Skipping workflow file without 'workflows:' key", "file", f)
			continue
		}

		workflowConfig := workflowManifest.Workflows
		allWorkflowsInFile := lo.Keys(workflowConfig)
		sort.Strings(allWorkflowsInFile)

		// Check if the workflow steps have the `name` attribute
		lo.ForEach(allWorkflowsInFile, func(item string, _ int) {
			workflowDefinition := workflowConfig[item]
			checkAndGenerateWorkflowStepNames(&workflowDefinition)
		})

		mapResult[f] = allWorkflowsInFile
		allResult[f] = workflowManifest
	}

	for k, v := range mapResult {
		for _, w := range v {
			listResult = append(listResult, schema.DescribeWorkflowsItem{
				File:     k,
				Workflow: w,
			})
		}
	}

	return listResult, mapResult, allResult, nil
}

// WorkflowMatch represents a workflow found during auto-discovery.
type WorkflowMatch struct {
	File        string // Workflow file name (e.g., "networking.yaml")
	Name        string // Workflow name (e.g., "deploy-all")
	Description string // Workflow description (if available)
}

// findWorkflowAcrossFiles searches for a workflow by name across all workflow files.
// Returns a list of matching workflows with their file locations.
func findWorkflowAcrossFiles(workflowName string, atmosConfig *schema.AtmosConfiguration) ([]WorkflowMatch, error) {
	defer perf.Track(atmosConfig, "exec.findWorkflowAcrossFiles")()

	listResult, _, allWorkflows, err := ExecuteDescribeWorkflows(*atmosConfig)
	if err != nil {
		return nil, err
	}

	var matches []WorkflowMatch
	for _, item := range listResult {
		if item.Workflow == workflowName {
			// Get description if available.
			description := ""
			if manifest, ok := allWorkflows[item.File]; ok {
				if workflowDef, ok := manifest.Workflows[workflowName]; ok {
					description = workflowDef.Description
				}
			}

			matches = append(matches, WorkflowMatch{
				File:        item.File,
				Name:        workflowName,
				Description: description,
			})
		}
	}

	return matches, nil
}

// promptForWorkflowFile shows an interactive selector for choosing a workflow file.
// Uses the Huh library with Atmos theme (same pattern as identity selector).
func promptForWorkflowFile(matches []WorkflowMatch) (string, error) {
	defer perf.Track(nil, "exec.promptForWorkflowFile")()

	if len(matches) == 0 {
		return "", ErrNoWorkflowFilesToSelect
	}

	// Check if we're in a TTY environment.
	if !term.IsTTYSupportForStdin() || telemetry.IsCI() {
		return "", ErrNonTTYWorkflowSelection
	}

	// Sort matches alphabetically by file name for consistent ordering.
	sortedMatches := make([]WorkflowMatch, len(matches))
	copy(sortedMatches, matches)
	sort.Slice(sortedMatches, func(i, j int) bool {
		return sortedMatches[i].File < sortedMatches[j].File
	})

	// Build options for the selector.
	// Each option shows the file name with description if available.
	options := make([]huh.Option[string], len(sortedMatches))
	for i, match := range sortedMatches {
		label := match.File
		if match.Description != "" {
			label = fmt.Sprintf("%s - %s", match.File, match.Description)
		}
		options[i] = huh.NewOption(label, match.File)
	}

	var selectedFile string

	// Create custom keymap that adds ESC to quit keys.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Multiple workflows found with name '%s'. Please choose:", sortedMatches[0].Name)).
				Description("Press ctrl+c or esc to exit").
				Options(options...).
				Value(&selectedFile),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", fmt.Errorf("workflow selection failed: %w", err)
	}

	return selectedFile, nil
}

func checkAndGenerateWorkflowStepNames(workflowDefinition *schema.WorkflowDefinition) {
	generateWorkflowStepNames(workflowDefinition.Steps, "")
}

func generateWorkflowStepNames(steps []schema.WorkflowStep, parent string) {
	for index := range steps {
		step := &steps[index]
		if step.Name == "" {
			if parent == "" {
				step.Name = fmt.Sprintf("step%d", index+1)
			} else {
				step.Name = fmt.Sprintf("%s_step%d", parent, index+1)
			}
		}
		if len(step.Steps) > 0 {
			generateWorkflowStepNames(step.Steps, step.Name)
		}
	}
}

func ExecuteWorkflowUI(atmosConfig schema.AtmosConfiguration) (string, string, string, error) {
	defer perf.Track(&atmosConfig, "exec.ExecuteWorkflowUI")()

	_, _, allWorkflows, err := ExecuteDescribeWorkflows(atmosConfig)
	if err != nil {
		return "", "", "", err
	}

	// Start the UI
	app, err := w.Execute(allWorkflows)
	_ = data.Writeln("")
	if err != nil {
		return "", "", "", err
	}

	selectedWorkflowFile := app.GetSelectedWorkflowFile()
	selectedWorkflow := app.GetSelectedWorkflow()
	selectedWorkflowStep := app.GetSelectedWorkflowStep()

	// If the user quit the UI, exit
	if app.ExitStatusQuit() || selectedWorkflowFile == "" || selectedWorkflow == "" {
		return "", "", "", nil
	}

	c := fmt.Sprintf("atmos workflow %s --file %s --from-step \"%s\"", selectedWorkflow, selectedWorkflowFile, selectedWorkflowStep)
	log.Info("Executing", "command", c)

	return selectedWorkflowFile, selectedWorkflow, selectedWorkflowStep, nil
}

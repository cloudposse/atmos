package terraform

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/execution"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Hook context variables. They name Terragrunt's contract because hooks written for Terragrunt
// read them; they are set for every provider-supplied hook.
const (
	hookEnvTerraformPath = "TG_CTX_TF_PATH"
	hookEnvCommand       = "TG_CTX_COMMAND"
	hookEnvHookName      = "TG_CTX_HOOK_NAME"
)

// executionPhase describes one Terraform subprocess that provider-supplied hooks and extra
// arguments may wrap. Each phase is matched on its own command: an implicit init is "init"
// whatever command the user requested, and workspace selection or provider locking are not
// phases at all, so a hook listed for "plan" never fires for them.
type executionPhase struct {
	// command is the name hooks and extra arguments list (init, plan, destroy, ...).
	command string
	// args is the full argument vector, starting with the Terraform command.
	args []string
	// insertAt is where extra arguments go: after the command and its subcommands.
	insertAt int
	// planFileArgument reports that the last argument is a saved plan.
	planFileArgument bool
	// componentPath is the provisioned module directory.
	componentPath string
	// main marks the command the user asked for, as opposed to a step that prepares for it.
	main bool
}

// mainCommandArgsKey carries the effective main command arguments to plan binding.
type mainCommandArgsKey struct{}

// mainCommandIgnoredKey carries a flag that the errors policy sets when it ignores a failure of the
// main command, so the caller can tell that run from one that succeeded.
type mainCommandIgnoredKey struct{}

// markMainCommandIgnored records that the errors policy ignored the main command's failure.
func markMainCommandIgnored(ctx context.Context) {
	if ignored, ok := ctx.Value(mainCommandIgnoredKey{}).(*bool); ok {
		*ignored = true
	}
}

// mainCommandRecoveryKey carries smart init's recovery into the main command's hook lifecycle.
type mainCommandRecoveryKey struct{}

// mainCommandRecovery runs smart init's recovery for a failed main command. Its retry re-runs only
// the Terraform subprocess, inside the lifecycle that already ran the before hooks.
type mainCommandRecovery struct {
	recover func(mainErr error, retry func() error) error
}

// lifecycleOwnsRecovery reports whether the main command runs inside a hook lifecycle that
// recovers from a missing init itself. Dry runs never run hooks, so they recover outside it.
func lifecycleOwnsRecovery(info *schema.ConfigAndStacksInfo) bool {
	config := info.ComponentExecution
	if info.DryRun || config == nil {
		return false
	}
	return len(config.BeforeHooks)+len(config.AfterHooks)+len(config.ErrorHooks) > 0
}

// runMainShellCommand runs the requested Terraform subprocess, wrapped by the component's
// provider-supplied hooks and extra arguments. They match the requested command, which is
// "destroy" for `apply -destroy`.
func (e *Executor) runMainShellCommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, args []string, componentPath string, opts []component.ExecutionOption) error {
	position := extraArgumentsPosition(info, args)
	phase := &executionPhase{
		command:          execution.CommandName(info.SubCommand, args[position:]),
		args:             args,
		insertAt:         position,
		planFileArgument: info.SubCommand == subcommandApply && info.UseTerraformPlan,
		componentPath:    componentPath,
		main:             true,
	}
	return e.runExecutionPhase(atmosConfig, info, phase, opts, func(phaseInfo *schema.ConfigAndStacksInfo, phaseArgs []string, phaseOpts []component.ExecutionOption) error {
		ctx := component.ContextFromOptions(phaseOpts...)
		if captured, ok := ctx.Value(mainCommandArgsKey{}).(*[]string); ok {
			*captured = slices.Clone(phaseArgs)
		}
		run := func() error {
			var changesPresent error
			err := ExecuteShellCommandWithRetry(
				atmosConfig,
				phaseInfo,
				info.SubCommand,
				func(o ...component.ExecutionOption) error {
					err := e.executeStreamingOrShell(atmosConfig, phaseInfo, &streamingExecRequest{
						componentPath:  componentPath,
						args:           phaseArgs,
						gatePhase:      info.SubCommand,
						subCommand:     info.SubCommand,
						redirectStdErr: info.RedirectStdErr,
						shellOpts:      o,
					})
					// Classify with the effective arguments and environment before legacy
					// output matching can retry a plan that already succeeded.
					if isDetailedExitCodeChanges(phase.command, phaseArgs, phaseInfo.ComponentEnvList, err, o...) {
						changesPresent = err
						return nil
					}
					return err
				},
				phaseOpts...,
			)
			if err != nil {
				return err
			}
			return changesPresent
		}
		err := run()
		// A plan that reports changes completed; only a real failure is given to init recovery.
		if err == nil || !lifecycleOwnsRecovery(info) || isDetailedExitCodeChanges(phase.command, phaseArgs, phaseInfo.ComponentEnvList, err, phaseOpts...) {
			return err
		}
		if recovery, ok := ctx.Value(mainCommandRecoveryKey{}).(*mainCommandRecovery); ok {
			return recovery.recover(err, run)
		}
		return err
	})
}

// preflightRequiredVarFiles checks the requested command's required variable files in the
// provisioned module directory. A saved-plan apply is exempt: Terraform rejects variable
// options beside a plan file, so runExecutionPhase never adds them.
func preflightRequiredVarFiles(info *schema.ConfigAndStacksInfo, componentPath string) error {
	config := info.ComponentExecution
	if config.IsEmpty() || !shouldRunMainTerraformCommand(info) {
		return nil
	}
	if info.SubCommand == subcommandApply && info.UseTerraformPlan {
		return nil
	}
	command := execution.CommandName(info.SubCommand, info.AdditionalArgsAndFlags)
	return execution.CheckRequiredVarFiles(config.ExtraArguments, command, componentPath)
}

// phaseAction runs the wrapped Terraform subprocess with the phase's arguments, environment, and options.
type phaseAction func(info *schema.ConfigAndStacksInfo, args []string, opts []component.ExecutionOption) error

// runExecutionPhase applies the component's extra arguments and runs its hooks around action.
// A component without provider-supplied execution runs action unchanged. Dry runs apply the
// arguments but never run hooks.
func (e *Executor) runExecutionPhase(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, phase *executionPhase, opts []component.ExecutionOption, action phaseAction) error {
	config := info.ComponentExecution
	if config.IsEmpty() {
		if err := validatePlanOutputFlags(phase.command, phase.args); err != nil {
			return err
		}
		return successfulPlanResult(phase.command, phase.args, info.ComponentEnvList, action(info, phase.args, opts), opts...)
	}

	applied, err := execution.ApplyExtraArguments(config.ExtraArguments, &execution.ArgumentsRequest{
		Command:          phase.command,
		Args:             phase.args,
		InsertAt:         phase.insertAt,
		PlanFileArgument: phase.planFileArgument,
		WorkingDirectory: phase.componentPath,
	})
	if err != nil {
		return err
	}
	if err := validatePlanOutputFlags(phase.command, applied.Args); err != nil {
		return err
	}
	// Extra environment applies to this phase and to its hooks, as it does for Terragrunt.
	phaseInfo := *info
	phaseInfo.ComponentEnvList = append(slices.Clone(info.ComponentEnvList), applied.Environment...)

	if info.DryRun {
		log.Debug("Skipping hooks in dry-run mode", "command", phase.command)
		return action(&phaseInfo, applied.Args, opts)
	}

	var capture *phaseCapture
	if len(config.ErrorHooks) > 0 {
		capture = &phaseCapture{}
		defer capture.cleanup()
	}
	runner := &hookRunner{executor: e, atmosConfig: atmosConfig, info: &phaseInfo, phase: phase, opts: hookOptions(opts), capture: capture}
	// With -detailed-exitcode, exit status 2 reports a plan with changes. Hooks must not see that
	// as a failure, but the status still has to reach the caller once the hooks are done.
	attempt := func(attemptOpts []component.ExecutionOption, hookStderr io.Writer) (changesPresent, runErr error) {
		runner.recoveryStderr = hookStderr
		runErr = execution.Run(component.ContextFromOptions(attemptOpts...), config, phase.command, runner, func() (execution.Output, error) {
			output, actionErr := runPhaseAction(&phaseInfo, applied.Args, attemptOpts, capture, action)
			if isDetailedExitCodeChanges(phase.command, applied.Args, phaseInfo.ComponentEnvList, actionErr, attemptOpts...) {
				// Mark the completed plan here, so the recovery path reports it the same way.
				changesPresent = &successfulPlanChanges{error: actionErr}
				return output, nil
			}
			return output, actionErr
		})
		return changesPresent, runErr
	}
	if config.Errors != nil {
		return e.recoverPhase(&phaseInfo, phase, config.Errors, opts, attempt)
	}
	changesPresent, err := attempt(opts, nil)
	if err != nil {
		return err
	}
	return changesPresent
}

// successfulPlanChanges distinguishes a completed plan (including its hooks) from a
// failed command or hook that happens to return the same exit status.
type successfulPlanChanges struct{ error }

func (e *successfulPlanChanges) Unwrap() error {
	defer perf.Track(nil, "terraform.successfulPlanChanges.Unwrap")()
	return e.error
}

func successfulPlanResult(command string, args, env []string, err error, opts ...component.ExecutionOption) error {
	if isDetailedExitCodeChanges(command, args, env, err, opts...) {
		return &successfulPlanChanges{error: err}
	}
	return err
}

// runPhaseAction runs the wrapped subprocess. Error hooks match against the failed command's
// output, which the caller may also be capturing, so it is captured only when they exist. The
// capture goes to a temporary file, never to memory, and keeps every retry attempt's output.
func runPhaseAction(info *schema.ConfigAndStacksInfo, args []string, opts []component.ExecutionOption, capture *phaseCapture, action phaseAction) (execution.Output, error) {
	if capture == nil {
		return nil, action(info, args, opts)
	}
	file := capture.create()
	stdout, stderr := composeRetryCaptureWriters(opts, file)
	err := action(info, args, append(slices.Clone(opts), component.WithStdoutCapture(stdout), component.WithStderrCapture(stderr)))
	return file.finish(), err
}

// detailedExitCodeChanges is the status `plan -detailed-exitcode` returns when changes are present.
const detailedExitCodeChanges = 2

// isDetailedExitCodeChanges reports whether err is exit status 2 of a command run with
// -detailed-exitcode, which Terragrunt treats as success for hooks and the success of the unit.
func isDetailedExitCodeChanges(command string, args, env []string, err error, opts ...component.ExecutionOption) bool {
	if command != "plan" || err == nil || errUtils.GetExitCode(err) != detailedExitCodeChanges {
		return false
	}
	return detailedExitCodeEnabled(command, args, env, opts...)
}

// detailedExitCodeEnabled reports whether -detailed-exitcode is in effect for the command.
// Terraform inserts TF_CLI_ARGS_<command>, then TF_CLI_ARGS, before the command-line arguments,
// and the last setting of a flag wins, so an explicit command-line setting takes precedence.
// Terraform reads the variables as shell words, so quoted values are unquoted the same way.
func detailedExitCodeEnabled(command string, args, env []string, opts ...component.ExecutionOption) bool {
	suffix := strings.NewReplacer("-", "_", " ", "_").Replace(command)
	words := slices.Concat(
		parseArgs(environmentValue(env, "TF_CLI_ARGS_"+suffix, opts...)),
		parseArgs(environmentValue(env, "TF_CLI_ARGS", opts...)),
		args,
	)
	enabled := false
	for _, word := range words {
		name, value, hasValue := strings.Cut(strings.TrimLeft(word, "-"), "=")
		if !strings.HasPrefix(word, "-") || name != "detailed-exitcode" {
			continue
		}
		if !hasValue {
			enabled = true
			continue
		}
		if parsed, err := strconv.ParseBool(value); err == nil {
			enabled = parsed
		}
	}
	return enabled
}

// environmentValue returns the value of key the subprocess sees: the component environment is
// appended to the selected process environment, so its last entry wins.
func environmentValue(env []string, key string, opts ...component.ExecutionOption) string {
	var options component.ExecutionOptions
	for _, opt := range opts {
		opt(&options)
	}
	base := options.ProcessEnv
	if base == nil {
		base = os.Environ()
	}
	merged := slices.Concat(base, env)
	for i := len(merged) - 1; i >= 0; i-- {
		if name, value, found := strings.Cut(merged[i], "="); found && name == key {
			return value
		}
	}
	return ""
}

// hookOptions carries over only what a hook shares with the command: cancellation, the process
// environment chosen by the identity, and the terminal streams. Output capture stays with the
// command so hook output never reaches plan parsers, CI summaries, or retry matching.
func hookOptions(opts []component.ExecutionOption) []component.ExecutionOption {
	var base component.ExecutionOptions
	for _, opt := range opts {
		opt(&base)
	}
	var result []component.ExecutionOption
	if base.Ctx != nil {
		result = append(result, component.WithProcessContext(base.Ctx))
	}
	if base.ProcessEnv != nil {
		result = append(result, component.WithEnvironment(base.ProcessEnv))
	}
	if base.Streams != nil {
		result = append(result, component.WithProcessStreams(*base.Streams))
	}
	return result
}

// hookRunner runs provider-supplied hooks through the same process facility as Terraform itself,
// so they get the invocation's environment (identity, toolchain PATH, stack and component env),
// secret masking, cancellation, and dry-run handling.
type hookRunner struct {
	executor    *Executor
	atmosConfig *schema.AtmosConfiguration
	info        *schema.ConfigAndStacksInfo
	phase       *executionPhase
	opts        []component.ExecutionOption
	// capture records hook output for error hook matching; nil means no error hooks exist.
	capture *phaseCapture
	// recoveryStderr is per attempt and separate from the caller's command capture.
	recoveryStderr io.Writer
}

// RunHook runs one hook and returns its captured output for error hook matching.
// Hook stdout goes to the UI channel (stderr), never to the data channel, unless suppressed.
func (r *hookRunner) RunHook(_ context.Context, kind execution.HookKind, hook *schema.ComponentHook) (execution.Output, error) {
	defer perf.Track(r.atmosConfig, "terraform.hookRunner.RunHook")()

	ui.Info(fmt.Sprintf("Executing %s: %s%s", strings.ReplaceAll(string(kind), "_", " "), hook.Name, hookUnitLabel(r.info)))

	var base component.ExecutionOptions
	for _, opt := range r.opts {
		opt(&base)
	}
	stdout := process.OSStreams().Stderr
	if base.Streams != nil && base.Streams.Stderr != nil {
		stdout = base.Streams.Stderr
	}
	if hook.SuppressStdout {
		stdout = io.Discard
	}

	captured := io.Discard
	var file *captureFile
	if r.capture != nil {
		file = r.capture.create()
		captured = file
	}
	stderrCapture := captured
	if r.recoveryStderr != nil {
		stderrCapture = io.MultiWriter(captured, &syncWriter{w: r.recoveryStderr})
	}
	opts := append(
		slices.Clone(r.opts),
		component.WithStdoutOverride(stdout),
		component.WithStdoutCapture(captured),
		component.WithStderrCapture(stderrCapture),
	)
	env := append(
		slices.Clone(r.info.ComponentEnvList),
		hookEnvTerraformPath+"="+r.info.Command,
		hookEnvCommand+"="+r.phase.command,
		hookEnvHookName+"="+hook.Name,
	)
	directory, err := r.workingDirectory(hook)
	if err != nil {
		return nil, err
	}
	arguments, err := r.hookArguments(hook, directory)
	if err != nil {
		return nil, err
	}
	err = r.executor.services.ExecuteShellCommand(*r.atmosConfig, hook.Execute[0], arguments, directory, env, false, "", opts...)
	if file == nil {
		return nil, err
	}
	return file.finish(), err
}

// hookArguments returns the arguments a hook runs with. A tflint hook gets the stack's final inputs
// bound to its variables.
func (r *hookRunner) hookArguments(hook *schema.ComponentHook, directory string) ([]string, error) {
	arguments := hook.Execute[1:]
	if hook.Execute[0] != "tflint" {
		return arguments, nil
	}
	arguments, err := resolveTflintHookArguments(arguments, directory, r.info.ComponentVarsSection)
	if err != nil {
		return nil, fmt.Errorf("tflint hook %q: %w", hook.Name, err)
	}
	return arguments, nil
}

// hookUnitLabel names the component and stack a hook runs for, so output of a bulk run can be attributed.
func hookUnitLabel(info *schema.ConfigAndStacksInfo) string {
	if info.ComponentFromArg == "" {
		return ""
	}
	return fmt.Sprintf(" (%s in %s)", info.ComponentFromArg, info.Stack)
}

// workingDirectory defaults to the provisioned module directory; relative paths resolve there.
// A directory that does not exist is reported as such, instead of as a failure to start the program.
func (r *hookRunner) workingDirectory(hook *schema.ComponentHook) (string, error) {
	switch {
	case hook.WorkingDirectory == "":
		return r.phase.componentPath, nil
	case filepath.IsAbs(hook.WorkingDirectory):
		return hook.WorkingDirectory, requireDirectory(hook.WorkingDirectory, "")
	default:
		resolved := filepath.Join(r.phase.componentPath, hook.WorkingDirectory)
		return resolved, requireDirectory(resolved, hook.WorkingDirectory)
	}
}

// requireDirectory fails with the missing directory's name; relative names also say where they resolved.
func requireDirectory(directory, relative string) error {
	if info, err := os.Stat(directory); err == nil && info.IsDir() {
		return nil
	}
	if relative == "" {
		return fmt.Errorf("%w: %s", errUtils.ErrComponentHookWorkingDirectory, directory)
	}
	return fmt.Errorf("%w: %s (working_dir %q resolves against the provisioned module directory, not the directory of terragrunt.hcl; use ${get_terragrunt_dir()}/%s for a directory beside it)", errUtils.ErrComponentHookWorkingDirectory, directory, relative, relative)
}

// expandPolicyPlanArguments gives a plan policy the effective plan invocation. The policy
// selects its artifact from -out before Terraform runs, but provider extra arguments are
// otherwise added later, in runExecutionPhase. This adds them now and rejects conflicting
// output flags. It then removes the added arguments from the component execution until
// restore, so runExecutionPhase applies only the environment and hooks, and the arguments
// are not added twice. Double-dash output flags are written as -out, which the policy
// guard recognizes. Terraform accepts both forms. The returned arguments are also the
// published plan path that plan binding records.
func expandPolicyPlanArguments(info *schema.ConfigAndStacksInfo, execCtx *componentExecContext, args []string, componentPath string) ([]string, func(), error) {
	noop := func() {}
	if execCtx.planPolicy == nil || info.DryRun || info.SubCommand != subcommandPlan || len(args) == 0 {
		return args, noop, nil
	}
	config := info.ComponentExecution
	if config.IsEmpty() {
		return singleDashOutputFlags(args), noop, nil
	}
	position := extraArgumentsPosition(info, args)
	command := execution.CommandName(info.SubCommand, args[position:])
	applied, err := execution.ApplyExtraArguments(config.ExtraArguments, &execution.ArgumentsRequest{
		Command: command, Args: args, InsertAt: position, WorkingDirectory: componentPath,
	})
	if err != nil {
		return nil, noop, err
	}
	if err := validatePlanOutputFlags(command, applied.Args); err != nil {
		return nil, noop, err
	}
	stripped := *config
	stripped.ExtraArguments = slices.Clone(config.ExtraArguments)
	for i := range stripped.ExtraArguments {
		if slices.Contains(stripped.ExtraArguments[i].Commands, command) {
			stripped.ExtraArguments[i].Arguments = nil
			stripped.ExtraArguments[i].RequiredVarFiles = nil
			stripped.ExtraArguments[i].OptionalVarFiles = nil
		}
	}
	info.ComponentExecution = &stripped
	return singleDashOutputFlags(applied.Args), func() { info.ComponentExecution = config }, nil
}

// singleDashOutputFlags rewrites double-dash output flags in single-dash form, keeping their values.
func singleDashOutputFlags(args []string) []string {
	result := slices.Clone(args)
	for i, arg := range result {
		if arg == "--out" || strings.HasPrefix(arg, "--out=") {
			result[i] = arg[1:]
		}
	}
	return result
}

// extraArgumentsPosition is the index after the Terraform command and its subcommands, so extra
// arguments precede the caller's flags exactly as Terragrunt inserts them.
func extraArgumentsPosition(info *schema.ConfigAndStacksInfo, args []string) int {
	position := min(len(strings.Fields(info.SubCommand)), len(args))
	if info.SubCommand2 != "" && position < len(args) && args[position] == info.SubCommand2 {
		position++
	}
	return position
}

package terraform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

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
}

// mainCommandArgsKey carries the effective main command arguments to plan binding.
type mainCommandArgsKey struct{}

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
	}
	return e.runExecutionPhase(atmosConfig, info, phase, opts, func(phaseInfo *schema.ConfigAndStacksInfo, phaseArgs []string, phaseOpts []component.ExecutionOption) error {
		if captured, ok := component.ContextFromOptions(phaseOpts...).Value(mainCommandArgsKey{}).(*[]string); ok {
			*captured = slices.Clone(phaseArgs)
		}
		return ExecuteShellCommandWithRetry(
			atmosConfig,
			phaseInfo,
			info.SubCommand,
			func(o ...component.ExecutionOption) error {
				return e.executeStreamingOrShell(atmosConfig, phaseInfo, &streamingExecRequest{
					componentPath:  componentPath,
					args:           phaseArgs,
					gatePhase:      info.SubCommand,
					subCommand:     info.SubCommand,
					redirectStdErr: info.RedirectStdErr,
					shellOpts:      o,
				})
			},
			phaseOpts...,
		)
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
		return action(info, phase.args, opts)
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
	// A detailed plan exit code of 2 reports changes, not a failure: ordinary after hooks run and
	// error hooks do not. The status itself is returned unchanged once the hooks have run.
	var detailedResult error
	err = execution.Run(component.ContextFromOptions(opts...), config, phase.command, runner, func() (execution.Output, error) {
		output, err := runPhaseAction(&phaseInfo, applied.Args, opts, capture, action)
		if isDetailedPlanChanges(phase, applied.Args, err) {
			detailedResult = err
			return output, nil
		}
		return output, err
	})
	if err == nil {
		return detailedResult
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

// isDetailedPlanChanges reports whether err is plan's "-detailed-exitcode" status for changes present.
func isDetailedPlanChanges(phase *executionPhase, args []string, err error) bool {
	return err != nil && phase.command == "plan" && slices.Contains(args, "-detailed-exitcode") && resolveExitCode(err) == detailedExitCodeChangesDetected
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
}

// RunHook runs one hook and returns its captured output for error hook matching.
// Hook stdout goes to the UI channel (stderr), never to the data channel, unless suppressed.
func (r *hookRunner) RunHook(_ context.Context, kind execution.HookKind, hook *schema.ComponentHook) (execution.Output, error) {
	defer perf.Track(r.atmosConfig, "terraform.hookRunner.RunHook")()

	ui.Info(fmt.Sprintf("Executing %s: %s", strings.ReplaceAll(string(kind), "_", " "), hook.Name))

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
	opts := append(
		slices.Clone(r.opts),
		component.WithStdoutOverride(stdout),
		component.WithStdoutCapture(captured),
		component.WithStderrCapture(captured),
	)
	env := append(
		slices.Clone(r.info.ComponentEnvList),
		hookEnvTerraformPath+"="+r.info.Command,
		hookEnvCommand+"="+r.phase.command,
		hookEnvHookName+"="+hook.Name,
	)
	err := r.executor.services.ExecuteShellCommand(*r.atmosConfig, hook.Execute[0], hook.Execute[1:], r.workingDirectory(hook), env, false, "", opts...)
	if file == nil {
		return nil, err
	}
	return file.finish(), err
}

// workingDirectory defaults to the provisioned module directory; relative paths resolve there.
func (r *hookRunner) workingDirectory(hook *schema.ComponentHook) string {
	switch {
	case hook.WorkingDirectory == "":
		return r.phase.componentPath
	case filepath.IsAbs(hook.WorkingDirectory):
		return hook.WorkingDirectory
	default:
		return filepath.Join(r.phase.componentPath, hook.WorkingDirectory)
	}
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

// phaseCapture owns the temporary files that record phase and hook output for error hook
// matching. Cleanup removes every file once the lifecycle, including matching, has finished.
type phaseCapture struct {
	mu    sync.Mutex
	files []*captureFile
}

// create starts a new capture file. If the file cannot be created, the command still runs and
// the capture reports the failure when error hooks read it.
func (c *phaseCapture) create() *captureFile {
	file, err := os.CreateTemp("", "atmos-phase-output-*")
	capture := &captureFile{file: file}
	if err != nil {
		capture.err = captureError(err)
		return capture
	}
	capture.path = file.Name()
	c.mu.Lock()
	c.files = append(c.files, capture)
	c.mu.Unlock()
	return capture
}

// cleanup closes and removes every capture file.
func (c *phaseCapture) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, file := range c.files {
		file.close()
		if err := os.Remove(file.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Debug("Failed to remove captured command output", "path", file.path, "error", err)
		}
	}
	c.files = nil
}

// captureFile records output in a temporary file. A write failure is kept and reported when
// the output is read. The command keeps running, so a full disk cannot fail it.
type captureFile struct {
	mu   sync.Mutex
	file *os.File
	path string
	err  error
}

// Write appends to the capture and always reports success to the writer chain.
//
//nolint:lintroller // Called for every output chunk; tracking would dominate the copy.
func (c *captureFile) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil && c.file != nil {
		if _, err := c.file.Write(p); err != nil {
			c.err = captureError(err)
		}
	}
	return len(p), nil
}

// finish closes the file for writing and returns it as lifecycle output.
func (c *captureFile) finish() execution.Output {
	c.close()
	return c
}

func (c *captureFile) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.file == nil {
		return
	}
	if err := c.file.Close(); err != nil && c.err == nil {
		c.err = captureError(err)
	}
	c.file = nil
}

// Open returns a reader over the captured output, or the capture failure.
func (c *captureFile) Open() (io.Reader, error) {
	defer perf.Track(nil, "terraform.captureFile.Open")()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	file, err := os.Open(c.path)
	if err != nil {
		return nil, captureError(err)
	}
	return file, nil
}

// captureError marks a failure to record or read captured command output.
func captureError(err error) error {
	return fmt.Errorf("%w: %w", errUtils.ErrComponentOutputCapture, err)
}

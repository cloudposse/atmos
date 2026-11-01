package terraform

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	}
	return e.runExecutionPhase(atmosConfig, info, phase, opts, func(phaseInfo *schema.ConfigAndStacksInfo, phaseArgs []string, phaseOpts []component.ExecutionOption) error {
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

// phaseAction runs the wrapped Terraform subprocess with the phase's arguments, environment, and options.
type phaseAction func(info *schema.ConfigAndStacksInfo, args []string, opts []component.ExecutionOption) error

// runExecutionPhase applies the component's extra arguments and runs its hooks around action.
// A component without provider-supplied execution runs action unchanged. Dry runs apply the
// arguments but never run hooks.
func (e *Executor) runExecutionPhase(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, phase *executionPhase, opts []component.ExecutionOption, action phaseAction) error {
	config := info.ComponentExecution
	if config.IsEmpty() {
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
	// Extra environment applies to this phase and to its hooks, as it does for Terragrunt.
	phaseInfo := *info
	phaseInfo.ComponentEnvList = append(slices.Clone(info.ComponentEnvList), applied.Environment...)

	if info.DryRun {
		log.Debug("Skipping hooks in dry-run mode", "command", phase.command)
		return action(&phaseInfo, applied.Args, opts)
	}

	runner := &hookRunner{executor: e, atmosConfig: atmosConfig, info: &phaseInfo, phase: phase, opts: hookOptions(opts)}
	// With -detailed-exitcode, exit status 2 reports a plan with changes. Hooks must not see that
	// as a failure, but the status still has to reach the caller once the hooks are done.
	var changesPresent error
	err = execution.Run(component.ContextFromOptions(opts...), config, phase.command, runner, func() (string, error) {
		output, actionErr := runObservedAction(&phaseInfo, applied.Args, opts, config, action)
		if isDetailedExitCodeChanges(applied.Args, actionErr) {
			changesPresent = actionErr
			return output, nil
		}
		return output, actionErr
	})
	if err != nil {
		return err
	}
	return changesPresent
}

// runObservedAction runs the wrapped subprocess, capturing its output for error hooks when any exist.
func runObservedAction(info *schema.ConfigAndStacksInfo, args []string, opts []component.ExecutionOption, config *schema.ComponentExecution, action phaseAction) (string, error) {
	if len(config.ErrorHooks) == 0 {
		return "", action(info, args, opts)
	}
	// Error hooks match against the failed command's output, which the caller may also be capturing.
	var output bytes.Buffer
	stdout, stderr := composeRetryCaptureWriters(opts, &output)
	err := action(info, args, append(slices.Clone(opts), component.WithStdoutCapture(stdout), component.WithStderrCapture(stderr)))
	return output.String(), err
}

// detailedExitCodeChanges is the status `plan -detailed-exitcode` returns when changes are present.
const detailedExitCodeChanges = 2

// isDetailedExitCodeChanges reports whether err is exit status 2 of a command run with
// -detailed-exitcode, which Terragrunt treats as success for hooks and the success of the unit.
func isDetailedExitCodeChanges(args []string, err error) bool {
	if err == nil || errUtils.GetExitCode(err) != detailedExitCodeChanges {
		return false
	}
	return slices.ContainsFunc(args, func(arg string) bool {
		return arg == "-detailed-exitcode" || arg == "--detailed-exitcode"
	})
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
}

// RunHook runs one hook and returns its captured output for error hook matching.
// Hook stdout goes to the UI channel (stderr), never to the data channel, unless suppressed.
func (r *hookRunner) RunHook(_ context.Context, kind execution.HookKind, hook *schema.ComponentHook) (string, error) {
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

	var output bytes.Buffer
	captured := &syncWriter{w: &output}
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
	directory, err := r.workingDirectory(hook)
	if err != nil {
		return "", err
	}
	err = r.executor.services.ExecuteShellCommand(*r.atmosConfig, hook.Execute[0], hook.Execute[1:], directory, env, false, "", opts...)
	return output.String(), err
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

// extraArgumentsPosition is the index after the Terraform command and its subcommands, so extra
// arguments precede the caller's flags exactly as Terragrunt inserts them.
func extraArgumentsPosition(info *schema.ConfigAndStacksInfo, args []string) int {
	position := min(len(strings.Fields(info.SubCommand)), len(args))
	if info.SubCommand2 != "" && position < len(args) && args[position] == info.SubCommand2 {
		position++
	}
	return position
}

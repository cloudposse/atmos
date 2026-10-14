package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"mvdan.cc/sh/v3/shell"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/retry"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
	_ "github.com/cloudposse/atmos/pkg/script/starlark" // Register the embedded interpreter.
)

type ControlEnvironmentFunc func(baseEnv []string, identity string, stepName string, workflowEnv map[string]string, stepEnv map[string]string) ([]string, error)

type ControlCommandRequest struct {
	// DryRun asks the runner to report the command without executing it.
	DryRun  bool
	Context context.Context
	Program string
	Args    []string
	Dir     string
	Env     []string
	Streams process.Streams
	Stdout  *bytes.Buffer
	Stderr  *bytes.Buffer
}

type ControlCommandRunner func(request *ControlCommandRequest) error

// ControlShellRequest carries the inputs for a control shell child's execution.
type ControlShellRequest struct {
	Command string
	Dir     string
	Env     []string
	Stdout  io.Writer
	Stderr  io.Writer
}

// ControlShellRunner executes a `type: shell` child's command string. When set on
// a ControlCommandExecutor it replaces the host `sh -c` / `RunCommand` path so
// shell children run through the in-process mvdan/sh interpreter (cross-platform,
// masked, cancellable). The workflow executor leaves it nil to keep its
// auth-aware RunCommand path; the registry bridge sets it.
type ControlShellRunner func(ctx context.Context, req *ControlShellRequest) error

type ControlCommandExecutor struct {
	// DryRun makes the executor execute nothing. Children that run through RunCommand
	// (atmos, shell without a ShellRunner, external script) are still dispatched with
	// ControlCommandRequest.DryRun set, so the runner owns the dry-run behavior (the
	// workflow runner emits its dry-run diagnostics there). Children with no dry-run-aware
	// runner (a ShellRunner shell child and every embedded script child) are skipped here,
	// before any runner or embedded engine is reached.
	DryRun bool
	// ScriptHook supplies host-owned lifecycle facts to embedded script children.
	ScriptHook *script.HookContext
	// ScriptComponent is the component the enclosing execution context is scoped
	// to. Embedded script children expose it as `ctx.component`.
	ScriptComponent *script.ComponentRef
	// ResolveComponent backs `components.get` in embedded script children.
	ResolveComponent script.ComponentResolver
	InstallTools     script.ToolInstaller
	// ProjectRoot is the absolute Atmos project base path. Embedded script children show paths under
	// it relative to it in errors and tracebacks.
	ProjectRoot string
	// ScriptProcessOverrides names the env keys whose values take precedence over
	// component env for component processes (command-level env). The values are
	// refreshed from each child's final process env at execution time.
	ScriptProcessOverrides map[string]string

	WorkflowDefinition  *schema.WorkflowDefinition
	BasePath            string
	BaseEnv             []string
	CommandLineStack    string
	CommandLineTags     []string
	CommandLineLabels   string
	CommandLineIdentity string
	PrepareEnv          ControlEnvironmentFunc
	RunCommand          ControlCommandRunner
	ShellRunner         ControlShellRunner

	outputMu sync.Mutex
}

func (executor *ControlCommandExecutor) Execute(ctx context.Context, child *ControlChild, output ControlChildOutput) (*ControlChildResult, error) {
	step := child.Step
	stepType := strings.TrimSpace(step.Type)
	if stepType == "" {
		stepType = schema.TaskTypeAtmos
	}

	switch stepType {
	case schema.TaskTypeShell, schema.TaskTypeAtmos, schema.TaskTypeScript:
		stepEnv, err := executor.stepEnv(&step)
		if err != nil {
			return &ControlChildResult{}, err
		}
		if stepType == schema.TaskTypeShell {
			return executor.executeShell(ctx, &step, stepEnv, output)
		}
		if stepType == schema.TaskTypeScript {
			return executor.executeScript(ctx, &step, stepEnv, output)
		}
		return executor.executeAtmos(ctx, &step, stepEnv, output)
	case "sleep":
		return executeControlSleep(ctx, &step)
	default:
		return &ControlChildResult{}, fmt.Errorf("%w: unsupported nested workflow step type %q", schema.ErrWorkflowControlStepInvalid, stepType)
	}
}

// rejectEmbeddedContainer fails a child that would run an embedded interpreter
// under a container, which cannot host an in-process engine.
func (executor *ControlCommandExecutor) rejectEmbeddedContainer(step *schema.WorkflowStep) error {
	workflowContainer := executor.WorkflowDefinition != nil && executor.WorkflowDefinition.Container.IsEnabled() && !StepContainerDisabled(step)
	if step.Container.IsEnabled() || workflowContainer {
		return fmt.Errorf("%w: embedded starlark requires container: false", errUtils.ErrStarlark)
	}
	return nil
}

func (executor *ControlCommandExecutor) stepEnv(step *schema.WorkflowStep) ([]string, error) {
	if executor.PrepareEnv == nil {
		return executor.BaseEnv, nil
	}
	stepIdentity := strings.TrimSpace(step.Identity)
	if stepIdentity == "" {
		stepIdentity = strings.TrimSpace(executor.CommandLineIdentity)
	}
	var workflowEnv map[string]string
	if executor.WorkflowDefinition != nil {
		workflowEnv = executor.WorkflowDefinition.Env
	}
	return executor.PrepareEnv(executor.BaseEnv, stepIdentity, step.Name, workflowEnv, step.Env)
}

func (executor *ControlCommandExecutor) executeScript(ctx context.Context, step *schema.WorkflowStep, stepEnv []string, output ControlChildOutput) (*ControlChildResult, error) {
	if engine, ok := script.Get(step.Interpreter); ok {
		return executor.executeEmbeddedScript(ctx, step, stepEnv, output, engine)
	}
	argv, stdin := process.ScriptInvocation(step.Interpreter, step.Script)
	dir := executor.workingDirectory(step)
	ioSpec := executor.commandStreams(output)
	if stdin != nil {
		ioSpec.streams.Stdin = stdin
	}
	err := retry.Do(ctx, step.Retry, func() error {
		return executor.runCommand(&ControlCommandRequest{
			DryRun:  executor.DryRun,
			Context: ctx,
			Program: argv[0],
			Args:    argv[1:],
			Dir:     dir,
			Env:     stepEnv,
			Streams: ioSpec.streams,
			Stdout:  ioSpec.stdout,
			Stderr:  ioSpec.stderr,
		})
	})
	ioSpec.flush()
	err = stepPkg.WrapScriptInterpreterError(step.Interpreter, err)
	return controlChildExecutionResult(ioSpec.stdout, ioSpec.stderr, err), err
}

func (executor *ControlCommandExecutor) executeEmbeddedScript(ctx context.Context, step *schema.WorkflowStep, stepEnv []string, output ControlChildOutput, engine script.Engine) (*ControlChildResult, error) {
	if err := executor.rejectEmbeddedContainer(step); err != nil {
		return nil, err
	}
	if executor.DryRun {
		// The engine only parses in dry-run mode, so syntax errors still surface without executing anything.
		_, err := engine.Execute(ctx, script.Spec{Name: step.Name, Source: step.Script, SourcePath: step.ScriptSource, ProjectRoot: executor.ProjectRoot, WorkingDirectory: executor.workingDirectory(step), DryRun: true})
		return &ControlChildResult{}, err
	}
	ioSpec := executor.commandStreams(output)
	var embedded script.Result
	inputs := step.Env
	if step.ScriptEnv != nil {
		inputs = step.ScriptEnv
	}
	err := retry.Do(ctx, step.Retry, func() error {
		var runErr error
		embedded, runErr = engine.Execute(ctx, script.Spec{
			InstallTools: executor.InstallTools,
			Name:         step.Name, Source: step.Script, SourcePath: step.ScriptSource, ProjectRoot: executor.ProjectRoot, WorkingDirectory: executor.workingDirectory(step),
			Env: inputs, ProcessEnv: stepEnv, DryRun: step.DryRun,
			Component: executor.ScriptComponent, ResolveComponent: executor.ResolveComponent,
			ProcessOverrides: executor.processOverrides(stepEnv),
			Hook:             executor.ScriptHook,
			Stdout:           io.MultiWriter(ioSpec.streams.Stdout, ioSpec.stdout),
			Stderr:           io.MultiWriter(ioSpec.streams.Stderr, ioSpec.stderr),
		})
		return runErr
	})
	ioSpec.flush()
	result := controlChildExecutionResult(ioSpec.stdout, ioSpec.stderr, err)
	if embedded.HasOutput {
		result.Value = &embedded.Value
	}
	return result, err
}

// processOverrides returns the command-level env overrides for a child, with each
// value taken from the child's final process env so a child's own env keeps
// precedence over the command-level value.
func (executor *ControlCommandExecutor) processOverrides(stepEnv []string) map[string]string {
	if len(executor.ScriptProcessOverrides) == 0 {
		return nil
	}
	final := envpkg.SliceToMap(stepEnv)
	overrides := make(map[string]string, len(executor.ScriptProcessOverrides))
	for key, value := range executor.ScriptProcessOverrides {
		if resolved, ok := final[key]; ok {
			value = resolved
		}
		overrides[key] = value
	}
	return overrides
}

func (executor *ControlCommandExecutor) executeShell(ctx context.Context, step *schema.WorkflowStep, stepEnv []string, output ControlChildOutput) (*ControlChildResult, error) {
	ioSpec := executor.commandStreams(output)

	// When a ShellRunner is wired (registry bridge), run the command through the
	// in-process interpreter instead of shelling out. Output is teed to the live
	// stream (which masks via the data layer) and the capture buffer (kept raw for
	// downstream template references, matching the RunCommand path).
	if executor.ShellRunner != nil {
		if executor.DryRun {
			return &ControlChildResult{}, nil
		}
		outW := io.MultiWriter(ioSpec.streams.Stdout, ioSpec.stdout)
		errW := io.MultiWriter(ioSpec.streams.Stderr, ioSpec.stderr)
		dir := executor.workingDirectory(step)
		err := retry.Do(ctx, step.Retry, func() error {
			return executor.ShellRunner(ctx, &ControlShellRequest{
				Command: step.Command,
				Dir:     dir,
				Env:     stepEnv,
				Stdout:  outW,
				Stderr:  errW,
			})
		})
		ioSpec.flush()
		return controlChildExecutionResult(ioSpec.stdout, ioSpec.stderr, err), err
	}

	program, args := controlShellInvocation(step.Command)
	dir := executor.workingDirectory(step)
	err := retry.Do(ctx, step.Retry, func() error {
		return executor.runCommand(&ControlCommandRequest{
			DryRun:  executor.DryRun,
			Context: ctx,
			Program: program,
			Args:    args,
			Dir:     dir,
			Env:     stepEnv,
			Streams: ioSpec.streams,
			Stdout:  ioSpec.stdout,
			Stderr:  ioSpec.stderr,
		})
	})
	ioSpec.flush()
	return controlChildExecutionResult(ioSpec.stdout, ioSpec.stderr, err), err
}

func controlShellInvocation(command string) (string, []string) {
	return controlShellInvocationForOS(runtime.GOOS, os.Getenv("COMSPEC"), command) //nolint:forbidigo // COMSPEC is a Windows system variable, not Atmos configuration.
}

func controlShellInvocationForOS(goos, comspec, command string) (string, []string) {
	if goos == "windows" {
		program := strings.TrimSpace(comspec)
		if program == "" {
			program = "cmd.exe"
		}
		return program, []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}

func (executor *ControlCommandExecutor) executeAtmos(ctx context.Context, step *schema.WorkflowStep, stepEnv []string, output ControlChildOutput) (*ControlChildResult, error) {
	args, parseErr := shell.Fields(step.Command, nil)
	if parseErr != nil {
		args = strings.Fields(step.Command)
	}
	args = AppendAtmosStepFlags(args, AtmosStepFlags{
		Stack:  executor.finalStack(step),
		Tags:   executor.CommandLineTags,
		Labels: executor.CommandLineLabels,
	})
	dir := executor.workingDirectory(step)

	ioSpec := executor.commandStreams(output)
	err := retry.Do(ctx, step.Retry, func() error {
		return executor.runCommand(&ControlCommandRequest{
			DryRun:  executor.DryRun,
			Context: ctx,
			Program: "atmos",
			Args:    args,
			Dir:     dir,
			Env:     stepEnv,
			Streams: ioSpec.streams,
			Stdout:  ioSpec.stdout,
			Stderr:  ioSpec.stderr,
		})
	})
	ioSpec.flush()
	return controlChildExecutionResult(ioSpec.stdout, ioSpec.stderr, err), err
}

func (executor *ControlCommandExecutor) runCommand(request *ControlCommandRequest) error {
	if executor.RunCommand == nil {
		return fmt.Errorf("%w: control command runner is not configured", schema.ErrWorkflowControlStepInvalid)
	}
	return executor.RunCommand(request)
}

func (executor *ControlCommandExecutor) workingDirectory(step *schema.WorkflowStep) string {
	workflowDef := executor.WorkflowDefinition
	if workflowDef == nil {
		workflowDef = &schema.WorkflowDefinition{}
	}
	dir := CalculateWorkingDirectory(workflowDef, step, executor.BasePath)
	if dir == "" {
		return currentDir
	}
	return dir
}

type controlCommandIO struct {
	stdout  *bytes.Buffer
	stderr  *bytes.Buffer
	streams process.Streams
	flush   func()
}

func (executor *ControlCommandExecutor) commandStreams(output ControlChildOutput) controlCommandIO {
	ioSpec := controlCommandIO{
		stdout:  &bytes.Buffer{},
		stderr:  &bytes.Buffer{},
		streams: process.Streams{Stdin: os.Stdin, Stdout: io.Discard, Stderr: io.Discard},
		flush:   func() {},
	}
	if output.Mode == ControlOutputPrefixed {
		ioCtx := iolib.GetContext()
		stdoutWriter := iolib.NewLinePrefixWriter(output.Prefix, ioCtx.Data(), &executor.outputMu)
		stderrWriter := iolib.NewLinePrefixWriter(output.Prefix, ioCtx.UI(), &executor.outputMu)
		ioSpec.streams.Stdout = stdoutWriter
		ioSpec.streams.Stderr = stderrWriter
		ioSpec.flush = func() {
			_ = stdoutWriter.Flush()
			_ = stderrWriter.Flush()
		}
	}
	return ioSpec
}

func (executor *ControlCommandExecutor) finalStack(step *schema.WorkflowStep) string {
	finalStack := ""
	if executor.WorkflowDefinition != nil {
		finalStack = strings.TrimSpace(executor.WorkflowDefinition.Stack)
	}
	if strings.TrimSpace(step.Stack) != "" {
		finalStack = strings.TrimSpace(step.Stack)
	}
	if strings.TrimSpace(executor.CommandLineStack) != "" {
		finalStack = strings.TrimSpace(executor.CommandLineStack)
	}
	return finalStack
}

func executeControlSleep(ctx context.Context, step *schema.WorkflowStep) (*ControlChildResult, error) {
	duration := time.Second
	if strings.TrimSpace(step.Timeout) != "" {
		parsed, err := time.ParseDuration(step.Timeout)
		if err != nil {
			return &ControlChildResult{}, err
		}
		duration = parsed
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return &ControlChildResult{Canceled: true}, ctx.Err()
	case <-timer.C:
		return &ControlChildResult{}, nil
	}
}

// AtmosStepFlags are command-line flags applied to an Atmos workflow step.
type AtmosStepFlags struct {
	Stack  string
	Tags   []string
	Labels string
}

// AppendAtmosStepFlags inserts workflow command flags before a pass-through separator.
func AppendAtmosStepFlags(args []string, flags AtmosStepFlags) []string {
	defer perf.Track(nil, "workflow.AppendAtmosStepFlags")()

	injected := make([]string, 0, 4)
	if flags.Stack != "" {
		injected = append(injected, "-s", flags.Stack)
	}
	if len(flags.Tags) > 0 {
		injected = append(injected, "--tags="+strings.Join(flags.Tags, ","))
	}
	if flags.Labels != "" {
		injected = append(injected, "--labels="+flags.Labels)
	}
	if len(injected) == 0 {
		return args
	}
	if idx := indexOfControlArg(args, "--"); idx != -1 {
		return append(args[:idx], append(injected, args[idx:]...)...)
	}
	return append(args, injected...)
}

func indexOfControlArg(values []string, needle string) int {
	for i, value := range values {
		if value == needle {
			return i
		}
	}
	return -1
}

func controlChildExecutionResult(stdout, stderr *bytes.Buffer, err error) *ControlChildResult {
	return &ControlChildResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Canceled: errors.Is(err, context.Canceled),
	}
}

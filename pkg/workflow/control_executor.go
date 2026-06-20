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

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

type ControlEnvironmentFunc func(baseEnv []string, identity string, stepName string, workflowEnv map[string]string, stepEnv map[string]string) ([]string, error)

type ControlCommandRequest struct {
	Context context.Context
	Program string
	Args    []string
	Env     []string
	Streams process.Streams
	Stdout  *bytes.Buffer
	Stderr  *bytes.Buffer
}

type ControlCommandRunner func(request *ControlCommandRequest) error

type ControlCommandExecutor struct {
	WorkflowDefinition  *schema.WorkflowDefinition
	BaseEnv             []string
	CommandLineStack    string
	CommandLineIdentity string
	PrepareEnv          ControlEnvironmentFunc
	RunCommand          ControlCommandRunner

	outputMu sync.Mutex
}

func (executor *ControlCommandExecutor) Execute(ctx context.Context, child *ControlChild, output ControlChildOutput) (*ControlChildResult, error) {
	step := child.Step
	stepType := strings.TrimSpace(step.Type)
	if stepType == "" {
		stepType = schema.TaskTypeAtmos
	}

	switch stepType {
	case schema.TaskTypeShell, schema.TaskTypeAtmos:
		stepEnv, err := executor.stepEnv(&step)
		if err != nil {
			return &ControlChildResult{}, err
		}
		if stepType == schema.TaskTypeShell {
			return executor.executeShell(ctx, &step, stepEnv, output)
		}
		return executor.executeAtmos(ctx, &step, stepEnv, output)
	case "sleep":
		return executeControlSleep(ctx, &step)
	default:
		return &ControlChildResult{}, fmt.Errorf("%w: unsupported nested workflow step type %q", schema.ErrWorkflowControlStepInvalid, stepType)
	}
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

func (executor *ControlCommandExecutor) executeShell(ctx context.Context, step *schema.WorkflowStep, stepEnv []string, output ControlChildOutput) (*ControlChildResult, error) {
	ioSpec := executor.commandStreams(output)
	program, args := controlShellInvocation(step.Command)
	err := retry.Do(ctx, step.Retry, func() error {
		return executor.runCommand(&ControlCommandRequest{
			Context: ctx,
			Program: program,
			Args:    args,
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
	args = appendControlStack(args, executor.finalStack(step))

	ioSpec := executor.commandStreams(output)
	err := retry.Do(ctx, step.Retry, func() error {
		return executor.runCommand(&ControlCommandRequest{
			Context: ctx,
			Program: "atmos",
			Args:    args,
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

func appendControlStack(args []string, stack string) []string {
	if stack == "" {
		return args
	}
	if idx := indexOfControlArg(args, "--"); idx != -1 {
		return append(args[:idx], append([]string{"-s", stack}, args[idx:]...)...)
	}
	return append(args, "-s", stack)
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

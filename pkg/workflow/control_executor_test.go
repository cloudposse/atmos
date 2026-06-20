package workflow

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControlCommandExecutorExecuteShell(t *testing.T) {
	var gotIdentity string
	var gotWorkflowEnv map[string]string
	var gotStepEnv map[string]string
	var gotProgram string
	var gotArgs []string
	var gotEnv []string

	executor := &ControlCommandExecutor{
		WorkflowDefinition:  &schema.WorkflowDefinition{Env: map[string]string{"WORKFLOW": "true"}},
		BaseEnv:             []string{"BASE=true"},
		CommandLineIdentity: "cli-id",
		PrepareEnv: func(baseEnv []string, identity string, stepName string, workflowEnv map[string]string, stepEnv map[string]string) ([]string, error) {
			assert.Equal(t, []string{"BASE=true"}, baseEnv)
			assert.Equal(t, "build", stepName)
			gotIdentity = identity
			gotWorkflowEnv = workflowEnv
			gotStepEnv = stepEnv
			return []string{"MERGED=true"}, nil
		},
		RunCommand: func(request *ControlCommandRequest) error {
			require.NotNil(t, request.Context)
			require.NotNil(t, request.Streams.Stdin)
			require.NotNil(t, request.Streams.Stdout)
			require.NotNil(t, request.Streams.Stderr)
			gotProgram = request.Program
			gotArgs = append([]string{}, request.Args...)
			gotEnv = append([]string{}, request.Env...)
			request.Stdout.WriteString("build ok")
			request.Stderr.WriteString("build warning")
			return nil
		},
	}

	result, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{
		Name:    "build",
		Type:    schema.TaskTypeShell,
		Command: "make build",
		Env:     map[string]string{"STEP": "true"},
	}}, ControlChildOutput{Mode: ControlOutputNone})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "cli-id", gotIdentity)
	assert.Equal(t, map[string]string{"WORKFLOW": "true"}, gotWorkflowEnv)
	assert.Equal(t, map[string]string{"STEP": "true"}, gotStepEnv)
	wantProgram, wantArgs := controlShellInvocation("make build")
	assert.Equal(t, wantProgram, gotProgram)
	assert.Equal(t, wantArgs, gotArgs)
	assert.Equal(t, []string{"MERGED=true"}, gotEnv)
	assert.Equal(t, "build ok", result.Stdout)
	assert.Equal(t, "build warning", result.Stderr)
	assert.False(t, result.Canceled)
}

func TestControlShellInvocationForOS(t *testing.T) {
	t.Run("non-windows", func(t *testing.T) {
		program, args := controlShellInvocationForOS("linux", "", "make build")
		assert.Equal(t, "sh", program)
		assert.Equal(t, []string{"-c", "make build"}, args)
	})

	t.Run("windows default", func(t *testing.T) {
		program, args := controlShellInvocationForOS("windows", "", "make build")
		assert.Equal(t, "cmd.exe", program)
		assert.Equal(t, []string{"/C", "make build"}, args)
	})

	t.Run("windows comspec", func(t *testing.T) {
		program, args := controlShellInvocationForOS("windows", `C:\Windows\System32\cmd.exe`, "make build")
		assert.Equal(t, `C:\Windows\System32\cmd.exe`, program)
		assert.Equal(t, []string{"/C", "make build"}, args)
	})

	t.Run("current os", func(t *testing.T) {
		program, args := controlShellInvocation("make build")
		if runtime.GOOS == "windows" {
			assert.NotEmpty(t, program)
			assert.Equal(t, []string{"/C", "make build"}, args)
			return
		}
		assert.Equal(t, "sh", program)
		assert.Equal(t, []string{"-c", "make build"}, args)
	})
}

func TestControlCommandExecutorExecuteAtmosAddsStackBeforeTerminator(t *testing.T) {
	var gotProgram string
	var gotArgs []string

	executor := &ControlCommandExecutor{
		WorkflowDefinition: &schema.WorkflowDefinition{Stack: "workflow-stack"},
		PrepareEnv: func(baseEnv []string, identity string, stepName string, workflowEnv map[string]string, stepEnv map[string]string) ([]string, error) {
			return baseEnv, nil
		},
		RunCommand: func(request *ControlCommandRequest) error {
			gotProgram = request.Program
			gotArgs = append([]string{}, request.Args...)
			request.Stdout.WriteString("plan ok")
			return nil
		},
	}

	result, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{
		Name:    "plan",
		Type:    schema.TaskTypeAtmos,
		Command: `terraform plan -- -target='module.example'`,
		Stack:   "step-stack",
	}}, ControlChildOutput{Mode: ControlOutputNone})

	require.NoError(t, err)
	assert.Equal(t, "atmos", gotProgram)
	assert.Equal(t, []string{"terraform", "plan", "-s", "step-stack", "--", "-target=module.example"}, gotArgs)
	assert.Equal(t, "plan ok", result.Stdout)
}

func TestControlCommandExecutorExecuteUsesCommandLineStackOverride(t *testing.T) {
	var gotArgs []string
	executor := &ControlCommandExecutor{
		WorkflowDefinition: &schema.WorkflowDefinition{Stack: "workflow-stack"},
		CommandLineStack:   "cli-stack",
		PrepareEnv: func(baseEnv []string, identity string, stepName string, workflowEnv map[string]string, stepEnv map[string]string) ([]string, error) {
			return baseEnv, nil
		},
		RunCommand: func(request *ControlCommandRequest) error {
			gotArgs = append([]string{}, request.Args...)
			return nil
		},
	}

	_, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{
		Name:    "plan",
		Type:    schema.TaskTypeAtmos,
		Command: "terraform plan",
		Stack:   "step-stack",
	}}, ControlChildOutput{Mode: ControlOutputNone})

	require.NoError(t, err)
	assert.Equal(t, []string{"terraform", "plan", "-s", "cli-stack"}, gotArgs)
}

func TestControlCommandExecutorErrors(t *testing.T) {
	t.Run("prepare env", func(t *testing.T) {
		executor := &ControlCommandExecutor{
			PrepareEnv: func(baseEnv []string, identity string, stepName string, workflowEnv map[string]string, stepEnv map[string]string) ([]string, error) {
				return nil, errors.New("env failed")
			},
		}
		result, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{Name: "build", Type: schema.TaskTypeShell}}, ControlChildOutput{})
		require.Error(t, err)
		assert.Equal(t, "env failed", err.Error())
		assert.NotNil(t, result)
	})

	t.Run("missing runner", func(t *testing.T) {
		executor := &ControlCommandExecutor{}
		result, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{Name: "build", Type: schema.TaskTypeShell}}, ControlChildOutput{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "control command runner is not configured")
		assert.NotNil(t, result)
	})

	t.Run("unsupported type", func(t *testing.T) {
		executor := &ControlCommandExecutor{}
		result, err := executor.Execute(context.Background(), &ControlChild{Step: schema.WorkflowStep{Name: "prompt", Type: "input"}}, ControlChildOutput{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported nested workflow step type")
		assert.NotNil(t, result)
	})
}

func TestControlCommandExecutorSleep(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		result, err := executeControlSleep(context.Background(), &schema.WorkflowStep{Timeout: "1ms"})
		require.NoError(t, err)
		assert.False(t, result.Canceled)
	})

	t.Run("invalid duration", func(t *testing.T) {
		result, err := executeControlSleep(context.Background(), &schema.WorkflowStep{Timeout: "not-a-duration"})
		require.Error(t, err)
		assert.NotNil(t, result)
	})

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := executeControlSleep(ctx, &schema.WorkflowStep{Timeout: time.Hour.String()})
		require.Error(t, err)
		assert.True(t, result.Canceled)
	})
}

func TestAppendControlStack(t *testing.T) {
	assert.Equal(t, []string{"plan"}, appendControlStack([]string{"plan"}, ""))
	assert.Equal(t, []string{"plan", "-s", "dev"}, appendControlStack([]string{"plan"}, "dev"))
	assert.Equal(t, []string{"plan", "-s", "dev", "--", "-flag"}, appendControlStack([]string{"plan", "--", "-flag"}, "dev"))
	assert.Equal(t, 1, indexOfControlArg([]string{"a", "b"}, "b"))
	assert.Equal(t, -1, indexOfControlArg([]string{"a", "b"}, "c"))
}

func TestControlChildExecutionResultCanceled(t *testing.T) {
	stdout := bytes.NewBufferString("out")
	stderr := bytes.NewBufferString("err")
	result := controlChildExecutionResult(stdout, stderr, context.Canceled)
	assert.Equal(t, "out", result.Stdout)
	assert.Equal(t, "err", result.Stderr)
	assert.True(t, result.Canceled)
}

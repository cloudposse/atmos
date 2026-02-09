package step

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var shellInitOnce sync.Once

// initShellTestIO initializes the I/O context and data writer for shell tests.
func initShellTestIO(t *testing.T) {
	t.Helper()
	shellInitOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		data.InitWriter(ioCtx)
		ui.InitFormatter(ioCtx)
	})
}

// Shell handler registration is tested in command_handlers_test.go.
// Shell handler basic validation is tested in command_handlers_test.go.
// This file focuses on Execute() tests with real shell commands.

func TestShellHandlerExecution(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get("shell")
	require.True(t, ok)

	t.Run("simple echo command", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_echo",
			Type:    "shell",
			Command: "echo hello",
			Output:  "capture",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "hello")
		assert.Equal(t, 0, result.Metadata["exit_code"])
	})

	t.Run("command with exit code", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_exit",
			Type:    "shell",
			Command: "exit 42",
			Output:  "capture",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		assert.Error(t, err)
		assert.Equal(t, 42, result.Metadata["exit_code"])
	})

	t.Run("command with environment variables", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_env",
			Type:    "shell",
			Command: "echo $TEST_VAR",
			Output:  "capture",
			Env: map[string]string{
				"TEST_VAR": "custom_value",
			},
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "custom_value")
	})

	t.Run("command with template in command", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_template",
			Type:    "shell",
			Command: "echo {{ .steps.input.value }}",
			Output:  "capture",
		}
		vars := NewVariables()
		vars.Set("input", NewStepResult("template_value"))

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "template_value")
	})

	t.Run("command with working directory", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:             "test_workdir",
			Type:             "shell",
			Command:          "pwd",
			Output:           "capture",
			WorkingDirectory: "/tmp",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		require.NoError(t, err)
		// On macOS, /tmp is symlinked to /private/tmp.
		assert.True(t, strings.Contains(result.Value, "/tmp") || strings.Contains(result.Value, "/private/tmp"))
	})

	t.Run("context cancellation", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_cancel",
			Type:    "shell",
			Command: "sleep 10",
			Output:  "capture",
		}
		vars := NewVariables()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
	})

	t.Run("stderr capture", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_stderr",
			Type:    "shell",
			Command: "echo error >&2 && exit 1",
			Output:  "capture",
		}
		vars := NewVariables()

		result, err := handler.Execute(context.Background(), step, vars)
		assert.Error(t, err)
		assert.Contains(t, result.Metadata["stderr"], "error")
		assert.Equal(t, 1, result.Metadata["exit_code"])
	})
}

func TestShellHandlerExecuteWithWorkflow(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get("shell")
	require.True(t, ok)
	shellHandler := handler.(*ShellHandler)

	t.Run("with workflow output mode", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_workflow",
			Type:    "shell",
			Command: "echo workflow_test",
		}
		workflow := &schema.WorkflowDefinition{
			Output: "capture",
		}
		vars := NewVariables()

		result, err := shellHandler.ExecuteWithWorkflow(context.Background(), step, vars, workflow)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "workflow_test")
	})

	t.Run("step output overrides workflow", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_override",
			Type:    "shell",
			Command: "echo override_test",
			Output:  "capture",
		}
		workflow := &schema.WorkflowDefinition{
			Output: "log",
		}
		vars := NewVariables()

		result, err := shellHandler.ExecuteWithWorkflow(context.Background(), step, vars, workflow)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "override_test")
	})

	t.Run("with template in env", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_template_env",
			Type:    "shell",
			Command: "echo $MY_VAR",
			Output:  "capture",
			Env: map[string]string{
				"MY_VAR": "{{ .steps.value.value }}",
			},
		}
		workflow := &schema.WorkflowDefinition{}
		vars := NewVariables()
		vars.Set("value", NewStepResult("resolved_value"))

		result, err := shellHandler.ExecuteWithWorkflow(context.Background(), step, vars, workflow)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "resolved_value")
	})
}

func TestGetExitCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: 1, // Default for non-exit errors.
		},
		{
			name: "exec.ExitError with code 2",
			err: &exec.ExitError{
				ProcessState: nil, // Will use default.
			},
			expected: -1, // ProcessState is nil.
		},
		{
			name:     "generic error",
			err:      assert.AnError,
			expected: 1, // Default for non-exit errors.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				// Test that nil is handled - though in practice getExitCode is only called on error.
				return
			}
			result := getExitCode(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShellHandlerWithOutputModes(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get("shell")
	require.True(t, ok)

	outputModes := []string{"capture", "none", "raw", "log"}

	for _, mode := range outputModes {
		t.Run("output_mode_"+mode, func(t *testing.T) {
			step := &schema.WorkflowStep{
				Name:    "test_mode",
				Type:    "shell",
				Command: "echo test_output",
				Output:  mode,
			}
			vars := NewVariables()

			result, err := handler.Execute(context.Background(), step, vars)
			require.NoError(t, err)
			// In capture mode, we should have output.
			if mode == "capture" || mode == "raw" {
				assert.Contains(t, result.Value, "test_output")
			}
			// All modes should have exit code metadata.
			assert.Equal(t, 0, result.Metadata["exit_code"])
		})
	}
}

func TestShellHandlerExecuteWithWorkflowErrorCases(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get("shell")
	require.True(t, ok)
	shellHandler := handler.(*ShellHandler)

	t.Run("invalid command template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_invalid_cmd",
			Type:    "shell",
			Command: "echo {{ .invalid.template",
			Output:  "capture",
		}
		workflow := &schema.WorkflowDefinition{}
		vars := NewVariables()

		_, err := shellHandler.ExecuteWithWorkflow(context.Background(), step, vars, workflow)
		assert.Error(t, err)
	})

	t.Run("invalid workdir template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:             "test_invalid_workdir",
			Type:             "shell",
			Command:          "echo hello",
			WorkingDirectory: "{{ .invalid.template",
			Output:           "capture",
		}
		workflow := &schema.WorkflowDefinition{}
		vars := NewVariables()

		_, err := shellHandler.ExecuteWithWorkflow(context.Background(), step, vars, workflow)
		assert.Error(t, err)
	})

	t.Run("invalid env template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_invalid_env",
			Type:    "shell",
			Command: "echo hello",
			Output:  "capture",
			Env: map[string]string{
				"BAD": "{{ .invalid.template",
			},
		}
		workflow := &schema.WorkflowDefinition{}
		vars := NewVariables()

		_, err := shellHandler.ExecuteWithWorkflow(context.Background(), step, vars, workflow)
		assert.Error(t, err)
	})

	t.Run("with show config from workflow", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_show",
			Type:    "shell",
			Command: "echo show_test",
			Output:  "capture",
		}
		showCommand := true
		workflow := &schema.WorkflowDefinition{
			Show: &schema.ShowConfig{
				Command: &showCommand,
			},
		}
		vars := NewVariables()

		result, err := shellHandler.ExecuteWithWorkflow(context.Background(), step, vars, workflow)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "show_test")
	})

	t.Run("with show config from step", func(t *testing.T) {
		showCommand := true
		step := &schema.WorkflowStep{
			Name:    "test_step_show",
			Type:    "shell",
			Command: "echo step_show_test",
			Output:  "capture",
			Show: &schema.ShowConfig{
				Command: &showCommand,
			},
		}
		workflow := &schema.WorkflowDefinition{}
		vars := NewVariables()

		result, err := shellHandler.ExecuteWithWorkflow(context.Background(), step, vars, workflow)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "step_show_test")
	})

	t.Run("with nil workflow", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_nil_workflow",
			Type:    "shell",
			Command: "echo nil_workflow",
			Output:  "capture",
		}
		vars := NewVariables()

		result, err := shellHandler.ExecuteWithWorkflow(context.Background(), step, vars, nil)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "nil_workflow")
	})

	t.Run("command with stderr and success", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test_stderr_success",
			Type:    "shell",
			Command: "echo 'stdout' && echo 'stderr' >&2",
			Output:  "capture",
		}
		workflow := &schema.WorkflowDefinition{}
		vars := NewVariables()

		result, err := shellHandler.ExecuteWithWorkflow(context.Background(), step, vars, workflow)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "stdout")
		assert.Contains(t, result.Metadata["stderr"], "stderr")
	})
}

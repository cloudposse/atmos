package function

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockShellExecutor implements ShellExecutor for testing.
type mockShellExecutor struct {
	output string
	err    error
}

func (m *mockShellExecutor) Execute(ctx context.Context, command, workingDir string, env []string) (string, error) {
	return m.output, m.err
}

func TestNewExecFunction(t *testing.T) {
	fn := NewExecFunction(nil)

	assert.Equal(t, "exec", fn.Name())
	assert.Empty(t, fn.Aliases())
	assert.Equal(t, PreMerge, fn.Phase())
}

func TestExecFunctionExecute(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		output      string
		execError   error
		expected    any
		expectError bool
	}{
		{
			name:     "simple string output",
			args:     "echo hello",
			output:   "hello",
			expected: "hello",
		},
		{
			name:     "JSON output parsed",
			args:     "echo json",
			output:   `{"key": "value"}`,
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "JSON array output",
			args:     "echo array",
			output:   `[1, 2, 3]`,
			expected: []any{float64(1), float64(2), float64(3)},
		},
		{
			name:        "empty args returns error",
			args:        "",
			expectError: true,
		},
		{
			name:        "execution failure",
			args:        "failing-command",
			execError:   errors.New("command failed"),
			expectError: true,
		},
		{
			name:     "invalid JSON returns string",
			args:     "echo partial",
			output:   `{invalid json`,
			expected: `{invalid json`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockShellExecutor{
				output: tt.output,
				err:    tt.execError,
			}
			fn := NewExecFunction(executor)

			result, err := fn.Execute(context.Background(), tt.args, nil)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecFunctionWithoutExecutor(t *testing.T) {
	fn := NewExecFunction(nil)

	_, err := fn.Execute(context.Background(), "echo hello", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionFailed)
}

func TestExecFunctionWithContext(t *testing.T) {
	var capturedWorkingDir string
	var capturedEnv []string

	fn := NewExecFunction(&capturingExecutor{
		capturedWorkingDir: &capturedWorkingDir,
		capturedEnv:        &capturedEnv,
	})

	execCtx := &ExecutionContext{
		WorkingDir: "/custom/path",
		Env:        map[string]string{"KEY": "value"},
	}

	_, _ = fn.Execute(context.Background(), "test command", execCtx)

	assert.Equal(t, "/custom/path", capturedWorkingDir)
	assert.Contains(t, capturedEnv, "KEY=value")
}

// capturingExecutor captures execution parameters for testing.
type capturingExecutor struct {
	capturedWorkingDir *string
	capturedEnv        *[]string
}

func (c *capturingExecutor) Execute(ctx context.Context, command, workingDir string, env []string) (string, error) {
	*c.capturedWorkingDir = workingDir
	*c.capturedEnv = env
	return "", nil
}

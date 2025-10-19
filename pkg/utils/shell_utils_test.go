package utils

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestShellRunner(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		expectError bool
		expectCode  int
	}{
		{
			name:        "successful command",
			command:     "echo 'test'",
			expectError: false,
			expectCode:  0,
		},
		{
			name:        "command with exit code 0",
			command:     "exit 0",
			expectError: false,
			expectCode:  0,
		},
		{
			name:        "command with exit code 1",
			command:     "exit 1",
			expectError: true,
			expectCode:  1,
		},
		{
			name:        "command with exit code 2",
			command:     "exit 2",
			expectError: true,
			expectCode:  2,
		},
		{
			name:        "command with exit code 42",
			command:     "exit 42",
			expectError: true,
			expectCode:  42,
		},
		{
			name:        "command with exit code 127",
			command:     "exit 127",
			expectError: true,
			expectCode:  127,
		},
		{
			name:        "command not found",
			command:     "nonexistentcommand12345",
			expectError: true,
			expectCode:  127,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := ShellRunner(tt.command, tt.name, ".", nil, &buf)

			if tt.expectError {
				require.Error(t, err, "Expected error for command: %s", tt.command)

				// Verify the exit code is preserved.
				var exitCodeErr errUtils.ExitCodeError
				if errors.As(err, &exitCodeErr) {
					assert.Equal(t, tt.expectCode, exitCodeErr.Code,
						"Exit code should be %d for command: %s", tt.expectCode, tt.command)
				} else {
					t.Errorf("Expected ExitCodeError, got: %T", err)
				}
			} else {
				assert.NoError(t, err, "Command should succeed: %s", tt.command)
			}
		})
	}
}

// TestShellRunnerExitCodePreservation specifically tests that exit codes are preserved via errors.Join().
func TestShellRunnerExitCodePreservation(t *testing.T) {
	var buf bytes.Buffer
	err := ShellRunner("exit 99", "test-exit-99", ".", nil, &buf)

	require.Error(t, err, "Should return error for non-zero exit")

	// Verify ExitCodeError is wrapped in the error chain.
	var exitCodeErr errUtils.ExitCodeError
	require.True(t, errors.As(err, &exitCodeErr), "Error should contain ExitCodeError")
	assert.Equal(t, 99, exitCodeErr.Code, "ExitCodeError.Code should be 99")

	// Verify the error message contains diagnostic context from the interpreter.
	// The errors.Join() should preserve both the ExitCodeError and the original error.
	assert.Contains(t, err.Error(), "code 99", "Error message should contain exit code")
}

// TestShellRunnerErrorContext tests that error context is preserved via errors.Join().
func TestShellRunnerErrorContext(t *testing.T) {
	var buf bytes.Buffer

	// Test command that doesn't exist to get interpreter error.
	err := ShellRunner("command_that_does_not_exist_xyz", "test-not-found", ".", nil, &buf)

	require.Error(t, err, "Should return error for command not found")

	// Verify both ExitCodeError and original error context are preserved.
	var exitCodeErr errUtils.ExitCodeError
	require.True(t, errors.As(err, &exitCodeErr), "Error should contain ExitCodeError")
	assert.Equal(t, 127, exitCodeErr.Code, "Command not found should have exit code 127")

	// The error should contain both the exit code and context from the interpreter.
	errStr := err.Error()
	assert.Contains(t, errStr, "code 127", "Error should mention exit code")
	// Note: The exact error message from the interpreter may vary, but it should contain something about the command.
}

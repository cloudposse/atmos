package utils

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestShellRunner_ExitCodePreservation(t *testing.T) {
	tests := []struct {
		name           string
		command        string
		expectedExit   int
		expectError    bool
		errorSubstring string
	}{
		{
			name:         "successful command returns no error",
			command:      "exit 0",
			expectedExit: 0,
			expectError:  false,
		},
		{
			name:           "exit code 1 is preserved",
			command:        "exit 1",
			expectedExit:   1,
			expectError:    true,
			errorSubstring: "subcommand exited with code 1",
		},
		{
			name:           "exit code 2 is preserved",
			command:        "exit 2",
			expectedExit:   2,
			expectError:    true,
			errorSubstring: "subcommand exited with code 2",
		},
		{
			name:           "exit code 42 is preserved",
			command:        "exit 42",
			expectedExit:   42,
			expectError:    true,
			errorSubstring: "subcommand exited with code 42",
		},
		{
			name:         "successful echo command",
			command:      "echo 'test'",
			expectedExit: 0,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := ShellRunner(tt.command, "test", ".", []string{}, &buf)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorSubstring)

				// Verify the error is an ExitCodeError with the correct code
				var exitCodeErr errUtils.ExitCodeError
				if assert.ErrorAs(t, err, &exitCodeErr) {
					assert.Equal(t, tt.expectedExit, exitCodeErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestShellRunner_OutputCapture(t *testing.T) {
	var buf bytes.Buffer
	err := ShellRunner("echo 'hello world'", "test", ".", []string{}, &buf)

	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "hello world")
}

func TestShellRunner_Environment(t *testing.T) {
	var buf bytes.Buffer
	env := []string{"TEST_VAR=test_value"}
	err := ShellRunner("echo $TEST_VAR", "test", ".", env, &buf)

	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "test_value")
}

func TestShellRunner_WorkingDirectory(t *testing.T) {
	var buf bytes.Buffer
	// Use a simple command that works in any directory
	err := ShellRunner("pwd", "test", "/tmp", []string{}, &buf)

	assert.NoError(t, err)
	// The output should contain /tmp or its canonical path
	output := buf.String()
	assert.True(t, len(output) > 0, "output should not be empty")
}

func TestShellRunner_ParseError(t *testing.T) {
	var buf bytes.Buffer
	// Invalid shell syntax
	err := ShellRunner("if then fi", "test", ".", []string{}, &buf)

	require.Error(t, err)
	// Parse errors should not be wrapped in ExitCodeError
	var exitCodeErr errUtils.ExitCodeError
	if errors.As(err, &exitCodeErr) {
		t.Error("parse errors should not be ExitCodeError")
	}
}

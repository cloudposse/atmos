package shell

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestRunCommand_Validation(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		env           []string
		expectedError error
	}{
		{
			name:          "empty args returns error",
			args:          []string{},
			env:           nil,
			expectedError: errUtils.ErrNoCommandSpecified,
		},
		{
			name:          "nil args returns error",
			args:          nil,
			env:           nil,
			expectedError: errUtils.ErrNoCommandSpecified,
		},
		{
			name:          "command not found",
			args:          []string{"nonexistent-command-that-does-not-exist-12345"},
			env:           nil,
			expectedError: errUtils.ErrCommandNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunCommand(tt.args, tt.env)

			assert.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedError)

			// Regression guard for the "atmos requires a subcommand" bug: a missing
			// external executable must report ErrCommandNotFound and must NEVER be
			// classified as an unknown Atmos subcommand, or the root error handler
			// masks it with root usage output.
			if errors.Is(tt.expectedError, errUtils.ErrCommandNotFound) {
				assert.NotErrorIs(t, err, errUtils.ErrUnknownSubcommand)
			}
		})
	}
}

func TestRunCommand_WithValidCommand(t *testing.T) {
	// Cross-platform: spawn the test binary itself with the exit-OK env flag
	// (handled by TestMain). This avoids dependence on PATH-resolved binaries
	// like `go` / `true` which aren't available on every CI runner.
	exe, err := os.Executable()
	require.NoError(t, err)

	err = RunCommand([]string{exe}, []string{"_ATMOS_SHELL_TEST_EXIT_OK=1"})
	assert.NoError(t, err)
}

func TestRunCommand_NonZeroExit(t *testing.T) {
	// Cross-platform exit-1 subprocess: test binary + flag, expect ExitCodeError.
	exe, err := os.Executable()
	require.NoError(t, err)

	err = RunCommand([]string{exe}, []string{"_ATMOS_SHELL_TEST_EXIT_ONE=1"})
	require.Error(t, err)
	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr,
		"non-zero subprocess exit must surface as errUtils.ExitCodeError so the root can propagate the code")
	assert.Equal(t, 1, exitErr.Code)
}

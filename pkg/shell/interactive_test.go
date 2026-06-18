package shell

import (
	"os"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestDetermine(t *testing.T) {
	tests := []struct {
		name            string
		shellOverride   string
		shellArgs       []string
		expectedShell   string
		expectedArgs    []string
		skipOnWindows   bool
		setupViperShell string
	}{
		{
			name:          "override takes precedence",
			shellOverride: "/usr/bin/custom-shell",
			shellArgs:     []string{"-c", "echo test"},
			expectedShell: "/usr/bin/custom-shell",
			expectedArgs:  []string{"-c", "echo test"},
			skipOnWindows: true,
		},
		{
			name:          "default login shell with no args",
			shellOverride: "",
			shellArgs:     []string{},
			expectedArgs:  []string{"-l"},
			skipOnWindows: true,
		},
		{
			name:          "custom args provided",
			shellOverride: "",
			shellArgs:     []string{"-c", "exit 0"},
			expectedArgs:  []string{"-c", "exit 0"},
			skipOnWindows: true,
		},
		{
			name:            "viper shell value used",
			shellOverride:   "",
			shellArgs:       []string{},
			setupViperShell: "/bin/zsh",
			expectedShell:   "/bin/zsh",
			expectedArgs:    []string{"-l"},
			skipOnWindows:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == osWindows {
				t.Skipf("Skipping test on Windows: shell behavior differs")
			}

			if tt.setupViperShell != "" {
				viper.Set("shell", tt.setupViperShell)
				defer viper.Set("shell", "")
			}

			shellCommand, shellCommandArgs := Determine(tt.shellOverride, tt.shellArgs)

			if tt.expectedShell != "" {
				assert.Equal(t, tt.expectedShell, shellCommand)
			} else {
				assert.NotEmpty(t, shellCommand, "shell command should not be empty")
			}
			assert.Equal(t, tt.expectedArgs, shellCommandArgs)
		})
	}
}

func TestDetermine_Windows(t *testing.T) {
	if runtime.GOOS != osWindows {
		t.Skipf("Skipping Windows-specific test on non-Windows platform")
	}

	tests := []struct {
		name          string
		shellOverride string
		shellArgs     []string
		expectedShell string
		expectedArgs  []string
	}{
		{
			name:          "windows uses cmd.exe by default",
			shellOverride: "",
			shellArgs:     []string{},
			expectedShell: "cmd.exe",
			expectedArgs:  []string{},
		},
		{
			name:          "windows with custom args",
			shellOverride: "",
			shellArgs:     []string{"/c", "echo test"},
			expectedShell: "cmd.exe",
			expectedArgs:  []string{"/c", "echo test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shellCommand, shellCommandArgs := Determine(tt.shellOverride, tt.shellArgs)

			assert.Equal(t, tt.expectedShell, shellCommand)
			assert.Equal(t, tt.expectedArgs, shellCommandArgs)
		})
	}
}

func TestFindAvailableShell(t *testing.T) {
	if runtime.GOOS == osWindows {
		t.Skipf("Skipping Unix shell test on Windows")
	}

	shellPath := findAvailableShell()

	// Should find either bash or sh on Unix systems.
	assert.NotEmpty(t, shellPath, "should find a shell on Unix systems")
}

func TestStartInteractive_NoShell(t *testing.T) {
	// An empty shell command must surface ErrNoSuitableShell rather than panic.
	err := StartInteractive("", nil, nil)
	assert.ErrorIs(t, err, errUtils.ErrNoSuitableShell)
}

func TestStartInteractive_WithAbsoluteTestBinary(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	err = StartInteractive(exe, nil, []string{"_ATMOS_SHELL_TEST_EXIT_OK=1"})
	assert.NoError(t, err)
}

func TestStartInteractive_PropagatesExitCode(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	err = StartInteractive(exe, nil, []string{"_ATMOS_SHELL_TEST_EXIT_ONE=1"})
	require.Error(t, err)

	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.Code)
}

func TestStartInteractive_RelativeMissingShell(t *testing.T) {
	err := StartInteractive("definitely-missing-shell-for-atmos-test", nil, nil)
	assert.ErrorIs(t, err, errUtils.ErrNoSuitableShell)
}

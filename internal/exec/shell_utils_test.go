package exec

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMergeEnvVars(t *testing.T) {
	// Set up test environment variables
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("TF_CLI_ARGS_plan", "-lock=false")
	t.Setenv("HOME", "/home/test")

	// Atmos environment variables to merge
	componentEnv := []string{
		"TF_CLI_ARGS_plan=-compact-warnings",
		"ATMOS_VAR=value",
		"HOME=/overridden/home",
		"NEW_VAR=newvalue",
	}

	merged := envpkg.MergeSystemEnv(componentEnv)

	// Convert the merged list back to a map for easier assertions
	mergedMap := make(map[string]string)
	for _, env := range merged {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			mergedMap[parts[0]] = parts[1]
		}
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"PATH", "/usr/bin"}, // should be preserved
		{"TF_CLI_ARGS_plan", "-compact-warnings -lock=false"}, // prepended
		{"ATMOS_VAR", "value"},                                // new variable
		{"HOME", "/overridden/home"},                          // overridden
		{"NEW_VAR", "newvalue"},                               // added
	}

	for _, test := range tests {
		if val, ok := mergedMap[test.key]; !ok {
			t.Errorf("Missing key %q in merged environment", test.key)
		} else if val != test.expected {
			t.Errorf("Incorrect value for %q: expected %q, got %q", test.key, test.expected, val)
		}
	}
}

func TestGetAtmosShellLevel(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected int
	}{
		{
			name:     "no env var set",
			envValue: "",
			expected: 0,
		},
		{
			name:     "level 1",
			envValue: "1",
			expected: 1,
		},
		{
			name:     "level 5",
			envValue: "5",
			expected: 5,
		},
		{
			name:     "invalid value returns 0",
			envValue: "invalid",
			expected: 0,
		},
		{
			name:     "negative value",
			envValue: "-1",
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(atmosShellLevelEnvVar, tt.envValue)
			} else {
				os.Unsetenv(atmosShellLevelEnvVar)
			}

			result := getAtmosShellLevel()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetAtmosShellLevel(t *testing.T) {
	tests := []struct {
		name  string
		level int
	}{
		{
			name:  "set level 0",
			level: 0,
		},
		{
			name:  "set level 1",
			level: 1,
		},
		{
			name:  "set level 10",
			level: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setAtmosShellLevel(tt.level)
			require.NoError(t, err)

			result := getAtmosShellLevel()
			assert.Equal(t, tt.level, result)

			// Clean up.
			os.Unsetenv(atmosShellLevelEnvVar)
		})
	}
}

func TestDecrementAtmosShellLevel(t *testing.T) {
	tests := []struct {
		name     string
		initial  int
		expected int
	}{
		{
			name:     "decrement from 3 to 2",
			initial:  3,
			expected: 2,
		},
		{
			name:     "decrement from 1 to 0",
			initial:  1,
			expected: 0,
		},
		{
			name:     "decrement from 0 stays 0",
			initial:  0,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setAtmosShellLevel(tt.initial)
			require.NoError(t, err)

			decrementAtmosShellLevel()

			result := getAtmosShellLevel()
			assert.Equal(t, tt.expected, result)

			// Clean up.
			os.Unsetenv(atmosShellLevelEnvVar)
		})
	}
}

func TestConvertEnvMapToSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]string // Use map to check without caring about order.
	}{
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: map[string]string{},
		},
		{
			name: "single entry",
			input: map[string]string{
				"KEY1": "value1",
			},
			expected: map[string]string{
				"KEY1": "value1",
			},
		},
		{
			name: "multiple entries",
			input: map[string]string{
				"AWS_ACCESS_KEY_ID":     "AKIAEXAMPLE",
				"AWS_SECRET_ACCESS_KEY": "secretkey",
				"AWS_SESSION_TOKEN":     "token",
			},
			expected: map[string]string{
				"AWS_ACCESS_KEY_ID":     "AKIAEXAMPLE",
				"AWS_SECRET_ACCESS_KEY": "secretkey",
				"AWS_SESSION_TOKEN":     "token",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertEnvMapToSlice(tt.input)
			assert.Len(t, result, len(tt.expected))

			// Convert result back to map for easier comparison.
			resultMap := make(map[string]string)
			for _, entry := range result {
				key, value, found := parseEnvVar(entry)
				require.True(t, found, "invalid env var format: %s", entry)
				resultMap[key] = value
			}

			assert.Equal(t, tt.expected, resultMap)
		})
	}
}

func TestDetermineShell(t *testing.T) {
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
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skipf("Skipping test on Windows: shell behavior differs")
			}

			if tt.setupViperShell != "" {
				viper.Set("shell", tt.setupViperShell)
				defer viper.Set("shell", "")
			}

			shellCommand, shellCommandArgs := determineShell(tt.shellOverride, tt.shellArgs)

			if tt.expectedShell != "" {
				assert.Equal(t, tt.expectedShell, shellCommand)
			} else {
				assert.NotEmpty(t, shellCommand, "shell command should not be empty")
			}
			assert.Equal(t, tt.expectedArgs, shellCommandArgs)
		})
	}
}

func TestDetermineShell_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
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
			shellCommand, shellCommandArgs := determineShell(tt.shellOverride, tt.shellArgs)

			assert.Equal(t, tt.expectedShell, shellCommand)
			assert.Equal(t, tt.expectedArgs, shellCommandArgs)
		})
	}
}

func TestFindAvailableShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("Skipping Unix shell test on Windows")
	}

	shellPath := findAvailableShell()

	// Should find either bash or sh on Unix systems.
	assert.NotEmpty(t, shellPath, "should find a shell on Unix systems")
}

func TestMergeEnvVarsSimple(t *testing.T) {
	tests := []struct {
		name        string
		newEnvList  []string
		expectedEnv map[string]string
	}{
		{
			name:        "empty list",
			newEnvList:  []string{},
			expectedEnv: map[string]string{},
		},
		{
			name: "single new var",
			newEnvList: []string{
				"NEW_VAR=new_value",
			},
			expectedEnv: map[string]string{
				"NEW_VAR": "new_value",
			},
		},
		{
			name: "multiple new vars",
			newEnvList: []string{
				"VAR1=value1",
				"VAR2=value2",
				"VAR3=value3",
			},
			expectedEnv: map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
				"VAR3": "value3",
			},
		},
		{
			name: "override existing var",
			newEnvList: []string{
				"PATH=/custom/path",
			},
			expectedEnv: map[string]string{
				"PATH": "/custom/path",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := envpkg.MergeSystemEnvSimple(tt.newEnvList)

			// Convert result to map for easier comparison.
			resultMap := make(map[string]string)
			for _, entry := range result {
				key, value, found := parseEnvVar(entry)
				require.True(t, found, "invalid env var format: %s", entry)
				resultMap[key] = value
			}

			// Check that expected vars are present with correct values.
			for key, expectedValue := range tt.expectedEnv {
				actualValue, found := resultMap[key]
				assert.True(t, found, "expected env var %s not found", key)
				assert.Equal(t, expectedValue, actualValue, "env var %s has wrong value", key)
			}
		})
	}
}

// parseEnvVar is a helper function to parse env var string.
func parseEnvVar(envVar string) (key string, value string, found bool) {
	for i := 0; i < len(envVar); i++ {
		if envVar[i] == '=' {
			return envVar[:i], envVar[i+1:], true
		}
	}
	return "", "", false
}

func TestExecAuthShellCommand_ExitCodePropagation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("Skipping Unix shell test on Windows")
	}

	if testing.Short() {
		t.Skipf("Skipping test in short mode: spawns actual shell process")
	}

	tests := []struct {
		name         string
		shellArgs    []string
		expectedCode int
	}{
		{
			name:         "exit code 0",
			shellArgs:    []string{"-c", "exit 0"},
			expectedCode: 0,
		},
		{
			name:         "exit code 1",
			shellArgs:    []string{"-c", "exit 1"},
			expectedCode: 1,
		},
		{
			name:         "exit code 42",
			shellArgs:    []string{"-c", "exit 42"},
			expectedCode: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVars := map[string]string{
				"TEST_VAR": "test_value",
			}
			atmosConfig := &schema.AtmosConfiguration{}

			err := ExecAuthShellCommand(atmosConfig, "test-identity", "test-provider", envVars, "/bin/sh", tt.shellArgs)

			if tt.expectedCode == 0 {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				var exitCodeErr errUtils.ExitCodeError
				require.True(t, errors.As(err, &exitCodeErr), "error should be ExitCodeError")
				assert.Equal(t, tt.expectedCode, exitCodeErr.Code)
			}
		})
	}
}

func TestExecuteShellCommand(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}

	t.Run("dry run mode", func(t *testing.T) {
		err := ExecuteShellCommand(
			atmosConfig,
			"echo",
			[]string{"test"},
			".",
			nil,
			true, // dry run
			"",
		)
		assert.NoError(t, err, "dry run should not execute command")
	})

	t.Run("redirect to /dev/stderr", func(t *testing.T) {
		err := ExecuteShellCommand(
			atmosConfig,
			"echo",
			[]string{"test"},
			".",
			nil,
			false,
			"/dev/stderr",
		)
		assert.NoError(t, err)
	})

	t.Run("redirect to /dev/stdout", func(t *testing.T) {
		err := ExecuteShellCommand(
			atmosConfig,
			"echo",
			[]string{"test"},
			".",
			nil,
			false,
			"/dev/stdout",
		)
		assert.NoError(t, err)
	})

	t.Run("redirect to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "test.log")

		err := ExecuteShellCommand(
			atmosConfig,
			"echo",
			[]string{"test output"},
			".",
			nil,
			false,
			logFile,
		)
		assert.NoError(t, err)

		// Verify file was created and contains output
		_, err = os.Stat(logFile)
		assert.NoError(t, err, "log file should exist")
	})

	t.Run("windows NUL redirect", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("Windows-specific test")
		}

		err := ExecuteShellCommand(
			atmosConfig,
			"echo",
			[]string{"test"},
			".",
			nil,
			false,
			"/dev/null",
		)
		assert.NoError(t, err)
	})

	t.Run("invalid redirect file path", func(t *testing.T) {
		err := ExecuteShellCommand(
			atmosConfig,
			"echo",
			[]string{"test"},
			".",
			nil,
			false,
			"/nonexistent/directory/output.log",
		)
		assert.Error(t, err, "should fail to create file in nonexistent directory")
	})

	t.Run("custom environment variables", func(t *testing.T) {
		err := ExecuteShellCommand(
			atmosConfig,
			"echo",
			[]string{"test"},
			".",
			[]string{"CUSTOM_VAR=value"},
			false,
			"",
		)
		assert.NoError(t, err)
	})
}

func TestExecuteShell(t *testing.T) {
	t.Run("simple echo command", func(t *testing.T) {
		err := ExecuteShell(
			"echo 'test'",
			"test-shell",
			".",
			nil,
			false,
		)
		assert.NoError(t, err)
	})

	t.Run("dry run mode", func(t *testing.T) {
		err := ExecuteShell(
			"echo 'test'",
			"test-shell",
			".",
			nil,
			true, // dry run
		)
		assert.NoError(t, err)
	})

	t.Run("syntax error in shell command", func(t *testing.T) {
		err := ExecuteShell(
			"echo 'unclosed quote",
			"test-shell",
			".",
			nil,
			false,
		)
		assert.Error(t, err, "should fail on syntax error")
	})

	t.Run("custom environment variables", func(t *testing.T) {
		err := ExecuteShell(
			"echo test",
			"test-shell",
			".",
			[]string{"CUSTOM_VAR=value"},
			false,
		)
		assert.NoError(t, err)
	})

	t.Run("custom env vars override parent env", func(t *testing.T) {
		// Test that custom env vars can override parent environment variables
		// while still inheriting everything else like PATH.
		if runtime.GOOS == "windows" {
			t.Skip("Skipping Unix-specific test on Windows")
		}

		// Set a test environment variable in the parent process.
		t.Setenv("TEST_OVERRIDE", "parent_value")

		// Pass a custom env that overrides TEST_OVERRIDE.
		// The shell command should see the override value, not the parent value,
		// but should still have access to PATH.
		err := ExecuteShell(
			"test \"$TEST_OVERRIDE\" = \"custom_value\" && env | grep -q PATH",
			"test-shell",
			".",
			[]string{"TEST_OVERRIDE=custom_value"},
			false,
		)
		assert.NoError(t, err, "custom env var should override parent, and PATH should be inherited")
	})

	t.Run("empty env should inherit PATH from parent process", func(t *testing.T) {
		// This test demonstrates the bug reported in DEV-3725.
		// When ExecuteShell is called with an empty env slice (as workflow commands do),
		// it should still be able to find executables in PATH.
		// However, due to a bug, ExecuteShell appends ATMOS_SHLVL to the empty slice,
		// making it non-empty, which causes ShellRunner to use ONLY that env variable,
		// losing PATH and all other environment variables.

		if runtime.GOOS == "windows" {
			t.Skip("Skipping Unix-specific PATH test on Windows")
		}

		// Call ExecuteShell with empty env slice, just like workflow commands do.
		err := ExecuteShell(
			"env",
			"test-shell",
			".",
			[]string{}, // Empty slice - should fallback to os.Environ()
			false,
		)

		// This currently fails with "env": executable file not found in $PATH
		// because ExecuteShell adds ATMOS_SHLVL to the empty slice, making it non-empty,
		// and ShellRunner then uses ONLY that environment, losing PATH.
		assert.NoError(t, err, "should be able to execute 'env' command when PATH is inherited")
	})

	t.Run("empty env should inherit PATH for common commands", func(t *testing.T) {
		// Test various common Unix commands that should be findable in PATH.
		// Note: Some commands like 'pwd' and 'echo' are shell builtins and don't require PATH,
		// but external executables like 'ls' and 'env' require PATH to be set.
		if runtime.GOOS == "windows" {
			t.Skip("Skipping Unix-specific PATH test on Windows")
		}

		testCases := []struct {
			cmd          string
			requiresPATH bool
		}{
			{"ls", true},          // External executable - requires PATH
			{"env", true},         // External executable - requires PATH
			{"pwd", false},        // Shell builtin - works without PATH
			{"echo hello", false}, // Shell builtin - works without PATH
		}

		for _, tc := range testCases {
			t.Run(tc.cmd, func(t *testing.T) {
				err := ExecuteShell(
					tc.cmd,
					"test-shell",
					".",
					[]string{}, // Empty slice - workflow commands pass this
					false,
				)
				if tc.requiresPATH {
					// These currently fail due to the bug - PATH is not inherited.
					assert.NoError(t, err, "should be able to execute '%s' when PATH is inherited", tc.cmd)
				} else {
					// Shell builtins work even without PATH.
					assert.NoError(t, err, "shell builtin '%s' should work", tc.cmd)
				}
			})
		}
	})
}

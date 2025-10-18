package exec

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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

	merged := mergeEnvVars(componentEnv)

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
			result := mergeEnvVarsSimple(tt.newEnvList)

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

			err := ExecAuthShellCommand(nil, "test-identity", envVars, "/bin/sh", tt.shellArgs)

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

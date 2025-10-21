package exec

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

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
}

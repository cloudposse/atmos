package step

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var spinInitOnce sync.Once

// initSpinTestIO initializes the I/O context for spin tests.
func initSpinTestIO(t *testing.T) {
	t.Helper()
	spinInitOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
	})
}

// SpinHandler registration and validation are tested in command_handlers_test.go.
// This file tests the helper methods.

func TestSpinHandler_PrepareExecution(t *testing.T) {
	handler, ok := Get("spin")
	require.True(t, ok)
	spinHandler := handler.(*SpinHandler)

	t.Run("basic preparation", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "spin",
			Command: "echo hello",
			Title:   "Running test",
		}
		vars := NewVariables()
		ctx := context.Background()

		opts, err := spinHandler.prepareExecution(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "echo hello", opts.command)
		assert.Empty(t, opts.workDir)
		assert.NotNil(t, opts.envVars)
	})

	t.Run("with working directory", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:             "test",
			Type:             "spin",
			Command:          "ls",
			Title:            "Listing",
			WorkingDirectory: "/tmp",
		}
		vars := NewVariables()
		ctx := context.Background()

		opts, err := spinHandler.prepareExecution(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "/tmp", opts.workDir)
	})

	t.Run("with template in command", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "spin",
			Command: "echo {{ .steps.msg.value }}",
			Title:   "Echo test",
		}
		vars := NewVariables()
		vars.Set("msg", NewStepResult("hello world"))
		ctx := context.Background()

		opts, err := spinHandler.prepareExecution(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "echo hello world", opts.command)
	})

	t.Run("with template in working directory", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:             "test",
			Type:             "spin",
			Command:          "ls",
			Title:            "Listing",
			WorkingDirectory: "{{ .steps.dir.value }}/components",
		}
		vars := NewVariables()
		vars.Set("dir", NewStepResult("/project"))
		ctx := context.Background()

		opts, err := spinHandler.prepareExecution(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "/project/components", opts.workDir)
	})

	t.Run("with environment variables", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "spin",
			Command: "echo $MY_VAR",
			Title:   "Echo test",
			Env: map[string]string{
				"MY_VAR": "test_value",
			},
		}
		vars := NewVariables()
		ctx := context.Background()

		opts, err := spinHandler.prepareExecution(ctx, step, vars)
		require.NoError(t, err)

		// Check that MY_VAR is in the env vars.
		found := false
		for _, env := range opts.envVars {
			if env == "MY_VAR=test_value" {
				found = true
				break
			}
		}
		assert.True(t, found, "MY_VAR should be in env vars")
	})

	t.Run("with template in env", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "spin",
			Command: "echo $TARGET",
			Title:   "Echo test",
			Env: map[string]string{
				"TARGET": "{{ .steps.env.value }}",
			},
		}
		vars := NewVariables()
		vars.Set("env", NewStepResult("production"))
		ctx := context.Background()

		opts, err := spinHandler.prepareExecution(ctx, step, vars)
		require.NoError(t, err)

		// Check that TARGET=production is in the env vars.
		found := false
		for _, env := range opts.envVars {
			if env == "TARGET=production" {
				found = true
				break
			}
		}
		assert.True(t, found, "TARGET=production should be in env vars")
	})

	t.Run("invalid command template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "spin",
			Command: "echo {{ .steps.invalid.value",
			Title:   "Echo test",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := spinHandler.prepareExecution(ctx, step, vars)
		assert.Error(t, err)
	})

	t.Run("invalid workdir template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:             "test",
			Type:             "spin",
			Command:          "ls",
			Title:            "Listing",
			WorkingDirectory: "{{ .steps.invalid.value",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := spinHandler.prepareExecution(ctx, step, vars)
		assert.Error(t, err)
	})

	t.Run("invalid env template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "spin",
			Command: "echo $BAD",
			Title:   "Echo test",
			Env: map[string]string{
				"BAD": "{{ .steps.invalid.value",
			},
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := spinHandler.prepareExecution(ctx, step, vars)
		assert.Error(t, err)
	})
}

func TestSpinHandler_CreateExecContext(t *testing.T) {
	handler, ok := Get("spin")
	require.True(t, ok)
	spinHandler := handler.(*SpinHandler)

	t.Run("no timeout", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "spin",
			Command: "echo hello",
		}
		ctx := context.Background()

		execCtx, cancel := spinHandler.createExecContext(ctx, step)
		assert.Equal(t, ctx, execCtx)
		assert.Nil(t, cancel)
	})

	t.Run("with valid timeout", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "spin",
			Command: "echo hello",
			Timeout: "5s",
		}
		ctx := context.Background()

		execCtx, cancel := spinHandler.createExecContext(ctx, step)
		assert.NotEqual(t, ctx, execCtx)
		assert.NotNil(t, cancel)
		cancel()
	})

	t.Run("with invalid timeout", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "spin",
			Command: "echo hello",
			Timeout: "invalid",
		}
		ctx := context.Background()

		execCtx, cancel := spinHandler.createExecContext(ctx, step)
		assert.Equal(t, ctx, execCtx)
		assert.Nil(t, cancel)
	})

	t.Run("with zero timeout", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "spin",
			Command: "echo hello",
			Timeout: "0s",
		}
		ctx := context.Background()

		execCtx, cancel := spinHandler.createExecContext(ctx, step)
		assert.Equal(t, ctx, execCtx)
		assert.Nil(t, cancel)
	})

	t.Run("with negative timeout", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "spin",
			Command: "echo hello",
			Timeout: "-5s",
		}
		ctx := context.Background()

		execCtx, cancel := spinHandler.createExecContext(ctx, step)
		assert.Equal(t, ctx, execCtx)
		assert.Nil(t, cancel)
	})
}

func TestSpinHandler_BuildResult(t *testing.T) {
	handler, ok := Get("spin")
	require.True(t, ok)
	spinHandler := handler.(*SpinHandler)

	t.Run("success result", func(t *testing.T) {
		result := spinHandler.buildResult("stdout content", "stderr content", nil)
		assert.Equal(t, "stdout content", result.Value)
		assert.Equal(t, "stdout content", result.Metadata["stdout"])
		assert.Equal(t, "stderr content", result.Metadata["stderr"])
		assert.Empty(t, result.Error)
	})

	t.Run("error result", func(t *testing.T) {
		result := spinHandler.buildResult("partial stdout", "error message", assert.AnError)
		assert.Equal(t, "", result.Value)
		assert.Equal(t, "partial stdout", result.Metadata["stdout"])
		assert.Equal(t, "error message", result.Metadata["stderr"])
		assert.Equal(t, "error message", result.Error)
	})

	t.Run("empty stdout on success", func(t *testing.T) {
		result := spinHandler.buildResult("", "", nil)
		assert.Equal(t, "", result.Value)
		assert.Equal(t, "", result.Metadata["stdout"])
		assert.Equal(t, "", result.Metadata["stderr"])
		assert.Empty(t, result.Error)
	})
}

// getPwdCommand returns a platform-specific command to print the current directory.
func getPwdCommand() string {
	if runtime.GOOS == "windows" {
		return "cd"
	}
	return "pwd"
}

// getEchoEnvCommand returns a platform-specific command to echo an environment variable.
func getEchoEnvCommand(varName string) string {
	if runtime.GOOS == "windows" {
		return "echo %" + varName + "%"
	}
	return "echo $" + varName
}

// getTempDir returns a platform-specific temp directory path.
func getTempDir() string {
	if runtime.GOOS == "windows" {
		return "C:\\Windows\\Temp"
	}
	return "/tmp"
}

// assertContainsTempDir checks if the output contains a valid temp directory path.
func assertContainsTempDir(t *testing.T, output string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Windows cd command outputs the current directory.
		assert.True(t, strings.Contains(strings.ToLower(output), "temp"),
			"Output should contain temp directory path, got: %s", output)
	} else {
		// On macOS, /tmp is symlinked to /private/tmp.
		assert.True(t, strings.Contains(output, "/tmp") || strings.Contains(output, "/private/tmp"),
			"Output should contain /tmp or /private/tmp, got: %s", output)
	}
}

func TestSpinHandler_RunCommand(t *testing.T) {
	initSpinTestIO(t)
	handler, ok := Get("spin")
	require.True(t, ok)
	spinHandler := handler.(*SpinHandler)

	t.Run("simple echo command", func(t *testing.T) {
		opts := &spinExecOptions{
			command: "echo hello",
			envVars: []string{},
		}
		var stdout, stderr bytes.Buffer
		ctx := context.Background()

		err := spinHandler.runCommand(ctx, opts, &stdout, &stderr)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "hello")
		assert.Empty(t, stderr.String())
	})

	t.Run("command with stderr", func(t *testing.T) {
		opts := &spinExecOptions{
			command: "echo 'error' >&2",
			envVars: []string{},
		}
		var stdout, stderr bytes.Buffer
		ctx := context.Background()

		err := spinHandler.runCommand(ctx, opts, &stdout, &stderr)
		require.NoError(t, err)
		assert.Contains(t, stderr.String(), "error")
	})

	t.Run("failing command", func(t *testing.T) {
		opts := &spinExecOptions{
			command: "exit 42",
			envVars: []string{},
		}
		var stdout, stderr bytes.Buffer
		ctx := context.Background()

		err := spinHandler.runCommand(ctx, opts, &stdout, &stderr)
		assert.Error(t, err)
	})

	t.Run("empty command", func(t *testing.T) {
		opts := &spinExecOptions{
			command: "",
			envVars: []string{},
		}
		var stdout, stderr bytes.Buffer
		ctx := context.Background()

		err := spinHandler.runCommand(ctx, opts, &stdout, &stderr)
		assert.Error(t, err)
	})

	t.Run("command with workdir", func(t *testing.T) {
		opts := &spinExecOptions{
			command: getPwdCommand(),
			workDir: getTempDir(),
			envVars: []string{},
		}
		var stdout, stderr bytes.Buffer
		ctx := context.Background()

		err := spinHandler.runCommand(ctx, opts, &stdout, &stderr)
		require.NoError(t, err)
		assertContainsTempDir(t, stdout.String())
	})

	t.Run("command with env vars", func(t *testing.T) {
		envVars := []string{"MY_TEST_VAR=test_value"}
		if runtime.GOOS == "windows" {
			envVars = append(envVars, "PATH=C:\\Windows\\System32")
		} else {
			envVars = append(envVars, "PATH=/usr/bin:/bin")
		}
		opts := &spinExecOptions{
			command: getEchoEnvCommand("MY_TEST_VAR"),
			envVars: envVars,
		}
		var stdout, stderr bytes.Buffer
		ctx := context.Background()

		err := spinHandler.runCommand(ctx, opts, &stdout, &stderr)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "test_value")
	})

	t.Run("context cancellation", func(t *testing.T) {
		opts := &spinExecOptions{
			command: "sleep 10",
			envVars: []string{},
		}
		var stdout, stderr bytes.Buffer
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately.

		err := spinHandler.runCommand(ctx, opts, &stdout, &stderr)
		assert.Error(t, err)
	})
}

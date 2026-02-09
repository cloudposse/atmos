package step

import (
	"os/exec"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

var outputModeInitOnce sync.Once

// initOutputModeTestIO initializes the I/O context for output mode tests.
func initOutputModeTestIO(t *testing.T) {
	t.Helper()
	outputModeInitOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		ui.InitFormatter(ioCtx)
	})
}

// OutputModeWriter creation is tested in output_mode_test.go.
// GetOutputMode is tested in command_handlers_test.go.
// GetViewportConfig is tested in command_handlers_test.go.
// FormatStepLabel is tested in output_mode_test.go.
// RenderCommand is tested in output_mode_test.go.
// StreamingOutputWriter is tested in output_mode_test.go.
// formatStepFooter is tested in output_mode_test.go.
// This file tests the Execute() method variations.

func TestOutputModeWriterExecuteNone(t *testing.T) {
	writer := NewOutputModeWriter(OutputModeNone, "test_step", nil)

	// Execute simple echo command.
	cmd := exec.Command("echo", "hello world")
	stdout, stderr, err := writer.Execute(cmd)

	require.NoError(t, err)
	assert.Contains(t, stdout, "hello world")
	assert.Empty(t, stderr)
}

func TestOutputModeWriterExecuteNoneWithStderr(t *testing.T) {
	writer := NewOutputModeWriter(OutputModeNone, "test_step", nil)

	// Execute command that writes to stderr.
	cmd := exec.Command("sh", "-c", "echo 'error message' >&2")
	stdout, stderr, err := writer.Execute(cmd)

	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "error message")
}

func TestOutputModeWriterExecuteNoneWithError(t *testing.T) {
	writer := NewOutputModeWriter(OutputModeNone, "test_step", nil)

	// Execute command that fails.
	cmd := exec.Command("sh", "-c", "exit 42")
	stdout, stderr, err := writer.Execute(cmd)

	assert.Error(t, err)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)

	// Check exit code.
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 42, exitErr.ExitCode())
}

func TestOutputModeWriterExecuteRaw(t *testing.T) {
	writer := NewOutputModeWriter(OutputModeRaw, "test_step", nil)

	// Execute simple echo command.
	cmd := exec.Command("echo", "raw output")
	stdout, stderr, err := writer.Execute(cmd)

	require.NoError(t, err)
	assert.Contains(t, stdout, "raw output")
	assert.Empty(t, stderr)
}

func TestOutputModeWriterExecuteRawWithStderr(t *testing.T) {
	writer := NewOutputModeWriter(OutputModeRaw, "test_step", nil)

	// Execute command that writes to both stdout and stderr.
	cmd := exec.Command("sh", "-c", "echo 'stdout' && echo 'stderr' >&2")
	stdout, stderr, err := writer.Execute(cmd)

	require.NoError(t, err)
	assert.Contains(t, stdout, "stdout")
	assert.Contains(t, stderr, "stderr")
}

func TestOutputModeWriterExecuteLog(t *testing.T) {
	initOutputModeTestIO(t)
	writer := NewOutputModeWriter(OutputModeLog, "test_step", nil)

	// Execute simple echo command.
	cmd := exec.Command("echo", "logged output")
	stdout, stderr, err := writer.Execute(cmd)

	require.NoError(t, err)
	assert.Contains(t, stdout, "logged output")
	assert.Empty(t, stderr)
}

func TestOutputModeWriterExecuteLogWithError(t *testing.T) {
	initOutputModeTestIO(t)
	writer := NewOutputModeWriter(OutputModeLog, "failing_step", nil)

	// Execute command that fails.
	cmd := exec.Command("sh", "-c", "echo 'partial output' && exit 1")
	stdout, stderr, err := writer.Execute(cmd)

	assert.Error(t, err)
	assert.Contains(t, stdout, "partial output")
	assert.Empty(t, stderr)
}

func TestOutputModeWriterExecuteLogWithStderr(t *testing.T) {
	initOutputModeTestIO(t)
	writer := NewOutputModeWriter(OutputModeLog, "test_step", nil)

	// Execute command that writes to stderr.
	cmd := exec.Command("sh", "-c", "echo 'stdout' && echo 'stderr' >&2")
	stdout, stderr, err := writer.Execute(cmd)

	require.NoError(t, err)
	assert.Contains(t, stdout, "stdout")
	assert.Contains(t, stderr, "stderr")
}

func TestOutputModeWriterExecuteDefaultMode(t *testing.T) {
	initOutputModeTestIO(t)
	// Use an invalid mode to trigger default behavior (log mode).
	writer := NewOutputModeWriter(OutputMode("invalid"), "test_step", nil)

	// Execute simple echo command.
	cmd := exec.Command("echo", "default mode")
	stdout, stderr, err := writer.Execute(cmd)

	require.NoError(t, err)
	assert.Contains(t, stdout, "default mode")
	assert.Empty(t, stderr)
}

func TestOutputModeWriterFallbackToLog(t *testing.T) {
	initOutputModeTestIO(t)
	writer := NewOutputModeWriter(OutputModeLog, "test_step", nil)

	// Test fallbackToLog directly.
	stdout, stderr, err := writer.fallbackToLog("stdout content", "stderr content", nil)

	require.NoError(t, err)
	assert.Equal(t, "stdout content", stdout)
	assert.Equal(t, "stderr content", stderr)
}

func TestOutputModeWriterFallbackToLogWithError(t *testing.T) {
	initOutputModeTestIO(t)
	writer := NewOutputModeWriter(OutputModeLog, "test_step", nil)

	// Test fallbackToLog with an error.
	stdout, stderr, err := writer.fallbackToLog("stdout content", "stderr content", assert.AnError)

	assert.Error(t, err)
	assert.Equal(t, "stdout content", stdout)
	assert.Equal(t, "stderr content", stderr)
}

func TestOutputModeWriterExecuteViewport(t *testing.T) {
	// Viewport mode requires TTY, which we don't have in tests.
	// It should fall back to log mode when TTY is not available.
	initOutputModeTestIO(t)
	writer := NewOutputModeWriter(OutputModeViewport, "test_step", nil)

	// Execute simple echo command.
	cmd := exec.Command("echo", "viewport output")
	stdout, stderr, err := writer.Execute(cmd)

	require.NoError(t, err)
	assert.Contains(t, stdout, "viewport output")
	assert.Empty(t, stderr)
}

func TestOutputModeWriterExecuteViewportWithError(t *testing.T) {
	initOutputModeTestIO(t)
	writer := NewOutputModeWriter(OutputModeViewport, "test_step", nil)

	// Execute command that fails - viewport should capture output and return error.
	cmd := exec.Command("sh", "-c", "echo 'partial' && exit 1")
	stdout, stderr, err := writer.Execute(cmd)

	assert.Error(t, err)
	assert.Contains(t, stdout, "partial")
	assert.Empty(t, stderr)
}

func TestOutputModeWriterExecuteNoneMultipleCommands(t *testing.T) {
	writer := NewOutputModeWriter(OutputModeNone, "test_step", nil)

	// First command.
	cmd1 := exec.Command("echo", "first")
	stdout1, _, err1 := writer.Execute(cmd1)
	require.NoError(t, err1)
	assert.Contains(t, stdout1, "first")

	// Second command.
	cmd2 := exec.Command("echo", "second")
	stdout2, _, err2 := writer.Execute(cmd2)
	require.NoError(t, err2)
	assert.Contains(t, stdout2, "second")
}

func TestOutputModeWriterExecuteWithEnvVars(t *testing.T) {
	writer := NewOutputModeWriter(OutputModeNone, "test_step", nil)

	cmd := exec.Command("sh", "-c", "echo $TEST_VAR")
	cmd.Env = append(cmd.Env, "TEST_VAR=test_value")
	stdout, stderr, err := writer.Execute(cmd)

	require.NoError(t, err)
	assert.Contains(t, stdout, "test_value")
	assert.Empty(t, stderr)
}

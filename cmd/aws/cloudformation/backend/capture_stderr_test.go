package backend

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptureStderrRestoresAfterPanic(t *testing.T) {
	original := os.Stderr
	t.Cleanup(func() { os.Stderr = original })
	var captured *os.File
	require.Panics(t, func() {
		captureStderr(t, func() {
			captured = os.Stderr
			panic("callback stopped")
		})
	})
	assert.Same(t, original, os.Stderr)
	err := captured.Close()
	assert.ErrorIs(t, err, os.ErrClosed)
	assert.Equal(t, "captured", captureStderr(t, func() {
		_, err := os.Stderr.WriteString("captured")
		require.NoError(t, err)
	}))
	assert.Same(t, original, os.Stderr)
}

func TestCaptureStderrLargeOutput(t *testing.T) {
	original := os.Stderr
	t.Cleanup(func() { os.Stderr = original })
	// Exceed pipe capacity to catch captures that drain only after the callback.
	text := strings.Repeat("large output\n", 1<<17)
	var captured *os.File
	got := captureStderr(t, func() {
		captured = os.Stderr
		n, err := os.Stderr.WriteString(text)
		require.NoError(t, err)
		require.Equal(t, len(text), n)
	})
	assert.Equal(t, strings.Join(strings.Fields(text), " "), got)
	assert.Same(t, original, os.Stderr)
	err := captured.Close()
	assert.ErrorIs(t, err, os.ErrClosed)
}

func TestCaptureStderrRestoresAfterFailNow(t *testing.T) {
	const helperEnv = "ATMOS_TEST_CAPTURE_STDERR_FAIL_NOW"
	const restored = "capture restored and closed after FailNow"
	if os.Getenv(helperEnv) == "1" {
		original := os.Stderr
		var captured *os.File
		// A subprocess lets the callback genuinely fail without failing its parent test.
		passed := t.Run("callback fails", func(t *testing.T) {
			captureStderr(t, func() {
				captured = os.Stderr
				t.FailNow()
			})
		})
		err := captured.Close()
		if !passed && os.Stderr == original && errors.Is(err, os.ErrClosed) {
			_, _ = os.Stdout.WriteString(restored)
		}
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestCaptureStderrRestoresAfterFailNow$")
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	output, err := cmd.CombinedOutput()
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)
	assert.Equal(t, 1, exitError.ExitCode())
	assert.Contains(t, string(output), restored)
}

package step

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecuteWithIO_AllModesCaptureOutput exercises the ExecuteWithIO dispatch and
// every per-mode handler (none/raw/log/viewport/default). Each must invoke the
// runner with stdout/stderr writers and return the captured output. In the
// non-TTY test environment viewport falls back to log, and an unknown mode falls
// back to log via the default branch.
func TestExecuteWithIO_AllModesCaptureOutput(t *testing.T) {
	runner := func(stdout, stderr io.Writer) error {
		_, _ = io.WriteString(stdout, "hello-out")
		_, _ = io.WriteString(stderr, "hello-err")
		return nil
	}

	modes := []OutputMode{
		OutputModeNone,
		OutputModeRaw,
		OutputModeLog,
		OutputModeViewport,
		OutputMode("unknown-defaults-to-log"),
	}
	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			w := NewOutputModeWriter(mode, "step", nil)
			out, errOut, err := w.ExecuteWithIO(runner)
			require.NoError(t, err)
			assert.Equal(t, "hello-out", out)
			assert.Equal(t, "hello-err", errOut)
		})
	}
}

func TestExecuteWithIO_PropagatesRunnerError(t *testing.T) {
	wantErr := errors.New("boom")
	runner := func(stdout, _ io.Writer) error {
		_, _ = io.WriteString(stdout, "partial")
		return wantErr
	}

	for _, mode := range []OutputMode{OutputModeNone, OutputModeRaw, OutputModeLog, OutputModeViewport} {
		t.Run(string(mode), func(t *testing.T) {
			w := NewOutputModeWriter(mode, "step", nil)
			out, _, err := w.ExecuteWithIO(runner)
			require.ErrorIs(t, err, wantErr)
			assert.Equal(t, "partial", out)
		})
	}
}

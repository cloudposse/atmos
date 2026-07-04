package analyze

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// redirectToDevNull replaces os.Stdout and os.Stderr with os.DevNull
// and suppresses the global logger, so that tee output, spinner messages,
// and log.Error calls do not leak into go test's output (which causes CI failures).
func redirectToDevNull(t *testing.T) {
	t.Helper()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	require.NoError(t, err)

	realStdout := os.Stdout
	realStderr := os.Stderr
	os.Stdout = devNull
	os.Stderr = devNull

	// Also redirect the logger (it caches its writer at init time,
	// so changing os.Stderr alone does not suppress log.Error output).
	log.SetOutput(devNull)

	t.Cleanup(func() {
		os.Stdout = realStdout
		os.Stderr = realStderr
		log.SetOutput(realStderr)
		devNull.Close()
	})
}

func TestStartCapture_CapturesStdout(t *testing.T) {
	redirectToDevNull(t)

	cs, err := StartCapture()
	require.NoError(t, err)
	t.Cleanup(func() { cs.Stop() })

	// Write to stdout (which is now the pipe).
	fmt.Fprint(os.Stdout, "hello stdout")

	stdout, stderr := cs.Stop()

	assert.Equal(t, "hello stdout", stdout)
	assert.Empty(t, stderr)
}

func TestStartCapture_CapturesStderr(t *testing.T) {
	redirectToDevNull(t)

	cs, err := StartCapture()
	require.NoError(t, err)
	t.Cleanup(func() { cs.Stop() })

	// Write to stderr (which is now the pipe).
	fmt.Fprint(os.Stderr, "hello stderr")

	stdout, stderr := cs.Stop()

	assert.Empty(t, stdout)
	assert.Equal(t, "hello stderr", stderr)
}

func TestStartCapture_CapturesBoth(t *testing.T) {
	redirectToDevNull(t)

	cs, err := StartCapture()
	require.NoError(t, err)
	t.Cleanup(func() { cs.Stop() })

	fmt.Fprint(os.Stdout, "out1")
	fmt.Fprint(os.Stderr, "err1")
	fmt.Fprint(os.Stdout, "out2")

	stdout, stderr := cs.Stop()

	assert.Equal(t, "out1out2", stdout)
	assert.Equal(t, "err1", stderr)
}

func TestStartCapture_TeesToOriginalStreams(t *testing.T) {
	// Create pipes to act as the "original" stdout/stderr so we can read what was teed.
	origStdoutR, origStdoutW, err := os.Pipe()
	require.NoError(t, err)
	defer origStdoutR.Close()

	origStderrR, origStderrW, err := os.Pipe()
	require.NoError(t, err)
	defer origStderrR.Close()

	// Replace os.Stdout/os.Stderr with our pipes before capture starts.
	realStdout := os.Stdout
	realStderr := os.Stderr
	os.Stdout = origStdoutW
	os.Stderr = origStderrW
	defer func() {
		os.Stdout = realStdout
		os.Stderr = realStderr
	}()

	cs, captureErr := StartCapture()
	require.NoError(t, captureErr)
	t.Cleanup(func() { cs.Stop() })

	// Write while capture is active.
	fmt.Fprint(os.Stdout, "teed-out")
	fmt.Fprint(os.Stderr, "teed-err")

	stdout, stderr := cs.Stop()

	// Close the write ends so reads don't block.
	origStdoutW.Close()
	origStderrW.Close()

	// Verify capture buffer got the data.
	assert.Equal(t, "teed-out", stdout, "capture buffer should contain stdout")
	assert.Equal(t, "teed-err", stderr, "capture buffer should contain stderr")

	// Verify the original streams also received the data (tee contract).
	var origStdoutBuf, origStderrBuf [256]byte
	n, _ := origStdoutR.Read(origStdoutBuf[:])
	assert.Equal(t, "teed-out", string(origStdoutBuf[:n]), "original stdout should receive teed output")

	n, _ = origStderrR.Read(origStderrBuf[:])
	assert.Equal(t, "teed-err", string(origStderrBuf[:n]), "original stderr should receive teed output")
}

func TestStartCapture_RestoresStreams(t *testing.T) {
	redirectToDevNull(t)

	origStdout := os.Stdout
	origStderr := os.Stderr

	cs, err := StartCapture()
	require.NoError(t, err)
	t.Cleanup(func() { cs.Stop() })

	// During capture, os.Stdout/os.Stderr should be the pipes.
	assert.NotEqual(t, origStdout, os.Stdout)
	assert.NotEqual(t, origStderr, os.Stderr)

	cs.Stop()

	// After stop, os.Stdout/os.Stderr should be restored to the devNull streams.
	assert.Equal(t, origStdout, os.Stdout)
	assert.Equal(t, origStderr, os.Stderr)
}

func TestStartCapture_DoubleStopIsSafe(t *testing.T) {
	redirectToDevNull(t)

	cs, err := StartCapture()
	require.NoError(t, err)
	t.Cleanup(func() { cs.Stop() })

	fmt.Fprint(os.Stdout, "data")

	stdout1, stderr1 := cs.Stop()
	assert.Equal(t, "data", stdout1)
	assert.Empty(t, stderr1)

	// Second Stop() should not panic and should return the same buffered data.
	stdout2, stderr2 := cs.Stop()
	assert.Equal(t, "data", stdout2)
	assert.Empty(t, stderr2)
}

func TestStartCapture_EmptyOutput(t *testing.T) {
	redirectToDevNull(t)

	cs, err := StartCapture()
	require.NoError(t, err)
	t.Cleanup(func() { cs.Stop() })

	stdout, stderr := cs.Stop()

	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

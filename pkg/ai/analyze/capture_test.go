package analyze

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartCapture_CapturesStdout(t *testing.T) {
	cs, err := StartCapture()
	require.NoError(t, err)

	// Write to stdout (which is now the pipe).
	fmt.Fprint(os.Stdout, "hello stdout")

	stdout, stderr := cs.Stop()

	assert.Equal(t, "hello stdout", stdout)
	assert.Empty(t, stderr)
}

func TestStartCapture_CapturesStderr(t *testing.T) {
	cs, err := StartCapture()
	require.NoError(t, err)

	// Write to stderr (which is now the pipe).
	fmt.Fprint(os.Stderr, "hello stderr")

	stdout, stderr := cs.Stop()

	assert.Empty(t, stdout)
	assert.Equal(t, "hello stderr", stderr)
}

func TestStartCapture_CapturesBoth(t *testing.T) {
	cs, err := StartCapture()
	require.NoError(t, err)

	fmt.Fprint(os.Stdout, "out1")
	fmt.Fprint(os.Stderr, "err1")
	fmt.Fprint(os.Stdout, "out2")

	stdout, stderr := cs.Stop()

	assert.Equal(t, "out1out2", stdout)
	assert.Equal(t, "err1", stderr)
}

func TestStartCapture_RestoresStreams(t *testing.T) {
	origStdout := os.Stdout
	origStderr := os.Stderr

	cs, err := StartCapture()
	require.NoError(t, err)

	// During capture, os.Stdout/os.Stderr should be the pipes.
	assert.NotEqual(t, origStdout, os.Stdout)
	assert.NotEqual(t, origStderr, os.Stderr)

	cs.Stop()

	// After stop, os.Stdout/os.Stderr should be restored.
	assert.Equal(t, origStdout, os.Stdout)
	assert.Equal(t, origStderr, os.Stderr)
}

func TestStartCapture_EmptyOutput(t *testing.T) {
	cs, err := StartCapture()
	require.NoError(t, err)

	stdout, stderr := cs.Stop()

	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

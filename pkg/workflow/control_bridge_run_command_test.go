package workflow

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newFakeControlChildRequest builds a ControlCommandRequest that runs the test
// binary itself as the child process (via the controlBridgeFakeChildEnv gate in
// testmain_test.go), so plainControlRunCommand can be exercised without a real
// "atmos" binary or a platform-specific shell.
func newFakeControlChildRequest(t *testing.T, program, dir, marker string, fail bool) (*ControlCommandRequest, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	env := os.Environ()
	env = append(env, controlBridgeFakeChildEnv+"=1", controlBridgeFakeChildMarker+"="+marker)
	if fail {
		env = append(env, controlBridgeFakeChildFailEnv+"=1")
	}

	var liveStdout, liveStderr, captureStdout, captureStderr bytes.Buffer
	request := &ControlCommandRequest{
		Context: context.Background(),
		Program: program,
		Dir:     dir,
		Env:     env,
		Streams: process.Streams{Stdout: &liveStdout, Stderr: &liveStderr},
		Stdout:  &captureStdout,
		Stderr:  &captureStderr,
	}
	return request, &liveStdout, &liveStderr, &captureStdout, &captureStderr
}

func TestPlainControlRunCommand_HonorsDirEnvAndTeesOutput(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	tmpDir := t.TempDir()

	request, liveStdout, _, captureStdout, _ := newFakeControlChildRequest(t, exe, tmpDir, "hello123", false)

	require.NoError(t, plainControlRunCommand(request))

	assert.Contains(t, captureStdout.String(), "marker=hello123")
	// The fake child writes a marker file into its cwd; checking for its
	// existence (rather than string-matching a printed cwd) avoids false
	// negatives from Windows reporting os.Getwd() as an 8.3 short path.
	_, statErr := os.Stat(filepath.Join(tmpDir, controlBridgeFakeChildCwdMarkerFile))
	assert.NoError(t, statErr, "Dir must be honored: expected marker file in %s", tmpDir)
	assert.Equal(t, liveStdout.String(), captureStdout.String(), "controlWriter must tee identical output to the live stream and the capture buffer")
}

func TestPlainControlRunCommand_AtmosSubstitutesCurrentExecutable(t *testing.T) {
	tmpDir := t.TempDir()

	// A literal "atmos" binary won't be on PATH in CI, so success here is
	// strong evidence that request.Program == schema.TaskTypeAtmos triggered
	// the os.Executable() substitution rather than trying to exec "atmos" verbatim.
	request, _, _, captureStdout, _ := newFakeControlChildRequest(t, schema.TaskTypeAtmos, tmpDir, "atmos-substitution", false)

	require.NoError(t, plainControlRunCommand(request))
	assert.Contains(t, captureStdout.String(), "marker=atmos-substitution")
}

func TestPlainControlRunCommand_PropagatesFailureAndTeesStderr(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	request, _, liveStderr, _, captureStderr := newFakeControlChildRequest(t, exe, t.TempDir(), "fail-case", true)

	err = plainControlRunCommand(request)
	require.Error(t, err)

	assert.Contains(t, captureStderr.String(), "fake-child-stderr")
	assert.Equal(t, liveStderr.String(), captureStderr.String(), "controlWriter must tee stderr identically on failure too")
}

func TestPlainControlRunCommand_FallsBackToOSEnvironWhenRequestEnvEmpty(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	t.Setenv(controlBridgeFakeChildEnv, "1")
	t.Setenv(controlBridgeFakeChildMarker, "env-fallback")

	var captureStdout bytes.Buffer
	request := &ControlCommandRequest{
		Context: context.Background(),
		Program: exe,
		Dir:     t.TempDir(),
		// Env intentionally left nil so plainControlRunCommand falls back to os.Environ(),
		// which t.Setenv above has already populated with the fake-child gate.
		Streams: process.Streams{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}},
		Stdout:  &captureStdout,
		Stderr:  &bytes.Buffer{},
	}

	require.NoError(t, plainControlRunCommand(request))
	assert.Contains(t, captureStdout.String(), "marker=env-fallback")
}

func TestControlWriter(t *testing.T) {
	t.Run("tees to both writers when both are set", func(t *testing.T) {
		var stream, capture bytes.Buffer
		w := controlWriter(&stream, &capture)
		n, err := w.Write([]byte("hi"))
		require.NoError(t, err)
		assert.Equal(t, 2, n)
		assert.Equal(t, "hi", stream.String())
		assert.Equal(t, "hi", capture.String())
	})

	t.Run("returns capture when only capture is set", func(t *testing.T) {
		var capture bytes.Buffer
		w := controlWriter(nil, &capture)
		assert.Same(t, &capture, w)
	})

	t.Run("returns stream when only stream is set", func(t *testing.T) {
		var stream bytes.Buffer
		w := controlWriter(&stream, nil)
		assert.Same(t, &stream, w)
	})

	t.Run("returns nil when neither is set", func(t *testing.T) {
		assert.Nil(t, controlWriter(nil, nil))
	})
}

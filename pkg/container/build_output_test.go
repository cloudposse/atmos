package container

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	buildOutputHelperEnv = "_ATMOS_BUILD_OUTPUT_HELPER"
	buildOutputMarkerEnv = "_ATMOS_BUILD_OUTPUT_MARKER"
)

// TestBuildOutputHelper is the cross-platform child process used below.
func TestBuildOutputHelper(t *testing.T) {
	if os.Getenv(buildOutputHelperEnv) == "" {
		return
	}
	if os.Getenv(buildOutputHelperEnv) == "marker" {
		// Proves the process was executed by leaving a file behind.
		_ = os.WriteFile(os.Getenv(buildOutputMarkerEnv), []byte("ran"), 0o600)
		os.Exit(0)
	}
	_, _ = io.WriteString(os.Stdout, "build stage ready\n")
	if os.Getenv(buildOutputHelperEnv) == "wait" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	var ack [1]byte
	_, _ = os.Stdin.Read(ack[:])
	_, _ = io.WriteString(os.Stderr, "build failed detail\n")
	os.Exit(7)
}

type buildOutputAck struct {
	stdin io.WriteCloser
	once  sync.Once
}

func (w *buildOutputAck) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "ready") {
		w.once.Do(func() { _, _ = w.stdin.Write([]byte("x")); _ = w.stdin.Close() })
	}
	return len(p), nil
}

func TestBuildOutputStreamsBeforeExitAndRetainsFailure(t *testing.T) {
	t.Parallel()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestBuildOutputHelper$")
	cmd.Env = append(os.Environ(), buildOutputHelperEnv+"=1")
	reader, stdin := io.Pipe()
	cmd.Stdin = reader
	defer reader.Close()
	defer stdin.Close()
	captured, err := runBuildCommand(ctx, cmd, &buildOutputAck{stdin: stdin})
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 7, exit.ExitCode(), "child cannot finish until live output is delivered")
	require.NoError(t, ctx.Err())
	require.Contains(t, string(captured), "build stage ready")
	require.Contains(t, string(captured), "build failed detail")
}

// cancelBuildOutput cancels only after the child has started producing logs.
type cancelBuildOutput struct{ cancel context.CancelFunc }

func (w cancelBuildOutput) Write(p []byte) (int, error) {
	w.cancel()
	return len(p), nil
}

func TestBuildOutputCancellationStopsRunningChild(t *testing.T) {
	t.Parallel()
	executable, err := os.Executable()
	require.NoError(t, err)
	timeout, stop := context.WithTimeout(t.Context(), 15*time.Second)
	defer stop()
	ctx, cancel := context.WithCancel(timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestBuildOutputHelper$")
	cmd.Env = append(os.Environ(), buildOutputHelperEnv+"=wait")
	captured, err := runBuildCommand(ctx, cmd, cancelBuildOutput{cancel: cancel})
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, timeout.Err(), "cancellation must reap the child before the outer deadline")
	require.Contains(t, string(captured), "build stage ready")
}

// copyTestBinaryAsDocker links (or, when linking is unsupported, copies) the running test binary to
// <dir>/docker so it can stand in for a docker executable without relying on shell scripts or
// Unix-only tools. Linking avoids duplicating a large race-instrumented binary on disk.
func copyTestBinaryAsDocker(t *testing.T, dir string) string {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	name := "docker"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	target := filepath.Join(dir, name)
	if err := os.Link(executable, target); err == nil {
		return target
	}
	content, err := os.ReadFile(executable)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target, content, 0o700))
	return target
}

func TestBuildOutputRejectsExecutableFromRelativePathEntry(t *testing.T) {
	// t.Chdir and t.Setenv forbid t.Parallel.
	dir := t.TempDir()
	// A relative entry with a separator resolves to "bin/docker", which the runner would execute
	// verbatim; a bare "." entry would be re-rejected by the runner's own lookup and prove nothing.
	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.Mkdir(binDir, 0o700))
	absolute := copyTestBinaryAsDocker(t, binDir)
	marker := filepath.Join(dir, "marker")
	helperArgs := []string{"-test.run=^TestBuildOutputHelper$"}
	helperEnv := append(os.Environ(), buildOutputHelperEnv+"=marker", buildOutputMarkerEnv+"="+marker)

	t.Run("control: absolute path runs and leaves the marker", func(t *testing.T) {
		cmd := exec.CommandContext(t.Context(), absolute, helperArgs...)
		cmd.Env = helperEnv
		_, err := runBuildCommand(t.Context(), cmd, nil)
		require.NoError(t, err)
		require.FileExists(t, marker, "the marker mechanism must work or the test below proves nothing")
		require.NoError(t, os.Remove(marker))
	})

	t.Run("docker found through a relative PATH entry is not executed", func(t *testing.T) {
		t.Chdir(dir)
		t.Setenv("PATH", "bin")
		cmd := exec.CommandContext(t.Context(), "docker", helperArgs...)
		require.ErrorIs(t, cmd.Err, exec.ErrDot, "precondition: os/exec must reject the relative PATH entry")
		cmd.Env = helperEnv
		captured, err := runBuildCommand(t.Context(), cmd, nil)
		require.ErrorIs(t, err, errUtils.ErrProcessStartFailed)
		require.ErrorIs(t, err, exec.ErrDot)
		require.Empty(t, captured)
		require.NoFileExists(t, marker, "the rejected executable must never run")
	})
}

package container

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const buildOutputHelperEnv = "_ATMOS_BUILD_OUTPUT_HELPER"

// TestBuildOutputHelper is the cross-platform child process used below.
func TestBuildOutputHelper(t *testing.T) {
	if os.Getenv(buildOutputHelperEnv) == "" {
		return
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

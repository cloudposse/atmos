package process

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/signals"
)

const shutdownHelperDirEnv = "_ATMOS_TEST_SHUTDOWN_HELPER_DIR"

func TestRun_OwnedTerminalShutdown(t *testing.T) {
	skipOnWindows(t)
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			original := stdinIsTerminal
			stdinIsTerminal = func(io.Reader) bool { return true }
			t.Cleanup(func() { stdinIsTerminal = original })
			ctx, release := signals.WithShutdownContext(context.Background())
			t.Cleanup(release)
			dir := t.TempDir()
			killAtCleanup(t, dir, "child")
			binary, err := os.Executable()
			require.NoError(t, err)
			done := runAsync(ctx, &TaskSpec{
				Command: binary, Args: []string{"-test.run=^TestOwnedShutdownHelper$"},
				Env:     append(os.Environ(), shutdownHelperDirEnv+"="+dir),
				Streams: Streams{Stdin: os.Stdin}, ForwardSignals: true,
			})
			pid := waitForPID(t, dir, "child")
			child, err := os.FindProcess(pid)
			require.NoError(t, err)
			if sig == syscall.SIGINT {
				// A terminal delivers Ctrl-C to both processes in its foreground group.
				require.NoError(t, child.Signal(sig))
				awaitShutdownHelperFile(t, dir, "received")
			}
			require.True(t, signals.HandleShutdownSignal(sig))
			awaitShutdownHelperFile(t, dir, "received")
			select {
			case <-done:
				t.Fatal("the runner must wait for the child's signal handler")
			case <-time.After(100 * time.Millisecond):
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0o600))
			result := awaitRun(t, done)
			assert.Equal(t, 7, result.ExitCode, "Ctrl-C must not be delivered twice")
			var exitErr *exec.ExitError
			require.ErrorAs(t, result.Err, &exitErr)
			assert.Equal(t, 7, exitErr.ExitCode())
			requireGone(t, pid, "terminal child")
		})
	}
}

func TestOwnedShutdownHelper(t *testing.T) {
	dir := os.Getenv(shutdownHelperDirEnv)
	if dir == "" {
		return
	}
	incoming := make(chan os.Signal, 2)
	signal.Notify(incoming, os.Interrupt, syscall.SIGTERM)
	t.Cleanup(func() { signal.Stop(incoming) })
	writePIDFile(dir, "child")
	select {
	case <-incoming:
		require.NoError(t, os.WriteFile(filepath.Join(dir, "received"), nil, 0o600))
	case <-time.After(pidWait):
		t.Fatal("the child did not receive the shutdown signal")
	}
	deadline := time.Now().Add(pidWait)
	for time.Now().Before(deadline) {
		select {
		case <-incoming:
			os.Exit(99)
		case <-time.After(10 * time.Millisecond):
		}
		if _, err := os.Stat(filepath.Join(dir, "release")); err == nil {
			os.Exit(7)
		}
	}
	t.Fatal("the child was not released")
}

func awaitShutdownHelperFile(t *testing.T, dir, name string) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}, pidWait, 10*time.Millisecond, "helper file %s was not written (pid %s)", name, strconv.Itoa(readPID(dir, "child")))
}

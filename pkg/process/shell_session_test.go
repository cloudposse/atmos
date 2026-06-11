package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/signals"
	"github.com/cloudposse/atmos/pkg/terminal/pty"
)

// echoEnvToFileCommand returns a shell command that writes the value of an
// environment variable to a file, portable across sh and cmd.exe.
func echoEnvToFileCommand(envVar, path string) string {
	if runtime.GOOS == "windows" {
		return "echo %" + envVar + "% > " + path
	}
	return "echo $" + envVar + " > " + path
}

func TestRunShellSession_AttachedExitCode(t *testing.T) {
	err := RunShellSession(context.Background(), &ShellSessionSpec{
		Command: "exit 7",
		Name:    "exit-test",
	})

	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.Code)
}

func TestRunShellSession_AttachedSuccess(t *testing.T) {
	t.Setenv("ATMOS_SHLVL", "") // Hermetic: ignore any ambient shell level.
	marker := filepath.Join(t.TempDir(), "marker")

	err := RunShellSession(context.Background(), &ShellSessionSpec{
		Command: echoEnvToFileCommand("ATMOS_SHLVL", marker),
		Name:    "shlvl-test",
	})
	require.NoError(t, err)

	content, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, "1", strings.TrimSpace(string(content)), "ATMOS_SHLVL must be set in the session environment")
}

func TestRunShellSession_DryRunDoesNotExecute(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "marker")

	err := RunShellSession(context.Background(), &ShellSessionSpec{
		Command: echoEnvToFileCommand("ATMOS_SHLVL", marker),
		Name:    "dry-run-test",
		DryRun:  true,
	})
	require.NoError(t, err)

	assert.NoFileExists(t, marker, "dry-run must not execute the command")
}

func TestRunShellSession_WorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pwd-based working directory check requires sh")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")

	err := RunShellSession(context.Background(), &ShellSessionSpec{
		Command: "pwd > " + marker,
		Name:    "dir-test",
		Dir:     dir,
	})
	require.NoError(t, err)

	content, err := os.ReadFile(marker)
	require.NoError(t, err)
	// Resolve symlinks (macOS TempDir lives under /private/var via /var symlink).
	gotDir, err := filepath.EvalSymlinks(strings.TrimSpace(string(content)))
	require.NoError(t, err)
	wantDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, wantDir, gotDir)
}

func TestRunShellSession_ContextCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep-based cancellation check requires sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := RunShellSession(ctx, &ShellSessionSpec{
		Command: "sleep 10",
		Name:    "cancel-test",
	})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second, "cancellation must kill the child promptly")
}

func TestRunShellSession_InteractiveSuspendsInterruptExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep-based timing check requires sh")
	}
	require.False(t, signals.InterruptExitSuspended())

	done := make(chan error, 1)
	go func() {
		done <- RunShellSession(context.Background(), &ShellSessionSpec{
			Command:     "sleep 0.5",
			Name:        "suspend-test",
			Interactive: true,
		})
	}()

	// The suspension must be active while the session runs.
	assert.Eventually(t, signals.InterruptExitSuspended, time.Second, 10*time.Millisecond)

	require.NoError(t, <-done)
	assert.False(t, signals.InterruptExitSuspended(), "suspension must be released when the session ends")
}

func TestRunShellSession_PTYExitCode(t *testing.T) {
	if !pty.IsSupported() {
		t.Skipf("PTY not supported on %s", runtime.GOOS)
	}

	err := RunShellSession(context.Background(), &ShellSessionSpec{
		Command: "exit 7",
		Name:    "pty-exit-test",
		TTY:     true,
	})

	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.Code)
}

func TestRunShellSession_PTYChildSeesTTY(t *testing.T) {
	if !pty.IsSupported() {
		t.Skipf("PTY not supported on %s", runtime.GOOS)
	}
	marker := filepath.Join(t.TempDir(), "marker")

	// `test -t 0` and `test -t 1` succeed only when the streams are a TTY.
	err := RunShellSession(context.Background(), &ShellSessionSpec{
		Command: "test -t 0 && test -t 1 && echo tty > " + marker + "; sleep 0.1",
		Name:    "pty-tty-test",
		TTY:     true,
	})
	require.NoError(t, err)

	content, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, "tty", strings.TrimSpace(string(content)))
}

func TestSessionExitError(t *testing.T) {
	assert.NoError(t, sessionExitError(nil))

	plain := errors.New("boom")
	assert.Equal(t, plain, sessionExitError(plain))
}

func TestSessionShell(t *testing.T) {
	shell, flag := sessionShell()
	if runtime.GOOS == "windows" {
		assert.Equal(t, "/C", flag)
		assert.NotEmpty(t, shell)
	} else {
		assert.Equal(t, "sh", shell)
		assert.Equal(t, "-c", flag)
	}
}

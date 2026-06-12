package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

func shellSessionHelperCommand(t *testing.T, args ...string) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)

	commandArgs := append([]string{exe, "-test.run=TestShellSessionHelper", "--"}, args...)
	quoted := make([]string, len(commandArgs))
	for i, arg := range commandArgs {
		quoted[i] = shellQuoteArg(arg)
	}
	return strings.Join(quoted, " ")
}

func shellQuoteArg(arg string) string {
	if runtime.GOOS == "windows" {
		// cmd.exe /S /C passes the command verbatim; the child (the Go test
		// binary) parses standard quoting, so plain quotes are correct.
		return `"` + arg + `"`
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

func TestShellSessionHelper(t *testing.T) {
	args := helperArgs()
	if args == nil {
		return
	}

	switch args[0] {
	case "write-env":
		require.Len(t, args, 3)
		require.NoError(t, os.WriteFile(args[2], []byte(os.Getenv(args[1])), 0o600))
	case "write-dir":
		require.Len(t, args, 2)
		wd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(args[1], []byte(wd), 0o600))
	case "sleep":
		require.Len(t, args, 2)
		duration, err := time.ParseDuration(args[1])
		require.NoError(t, err)
		time.Sleep(duration)
	case "write-tty":
		require.Len(t, args, 2)
		if isCharDevice(os.Stdin) && isCharDevice(os.Stdout) {
			require.NoError(t, os.WriteFile(args[1], []byte("tty"), 0o600))
		}
	default:
		t.Fatalf("unknown helper command: %s", args[0])
	}
	os.Exit(0)
}

func helperArgs() []string {
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			return os.Args[i+1:]
		}
	}
	return nil
}

func isCharDevice(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
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
		Command: shellSessionHelperCommand(t, "write-env", "ATMOS_SHLVL", marker),
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
		Command: shellSessionHelperCommand(t, "write-env", "ATMOS_SHLVL", marker),
		Name:    "dry-run-test",
		DryRun:  true,
	})
	require.NoError(t, err)

	assert.NoFileExists(t, marker, "dry-run must not execute the command")
}

func TestRunShellSession_DryRunIgnoresMalformedShellLevel(t *testing.T) {
	t.Setenv("ATMOS_SHLVL", "not-a-number")

	err := RunShellSession(context.Background(), &ShellSessionSpec{
		Command: shellSessionHelperCommand(t, "sleep", "1ms"),
		Name:    "dry-run-shlvl-test",
		DryRun:  true,
	})
	require.NoError(t, err)
}

func TestRunShellSession_WorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")

	err := RunShellSession(context.Background(), &ShellSessionSpec{
		Command: shellSessionHelperCommand(t, "write-dir", marker),
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
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := RunShellSession(ctx, &ShellSessionSpec{
		Command: shellSessionHelperCommand(t, "sleep", "10s"),
		Name:    "cancel-test",
	})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second, "cancellation must kill the child promptly")
}

func TestRunShellSession_InteractiveSuspendsInterruptExit(t *testing.T) {
	require.False(t, signals.InterruptExitSuspended())

	done := make(chan error, 1)
	go func() {
		done <- RunShellSession(context.Background(), &ShellSessionSpec{
			Command:     shellSessionHelperCommand(t, "sleep", "500ms"),
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

	err := RunShellSession(context.Background(), &ShellSessionSpec{
		Command: shellSessionHelperCommand(t, "write-tty", marker),
		Name:    "pty-tty-test",
		// The test binary links TUI libraries that query the terminal when
		// stdout is a TTY and block waiting for replies; a dumb terminal
		// suppresses the queries.
		Env: append(os.Environ(), "TERM=dumb", "NO_COLOR=1"),
		TTY: true,
	})
	require.NoError(t, err)

	content, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, "tty", strings.TrimSpace(string(content)))
}

func TestRunSessionAttachedWarnsWhenMaskingEnabled(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(exe, "-test.run=TestShellSessionHelper", "--", "sleep", "1ms")
	cmd.Env = os.Environ()

	err = runSessionAttached(cmd, &ShellSessionSpec{EnableMasking: true})
	require.NoError(t, err)
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

func TestRunSessionAttachedWarnsWhenTTYRequested(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(exe, "-test.run=TestShellSessionHelper", "--", "sleep", "1ms")
	cmd.Env = os.Environ()

	// TTY requested but running through the attached fallback: the loss of
	// masking is unexpected, so the visible warning branch fires.
	err = runSessionAttached(cmd, &ShellSessionSpec{TTY: true, EnableMasking: true})
	require.NoError(t, err)
}

func TestRunShellSession_NilContext(t *testing.T) {
	err := RunShellSession(nil, &ShellSessionSpec{ //nolint:staticcheck // Intentionally nil to cover the default branch.
		Command: "exit 0",
		Name:    "nil-ctx-test",
	})
	assert.NoError(t, err)
}

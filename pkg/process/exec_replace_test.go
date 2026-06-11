package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestExecReplaceHelper is the subprocess entry point for exec tests.
// ReplaceShellSession cannot run inside the test process (on Unix it replaces
// the process), so tests spawn the test binary in this mode and assert on the
// resulting output and exit code.
//
// Helper args: exec-replace <command> <dir> [KEY=VALUE ...].
func TestExecReplaceHelper(t *testing.T) {
	args := execHelperArgs()
	if args == nil || args[0] != "exec-replace" {
		return
	}
	require.GreaterOrEqual(t, len(args), 3)

	err := ReplaceShellSession(&ExecSpec{
		Command: args[1],
		Name:    "exec-helper",
		Dir:     args[2],
		Env:     append(os.Environ(), args[3:]...),
	})
	// On Unix this code is unreachable when the exec succeeds (the process is
	// replaced). On Windows the emulation returns the child's exit code.
	var exitErr errUtils.ExitCodeError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.Code)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "exec-replace failed:", err)
		os.Exit(120)
	}
	os.Exit(0)
}

// execHelperArgs returns the args after the "--" separator, or nil when the
// test binary is not running in helper mode.
func execHelperArgs() []string {
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			return os.Args[i+1:]
		}
	}
	return nil
}

// runExecReplaceHelper spawns the test binary in exec-replace mode and
// returns combined output and exit code.
func runExecReplaceHelper(t *testing.T, command, dir string, extraEnv ...string) (string, int) {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)

	args := append([]string{"-test.run=TestExecReplaceHelper", "--", "exec-replace", command, dir}, extraEnv...)
	cmd := exec.Command(exe, args...)
	out, runErr := cmd.CombinedOutput()

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		require.ErrorAs(t, runErr, &exitErr, "helper output: %s", out)
		exitCode = exitErr.ExitCode()
	}
	return string(out), exitCode
}

func TestReplaceShellSession_ExitCodePropagates(t *testing.T) {
	// "exit 7" is valid in both sh and cmd.exe.
	out, code := runExecReplaceHelper(t, "exit 7", "")
	assert.Equal(t, 7, code, "helper output: %s", out)
}

func TestReplaceShellSession_ReplacesProcessAndInheritsEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses sh syntax")
	}

	// If the process is truly replaced, none of the helper's Go code runs
	// after the exec: the output is exactly what the command prints.
	out, code := runExecReplaceHelper(t, `echo "marker=$EXEC_TEST_MARKER"`, "", "EXEC_TEST_MARKER=inherited-ok")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "marker=inherited-ok", "exec'd command must inherit the provided environment")
	assert.NotContains(t, out, "exec-replace failed", "exec must not fall through to the helper error path")
}

func TestReplaceShellSession_ShellLevelNotIncremented(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses sh syntax")
	}

	// Shell exec semantics: the session replaces Atmos rather than nesting
	// under it, so ATMOS_SHLVL must pass through unchanged.
	out, code := runExecReplaceHelper(t, `echo "shlvl=$ATMOS_SHLVL"`, "", "ATMOS_SHLVL=5")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "shlvl=5")
}

func TestReplaceShellSession_WorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses sh syntax")
	}
	dir := t.TempDir()

	out, code := runExecReplaceHelper(t, "pwd", dir)
	assert.Equal(t, 0, code)

	gotDir, err := filepath.EvalSymlinks(strings.TrimSpace(out))
	require.NoError(t, err)
	wantDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, wantDir, gotDir)
}

func TestReplaceShellSession_DryRunDoesNotReplace(t *testing.T) {
	// Safe to call in-process: dry-run returns before any replacement.
	err := ReplaceShellSession(&ExecSpec{
		Command: "exit 7",
		Name:    "dry-run-test",
		DryRun:  true,
	})
	assert.NoError(t, err)
}

func TestReplaceShellSession_InvalidWorkingDirectory(t *testing.T) {
	// Safe to call in-process: Chdir fails before any replacement.
	err := ReplaceShellSession(&ExecSpec{
		Command: "exit 0",
		Name:    "bad-dir-test",
		Dir:     filepath.Join(t.TempDir(), "does-not-exist"),
	})
	assert.Error(t, err)
}

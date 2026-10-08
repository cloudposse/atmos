package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Environment gates that let this test binary stand in for Atmos and for a child program.
const (
	envRunMain = "_ATMOS_TEST_RUN_MAIN" // Run main() with the remaining arguments.
	envSleep   = "_ATMOS_TEST_SLEEP"    // Block until killed.
	envWrite   = "_ATMOS_TEST_WRITE"    // Write a marker file at this path and exit.
	envExit    = "_ATMOS_TEST_EXIT"     // Exit with this status.
)

// TestMain turns the test binary into a cross-platform stand-in for Atmos (envRunMain) and for the
// programs a script starts (envWrite, envSleep), so the interrupt test needs no shell or system tools.
func TestMain(m *testing.M) {
	if path := os.Getenv(envWrite); path != "" {
		if err := os.WriteFile(path, []byte("done"), 0o600); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if code := os.Getenv(envExit); code != "" {
		status, err := strconv.Atoi(code)
		if err != nil {
			os.Exit(1)
		}
		os.Exit(status)
	}
	if os.Getenv(envSleep) == "1" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	if os.Getenv(envRunMain) == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

// starlarkString quotes a path for a Starlark source literal.
func starlarkString(value string) string {
	return fmt.Sprintf("%q", value)
}

// waitForFile polls for a marker file the script or its child writes.
func waitForFile(t *testing.T, path string, within time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, within, 20*time.Millisecond, "timed out waiting for %s", path)
}

// Prerequisite: the stand-in program writes its marker, so a missing marker later can only mean
// the deferred call did not run.
func TestInterruptHelperWritesMarker(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	marker := filepath.Join(t.TempDir(), "marker")
	command := exec.Command(exe)
	command.Env = append(os.Environ(), envWrite+"="+marker)
	require.NoError(t, command.Run())
	assert.FileExists(t, marker)
}

// Ctrl-C used to exit the process at once, so a script's deferred cleanup never ran. The interrupt
// now cancels the script, its deferred calls run, and the process exits with the conventional 130.
func TestInterruptRunsDeferredCallsThenExits130(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("an interrupt cannot be delivered to a child process on Windows")
	}
	exe, err := os.Executable()
	require.NoError(t, err)
	dir := t.TempDir()
	ready, deferred := filepath.Join(dir, "ready"), filepath.Join(dir, "deferred")
	source := strings.Join([]string{
		fmt.Sprintf("defer(lambda: exec.run([%s], env={%q: %s}))", starlarkString(exe), envWrite, starlarkString(deferred)),
		fmt.Sprintf("exec.run([%s], env={%q: %s})", starlarkString(exe), envWrite, starlarkString(ready)),
		fmt.Sprintf("exec.run([%s], env={%q: \"1\"})", starlarkString(exe), envSleep),
		"print(\"must not be reached\")",
	}, "\n")
	scriptPath := filepath.Join(dir, "interrupt.star")
	require.NoError(t, os.WriteFile(scriptPath, []byte(source), 0o600))

	command := exec.Command(exe, scriptPath)
	command.Dir = dir
	command.Env = append(os.Environ(), envRunMain+"=1", "ATMOS_VERSION_CHECK_ENABLED=false", "NO_COLOR=1")
	var output strings.Builder
	command.Stdout, command.Stderr = &output, &output
	require.NoError(t, command.Start())
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	t.Cleanup(func() { _ = command.Process.Kill() })

	waitForFile(t, ready, 30*time.Second)
	require.NoError(t, command.Process.Signal(syscall.SIGINT))

	select {
	case err := <-finished:
		var exitErr *exec.ExitError
		require.True(t, errors.As(err, &exitErr), "the process must exit with a status, got %v\n%s", err, output.String())
		assert.Equal(t, 130, exitErr.ExitCode(), output.String())
	case <-time.After(60 * time.Second):
		t.Fatalf("the script did not stop after the interrupt\n%s", output.String())
	}
	assert.FileExists(t, deferred, "the deferred call must run before the process exits\n"+output.String())
	assert.NotContains(t, output.String(), "must not be reached")
}

// runAtmos runs this test binary as Atmos in dir and returns its exit status and combined output.
func runAtmos(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()
	return runAtmosWithStdin(t, dir, "", args...)
}

// runAtmosWithStdin is runAtmos with the given text piped to standard input.
func runAtmosWithStdin(t *testing.T, dir, stdin string, args ...string) (int, string) {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(exe, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), envRunMain+"=1", "ATMOS_VERSION_CHECK_ENABLED=false", "NO_COLOR=1")
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
	output, err := command.CombinedOutput()
	if err == nil {
		return 0, string(output)
	}
	var exitErr *exec.ExitError
	require.True(t, errors.As(err, &exitErr), "unexpected failure: %v\n%s", err, output)
	return exitErr.ExitCode(), string(output)
}

// A script piped to `atmos -` runs, receives the arguments after the marker, and prints no
// "reading from stdin" prompt because stdin is not a terminal.
func TestStdinScriptRuns(t *testing.T) {
	status, output := runAtmosWithStdin(t, t.TempDir(), "print(\"from stdin\", ctx.args)\n", "-", "one", "two")
	assert.Equal(t, 0, status, output)
	assert.Contains(t, output, `from stdin ["one", "two"]`)
	assert.NotContains(t, output, "Reading script from stdin")
}

// exec.run(check=True) failing makes Atmos exit with the child's own status, the same as a shell
// would, whether the script is a standalone file or a step of a custom command.
func TestScriptProcessFailureExitsWithTheChildStatus(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	source := fmt.Sprintf("exec.run([%s], env={%q: \"7\"})\nprint(\"must not be reached\")\n", starlarkString(exe), envExit)

	t.Run("standalone script", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "child.star"), []byte(source), 0o600))
		status, output := runAtmos(t, dir, "./child.star")
		assert.Equal(t, 7, status, output)
		assert.NotContains(t, output, "must not be reached")
	})
	t.Run("custom command script step", func(t *testing.T) {
		dir := t.TempDir()
		config := "base_path: \".\"\ncommands:\n  - name: child\n    description: Run a failing child.\n    steps:\n      - type: script\n        interpreter: starlark\n        script: !literal |\n" +
			"          " + strings.ReplaceAll(strings.TrimSpace(source), "\n", "\n          ") + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(config), 0o600))
		status, output := runAtmos(t, dir, "child")
		assert.Equal(t, 7, status, output)
		assert.NotContains(t, output, "must not be reached")
	})
	t.Run("check=False keeps going", func(t *testing.T) {
		dir := t.TempDir()
		lenient := fmt.Sprintf("result = exec.run([%s], env={%q: \"7\"}, check=False)\nprint(\"exit code\", result.exit_code)\n", starlarkString(exe), envExit)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lenient.star"), []byte(lenient), 0o600))
		status, output := runAtmos(t, dir, "./lenient.star")
		assert.Equal(t, 0, status, output)
		assert.Contains(t, output, "exit code 7")
	})
}

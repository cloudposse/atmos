package main

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
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

const (
	// The fake Terraform records signals and its progress under this directory.
	envFakeTerraform = "_ATMOS_TEST_FAKE_GRACEFUL_TERRAFORM"
	// When set, the fake Terraform never exits on its own.
	envFakeTerraformStuck = "_ATMOS_TEST_FAKE_GRACEFUL_TERRAFORM_STUCK"

	fakeTerraformSaveTime = time.Second
)

// runFakeTerraform stands in for `terraform` reacting to an interrupt: it records each signal,
// takes a moment to "save state", and exits 1 as an interrupted Terraform apply does.
func runFakeTerraform(dir string) int {
	received := make(chan os.Signal, 4)
	signal.Notify(received, os.Interrupt, syscall.SIGTERM)
	if err := os.WriteFile(filepath.Join(dir, "ready"), nil, 0o600); err != nil {
		return 2
	}
	record := func(sig os.Signal) {
		f, err := os.OpenFile(filepath.Join(dir, "signals"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = f.WriteString(sig.String() + "\n")
			_ = f.Close()
		}
	}
	if os.Getenv(envFakeTerraformStuck) == "1" {
		for sig := range received {
			record(sig)
		}
	}
	record(<-received)
	time.Sleep(fakeTerraformSaveTime)
	if err := os.WriteFile(filepath.Join(dir, "saved"), nil, 0o600); err != nil {
		return 2
	}
	return 1
}

// startAtmosWithFakeTerraform runs `atmos terraform version` against the fake Terraform.
func startAtmosWithFakeTerraform(t *testing.T, stuck bool) (*exec.Cmd, string, <-chan error, *strings.Builder) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("signals cannot be sent to a child process on Windows")
	}
	exe, err := os.Executable()
	require.NoError(t, err)
	dir := t.TempDir()
	stuckValue := "0"
	if stuck {
		stuckValue = "1"
	}
	config := strings.Join([]string{
		`base_path: "."`,
		"env:",
		"  " + envFakeTerraform + ": " + strconv.Quote(dir),
		"  " + envFakeTerraformStuck + ": " + stuckValue,
		"components:",
		"  terraform:",
		"    command: " + strconv.Quote(exe),
		"stacks:",
		"  base_path: stacks",
		`  included_paths: ["**/*"]`,
		`  name_template: "{{ .vars.stage }}"`,
		"",
	}, "\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(config), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "dev.yaml"), []byte("vars:\n  stage: dev\n"), 0o600))

	command := exec.Command(exe, "terraform", "version")
	command.Dir = dir
	command.Env = append(os.Environ(), envRunMain+"=1", "ATMOS_VERSION_CHECK_ENABLED=false", "NO_COLOR=1")
	output := &strings.Builder{}
	command.Stdout, command.Stderr = output, output
	require.NoError(t, command.Start())
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	t.Cleanup(func() { _ = command.Process.Kill() })
	waitForFile(t, filepath.Join(dir, "ready"), 30*time.Second)
	return command, dir, finished, output
}

func exitStatus(t *testing.T, err error, output string) int {
	t.Helper()
	var exitErr *exec.ExitError
	require.True(t, errors.As(err, &exitErr), "atmos must exit with a status, got %v\n%s", err, output)
	return exitErr.ExitCode()
}

func recordedFakeSignals(dir string) []string {
	data, _ := os.ReadFile(filepath.Join(dir, "signals"))
	return strings.Fields(string(data))
}

// Issue #3751: a signal used to make Atmos exit at once and kill Terraform before it saved state.
func TestFirstSignalWaitsForTerraformToStop(t *testing.T) {
	for _, tc := range []struct {
		sig    syscall.Signal
		name   string
		code   int
		notice string
	}{
		{
			sig: syscall.SIGINT, name: "interrupt", code: 130,
			notice: "Interrupt received; waiting for atmos.test to stop gracefully",
		},
		{
			sig: syscall.SIGTERM, name: "terminated", code: 143,
			notice: "Termination requested; waiting for atmos.test to stop gracefully",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command, dir, finished, output := startAtmosWithFakeTerraform(t, false)
			require.NoError(t, command.Process.Signal(tc.sig))
			select {
			case err := <-finished:
				t.Fatalf("atmos exited before Terraform stopped: %v\n%s", err, output.String())
			case <-time.After(fakeTerraformSaveTime / 2):
			}
			select {
			case err := <-finished:
				assert.Equal(t, tc.code, exitStatus(t, err, output.String()), output.String())
			case <-time.After(30 * time.Second):
				t.Fatalf("atmos did not exit after Terraform stopped\n%s", output.String())
			}
			assert.FileExists(t, filepath.Join(dir, "saved"), "Terraform must finish saving state")
			assert.Equal(t, []string{tc.name}, recordedFakeSignals(dir), "Terraform receives the signal exactly once")
			assert.Contains(t, output.String(), tc.notice)
		})
	}
}

func TestSecondSignalForcesExit(t *testing.T) {
	command, dir, finished, output := startAtmosWithFakeTerraform(t, true)
	require.NoError(t, command.Process.Signal(syscall.SIGINT))
	firstReceived := func() bool { return len(recordedFakeSignals(dir)) == 1 }
	require.Eventually(t, firstReceived, 10*time.Second, 20*time.Millisecond)
	require.NoError(t, command.Process.Signal(syscall.SIGINT))
	select {
	case err := <-finished:
		assert.Equal(t, 130, exitStatus(t, err, output.String()), output.String())
	case <-time.After(30 * time.Second):
		t.Fatalf("the second signal did not force the exit\n%s", output.String())
	}
	assert.NoFileExists(t, filepath.Join(dir, "saved"))
}

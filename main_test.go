package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/signals"
)

func TestShouldExitOnSignal(t *testing.T) {
	tests := []struct {
		name      string
		sig       os.Signal
		suspended bool
		want      bool
	}{
		{name: "SIGINT exits when not suspended", sig: os.Interrupt, suspended: false, want: true},
		{name: "SIGINT ignored while suspended", sig: os.Interrupt, suspended: true, want: false},
		{name: "SIGTERM always exits", sig: syscall.SIGTERM, suspended: false, want: true},
		{name: "SIGTERM exits even while suspended", sig: syscall.SIGTERM, suspended: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.suspended {
				release := signals.SuspendInterruptExit()
				defer release()
			}
			assert.Equal(t, tt.want, shouldExitOnSignal(tt.sig))
		})
	}
}

func TestSignalsRetainExitIntentDuringCredentialCleanup(t *testing.T) {
	_, release := signals.DeferExitForCleanup(t.Context())
	t.Cleanup(release)
	assert.True(t, shouldExitOnSignal(os.Interrupt))
	assert.True(t, shouldExitOnSignal(syscall.SIGTERM))
	release()
	assert.True(t, shouldExitOnSignal(os.Interrupt))
	assert.True(t, shouldExitOnSignal(syscall.SIGTERM))
}

func TestHasVersionFlag(t *testing.T) {
	assert.True(t, hasVersionFlag([]string{"atmos", "--version"}))
	assert.False(t, hasVersionFlag([]string{"atmos", "version"}))
	assert.False(t, hasVersionFlag([]string{"atmos"}))
	assert.False(t, hasVersionFlag([]string{"atmos", "terraform", "--version"}))
}

func TestHasUseVersionFlag(t *testing.T) {
	assert.True(t, hasUseVersionFlag([]string{"atmos", "--use-version", "1.2.3"}))
	assert.True(t, hasUseVersionFlag([]string{"atmos", "--use-version=1.2.3"}))
	assert.False(t, hasUseVersionFlag([]string{"atmos", "--version"}))
}

func TestSilentExitCode(t *testing.T) {
	// Silent exit-code carrier: return (code, true).
	code, ok := silentExitCode(errUtils.ExitCodeError{Code: 7, Silent: true})
	assert.True(t, ok)
	assert.Equal(t, 7, code)

	// Wrapped silent error is still detected.
	code, ok = silentExitCode(fmt.Errorf("step failed: %w", errUtils.ExitCodeError{Code: 130, Silent: true}))
	assert.True(t, ok)
	assert.Equal(t, 130, code)

	// Non-silent exit-code error is rendered normally (not silent).
	_, ok = silentExitCode(errUtils.ExitCodeError{Code: 1})
	assert.False(t, ok)

	// Plain errors are not silent.
	_, ok = silentExitCode(errors.New("boom"))
	assert.False(t, ok)
}

func TestSignalExitNoticeCoversDirectExit(t *testing.T) {
	const helperEnv = "_ATMOS_TEST_SIGNAL_EXIT_NOTICE"
	if os.Getenv(helperEnv) == "1" {
		var codes []int
		errUtils.OsExit = func(code int) { codes = append(codes, code) }
		installSignalExitNotice()
		errUtils.OsExit(0)
		cleaned := 0
		unregister := signals.RegisterExitCleanup(func() { cleaned++ })
		defer unregister()
		errUtils.OsExit(130)
		signals.RequestExit(os.Interrupt)
		errUtils.OsExit(0)
		assert.Equal(t, []int{0, 130, 130}, codes)
		assert.Equal(t, 1, cleaned)
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	child := exec.CommandContext(t.Context(), executable, "-test.run=^TestSignalExitNoticeCoversDirectExit$")
	child.Env = append(os.Environ(), helperEnv+"=1")
	output, err := child.CombinedOutput()
	require.NoError(t, err, "%s", output)
	assert.Equal(t, 1, strings.Count(string(output), "Interrupted."), "%s", output)
}

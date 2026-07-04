package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

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

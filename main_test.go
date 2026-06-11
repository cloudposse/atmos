package main

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

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

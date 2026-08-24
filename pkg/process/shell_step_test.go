package process

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunShellStep_PlainStepRunsFallback(t *testing.T) {
	called := false
	err := RunShellStep(context.Background(), &ShellSessionSpec{
		Command: "echo plain",
		Name:    "plain-step",
	}, func() error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called, "plain steps must run via the caller-provided fallback")
}

func TestRunShellStep_PlainStepPropagatesFallbackError(t *testing.T) {
	wantErr := errors.New("boom")
	err := RunShellStep(context.Background(), &ShellSessionSpec{
		Command: "echo plain",
		Name:    "plain-step",
	}, func() error { return wantErr })
	assert.ErrorIs(t, err, wantErr)
}

func TestRunShellStep_TerminalStepsBypassFallback(t *testing.T) {
	tests := []struct {
		name string
		spec ShellSessionSpec
	}{
		{name: "tty", spec: ShellSessionSpec{TTY: true}},
		{name: "interactive", spec: ShellSessionSpec{Interactive: true}},
		{name: "tty and interactive", spec: ShellSessionSpec{TTY: true, Interactive: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.spec
			spec.Command = "echo session"
			spec.Name = "session-step"
			spec.DryRun = true // Route check only; don't execute.

			fallbackCalled := false
			err := RunShellStep(context.Background(), &spec, func() error {
				fallbackCalled = true
				return nil
			})
			require.NoError(t, err)
			assert.False(t, fallbackCalled, "terminal steps must use the session path, not the fallback")
		})
	}
}

package terraform

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/signals"
)

// Cancellation that is not a signal must not look like one: running Terraform components are
// only spared from termination when the multi-component context ended because of SIGINT/SIGTERM.
func TestTerraformSignalContextFollowsTheCommandContext(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	command := &cobra.Command{}
	command.SetContext(parent)
	ctx, stop := terraformSignalContext(command)
	defer stop()
	require.NoError(t, ctx.Err())
	cancel()
	<-ctx.Done()
	assert.False(t, signals.InterruptedBySignal(ctx))

	fallback, stopFallback := terraformSignalContext(&cobra.Command{})
	require.NoError(t, fallback.Err(), "a command without a context still gets a live context")
	stopFallback()
	<-fallback.Done()
	assert.False(t, signals.InterruptedBySignal(fallback))
}

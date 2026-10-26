package emulator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStepInfo(t *testing.T) {
	info := stepInfo("aws", "local")
	assert.Equal(t, "aws", info.ComponentFromArg)
	assert.Equal(t, "aws", info.Component)
	assert.Equal(t, "local", info.Stack)
}

func TestStepRunner_RoutesActions(t *testing.T) {
	origUp, origEph, origDown, origReset := stepUp, stepUpEphemeral, stepDown, stepReset
	t.Cleanup(func() {
		stepUp, stepUpEphemeral, stepDown, stepReset = origUp, origEph, origDown, origReset
	})

	var calls []string
	var gotInfo *schema.ConfigAndStacksInfo
	var gotForce bool
	stepUp = func(_ context.Context, info *schema.ConfigAndStacksInfo) error {
		calls, gotInfo = append(calls, "up"), info
		return nil
	}
	stepUpEphemeral = func(_ context.Context, info *schema.ConfigAndStacksInfo) error {
		calls, gotInfo = append(calls, "up-ephemeral"), info
		return nil
	}
	stepDown = func(_ context.Context, info *schema.ConfigAndStacksInfo) error {
		calls, gotInfo = append(calls, "down"), info
		return nil
	}
	stepReset = func(_ context.Context, info *schema.ConfigAndStacksInfo, force bool) error {
		calls, gotInfo, gotForce = append(calls, "reset"), info, force
		return nil
	}

	r := stepRunner{}
	require.NoError(t, r.Up(context.Background(), "aws", "local", false))
	require.NoError(t, r.Up(context.Background(), "aws", "local", true))
	require.NoError(t, r.Down(context.Background(), "aws", "local"))
	require.NoError(t, r.Reset(context.Background(), "aws", "local"))

	assert.Equal(t, []string{"up", "up-ephemeral", "down", "reset"}, calls)
	assert.True(t, gotForce, "step-driven reset must be non-interactive (force=true)")
	// stepInfo wires the component/stack from the step into the executor info.
	assert.Equal(t, "aws", gotInfo.ComponentFromArg)
	assert.Equal(t, "local", gotInfo.Stack)
}

func TestStepRunner_PropagatesError(t *testing.T) {
	origDown := stepDown
	t.Cleanup(func() { stepDown = origDown })

	sentinel := errors.New("down failed")
	stepDown = func(_ context.Context, _ *schema.ConfigAndStacksInfo) error { return sentinel }

	err := stepRunner{}.Down(context.Background(), "aws", "local")
	require.ErrorIs(t, err, sentinel)
}

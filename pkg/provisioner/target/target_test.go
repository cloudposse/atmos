package target

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// fakeProvisioner records Deliver calls so tests can assert the registry returns
// the same registered instance (registry pattern, not a fresh factory build).
type fakeProvisioner struct {
	id        string
	delivered int
}

func (f *fakeProvisioner) Deliver(_ context.Context, _ *DeliverInput) error {
	f.delivered++
	return nil
}

func TestRegisterAndGetReturnSameInstance(t *testing.T) {
	// Use a unique kind so the test does not collide with real init()-registered kinds.
	const kind = "test-kind-same-instance"
	inst := &fakeProvisioner{id: "a"}
	Register(kind, inst)

	got1, ok1 := Get(kind)
	got2, ok2 := Get(kind)
	require.True(t, ok1)
	require.True(t, ok2)
	// Registry pattern: Get returns the SAME instance every time, not a new build.
	assert.Same(t, inst, got1)
	assert.Same(t, got1, got2)
}

func TestRegisterReplacesPriorInstance(t *testing.T) {
	const kind = "test-kind-replace"
	first := &fakeProvisioner{id: "first"}
	second := &fakeProvisioner{id: "second"}
	Register(kind, first)
	Register(kind, second)

	got, ok := Get(kind)
	require.True(t, ok)
	assert.Same(t, second, got)
}

func TestGetUnknownKind(t *testing.T) {
	_, ok := Get("nope-not-registered")
	assert.False(t, ok)
}

func TestDeliverRoutesToRegisteredInstance(t *testing.T) {
	const kind = "test-kind-deliver"
	inst := &fakeProvisioner{}
	Register(kind, inst)

	err := Deliver(context.Background(), kind, &DeliverInput{TargetName: "t"})
	require.NoError(t, err)
	assert.Equal(t, 1, inst.delivered)
}

func TestDeliverUnknownKind(t *testing.T) {
	err := Deliver(context.Background(), "nope-not-registered", &DeliverInput{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProvisionTargetKindUnknown)
}

func TestRegisteredKindsSorted(t *testing.T) {
	Register("zeta-kind", &fakeProvisioner{})
	Register("alpha-kind", &fakeProvisioner{})
	kinds := RegisteredKinds()
	// Confirm alpha-kind sorts before zeta-kind in the returned slice.
	ai, zi := -1, -1
	for i, k := range kinds {
		switch k {
		case "alpha-kind":
			ai = i
		case "zeta-kind":
			zi = i
		}
	}
	require.NotEqual(t, -1, ai)
	require.NotEqual(t, -1, zi)
	assert.Less(t, ai, zi)
}

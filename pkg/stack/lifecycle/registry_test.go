package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestRegistryFixedEventsAndIsolation(t *testing.T) {
	r := NewRegistry()
	for _, event := range []Event{PlanEvent, Build, Test, Deploy, Destroy, Validate} {
		h := NativeHandler("widgets", string(event))
		h.HookEvents = []string{"before.widgets." + string(event)}
		require.NoError(t, r.Register("widgets", event, h))
		_, ok := r.Lookup("widgets", event)
		assert.True(t, ok)
		require.ErrorContains(t, r.Register("widgets", event, h), "duplicate")
		h.Path[0] = "mutated"
	}
	require.Error(t, r.Register("widgets", Event("arbitrary"), NativeHandler("widgets", "arbitrary")))
	require.Error(t, r.Register("", Deploy, NativeHandler("deploy")))
	require.Error(t, r.Register("other", Deploy, &Handler{}))
	snapshot := r.Handlers(Deploy)
	snapshot["widgets"].Path[0] = "changed"
	snapshot["widgets"].HookEvents[0] = "changed"
	delete(snapshot, "widgets")
	handler, ok := r.Lookup("widgets", Deploy)
	require.True(t, ok)
	assert.Equal(t, "widgets", handler.Path[0])
	assert.Equal(t, "before.widgets.deploy", handler.HookEvents[0])
	_, ok = r.Lookup("other", Deploy)
	assert.False(t, ok)
}

func TestZeroValueRegistryRegistersLazily(t *testing.T) {
	// A zero-value Registry has no handler maps until the first registration.
	var r Registry
	require.NoError(t, r.Register("widgets", Deploy, NativeHandler("widgets", "deploy")))
	handler, ok := r.Lookup("widgets", Deploy)
	require.True(t, ok)
	assert.Equal(t, []string{"widgets", "deploy"}, handler.Path)
	require.ErrorIs(t, r.Register("widgets", Deploy, NativeHandler("widgets", "deploy")), errUtils.ErrInvalidConfig)
}

func TestMustRegisterPanicsOnInvalidRegistration(t *testing.T) {
	before := len(DefaultRegistry.Handlers(Deploy))
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		MustRegister("", Deploy, NativeHandler("deploy"))
	}()
	err, ok := recovered.(error)
	require.True(t, ok, "MustRegister panics with the registration error, got %v", recovered)
	assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	assert.Len(t, DefaultRegistry.Handlers(Deploy), before, "a rejected registration leaves the default registry unchanged")
}

package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryFixedEventsAndIsolation(t *testing.T) {
	r := NewRegistry()
	for _, event := range []Event{Build, Test, Deploy, Destroy, Validate} {
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

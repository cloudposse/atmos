package deferred

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/degradation"
)

func TestValueCacheStoresOnlyResolvedValues(t *testing.T) {
	cache := &ValueCache{}
	for _, value := range []any{
		degradation.AtmosComputedValue{},
		map[string]any{"nested": []any{"ok", degradation.AtmosComputedValue{}}},
	} {
		cache.Store("pending", value)
		_, found := cache.Load("pending")
		require.False(t, found)
	}
	resolved := map[string]any{"nested": []any{"resolved", 42}}
	cache.Store("result", resolved)
	got, found := cache.Load("result")
	require.True(t, found)
	require.Equal(t, resolved, got)
	_, found = cache.Load("other")
	require.False(t, found)
	var disabled *ValueCache
	disabled.Store("result", "ignored")
	_, found = disabled.Load("result")
	require.False(t, found)
}

package deferred

import (
	"sync"
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

func TestValueCacheLoadOrStore(t *testing.T) {
	cache := &ValueCache{}
	var wg sync.WaitGroup
	values := make([]any, 20)
	for i := range values {
		wg.Go(func() { values[i] = cache.LoadOrStore("binding", &struct{ id int }{i}) })
	}
	wg.Wait()
	for _, value := range values {
		require.Same(t, values[0], value, "concurrent lookups share a stable binding")
	}
	for _, value := range []any{nil, degradation.AtmosComputedValue{}} {
		require.Equal(t, value, cache.LoadOrStore("unresolved", value))
		_, found := cache.Load("unresolved")
		require.False(t, found)
	}
	var disabled *ValueCache
	require.Equal(t, "fresh", disabled.LoadOrStore("binding", "fresh"))
}

func TestIsCacheable(t *testing.T) {
	for _, value := range []any{nil, map[string]any(nil), []any(nil), (*string)(nil), degradation.AtmosComputedValue{}, map[string]any{"nested": []any{degradation.AtmosComputedValue{}}}} {
		require.False(t, IsCacheable(value))
	}
	for _, value := range []any{"", false, 0, map[string]any{}, map[string]any{"null_output": nil}, []any{1, 2}} {
		require.True(t, IsCacheable(value), "legitimate empty and null output values remain cacheable")
	}
}

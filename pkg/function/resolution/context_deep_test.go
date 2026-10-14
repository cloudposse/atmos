package resolution

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A deep lookup must retain the same goroutine-local context as its caller.
// Full stack traces can exceed the old capture limit even below resolution's
// depth limit, because each nested lookup adds several Go frames.
func TestResolutionContextDeepStackIsolation(t *testing.T) {
	t.Parallel()
	type result struct {
		shallowID, deepID string
		shallow, deep     *Context
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			defer Clear()
			shallowID := getGoroutineID()
			shallow := GetOrCreate()
			var deepID string
			var deep *Context
			withDeepResolutionStack(128, func() {
				deepID = getGoroutineID()
				deep = GetOrCreate()
			})
			results <- result{shallowID, deepID, shallow, deep}
		}()
	}
	first, second := <-results, <-results
	for _, got := range []result{first, second} {
		require.NotEqual(t, "unknown", got.deepID)
		require.Equal(t, got.shallowID, got.deepID)
		require.Same(t, got.shallow, got.deep)
	}
	require.NotEqual(t, first.deepID, second.deepID)
	require.NotSame(t, first.deep, second.deep)
}

//go:noinline
func withDeepResolutionStack(depth int, lookup func()) {
	if depth == 0 {
		lookup()
		return
	}
	withDeepResolutionStack(depth-1, lookup)
}
